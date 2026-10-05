package permission

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxFingerprintBytes = 512
	maxIdentityBytes    = 1024
	maxGenerations      = 32
	maxGenerationBytes  = 512
)

func (s *Store) CreateApproval(ctx context.Context, input CreateApprovalInput) (ApprovalRecord, error) {
	if err := contextDone(ctx); err != nil {
		return ApprovalRecord{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.expireLocked(); err != nil {
		return ApprovalRecord{}, err
	}
	if pendingCount(s.state.History) >= MaxPending || len(s.live) >= MaxLiveRequests {
		return ApprovalRecord{}, ErrLimit
	}
	if err := s.validateCreateInput(input); err != nil {
		return ApprovalRecord{}, err
	}

	now := s.now()
	expiresAt := input.ExpiresAt.UTC()
	if expiresAt.IsZero() {
		expiresAt = now.Add(DefaultExpiry)
	}
	if !expiresAt.After(now) {
		return ApprovalRecord{}, ErrExpired
	}
	if expiresAt.After(now.Add(DefaultExpiry)) {
		return ApprovalRecord{}, fmt.Errorf("%w: approval expiry exceeds maximum lifetime", ErrState)
	}

	id, err := newOpaqueID("approval")
	if err != nil {
		return ApprovalRecord{}, err
	}
	for approvalIndex(s.state.History, id) >= 0 {
		id, err = newOpaqueID("approval")
		if err != nil {
			return ApprovalRecord{}, err
		}
	}

	record := ApprovalRecord{
		SchemaVersion:   SchemaVersion,
		Version:         1,
		ID:              id,
		Audit:           input.Audit,
		Binding:         AuditBindingFor(input.Binding),
		Tool:            strings.TrimSpace(input.Prepared.Tool),
		Action:          strings.TrimSpace(input.Prepared.Action),
		Summary:         strings.TrimSpace(input.Summary),
		Scope:           strings.TrimSpace(input.Scope),
		Reason:          strings.TrimSpace(input.Reason),
		RuleID:          strings.TrimSpace(input.RuleID),
		PolicyRevision:  s.state.Policy.Revision,
		RuntimeEpoch:    s.epoch,
		Status:          Pending,
		CreatedAt:       now,
		ExpiresAt:       expiresAt,
		GrantKind:       "none",
		DispatchOutcome: NotDispatched,
	}

	candidate := cloneState(s.state)
	candidate.History = append(candidate.History, record)
	candidate.History = pruneHistory(candidate.History)
	if len(candidate.History) > MaxHistory {
		return ApprovalRecord{}, ErrLimit
	}
	candidate.Revision++
	if err := s.writeState(candidate); err != nil {
		return ApprovalRecord{}, err
	}
	s.state = candidate
	s.live[id] = liveApproval{
		prepared: clonePrepared(input.Prepared),
		facts:    cloneFacts(input.Facts),
	}
	return record, nil
}

func (s *Store) validateCreateInput(input CreateApprovalInput) error {
	if len(input.Summary) > MaxSummaryBytes || len(input.Scope) > MaxScopeBytes || len(input.Reason) > MaxReasonBytes {
		return fmt.Errorf("%w: approval display data exceeds bounds", ErrState)
	}
	prepared := input.Prepared
	if strings.TrimSpace(prepared.Fingerprint) == "" || len(prepared.Fingerprint) > maxFingerprintBytes {
		return fmt.Errorf("%w: prepared request fingerprint is invalid", ErrState)
	}
	if strings.TrimSpace(prepared.Tool) == "" || len(prepared.Tool) > 256 || len(prepared.Action) > 256 {
		return fmt.Errorf("%w: prepared request tool/action is invalid", ErrState)
	}
	if prepared.PolicyRevision != s.state.Policy.Revision {
		return ErrRevision
	}
	if prepared.RuntimeEpoch != s.epoch || prepared.Binding.RuntimeEpoch != s.epoch {
		return fmt.Errorf("%w: prepared request runtime epoch mismatch", ErrNotEligible)
	}
	if !permissionBindingEqual(prepared.Binding, input.Binding) || !permissionBindingEqual(input.Facts.Binding, input.Binding) {
		return fmt.Errorf("%w: prepared/facts authorization binding mismatch", ErrNotEligible)
	}
	if prepared.Tool != input.Facts.Tool || prepared.Action != input.Facts.Action {
		return fmt.Errorf("%w: prepared/facts tool identity mismatch", ErrNotEligible)
	}
	if len(prepared.Generations) > maxGenerations {
		return fmt.Errorf("%w: too many prepared request generations", ErrState)
	}
	for key, value := range prepared.Generations {
		if strings.TrimSpace(key) == "" || len(key) > maxGenerationBytes || len(value) > maxGenerationBytes {
			return fmt.Errorf("%w: invalid prepared request generation", ErrState)
		}
	}
	if err := validateBindingBounds(input.Binding); err != nil {
		return err
	}
	return nil
}

func validateBindingBounds(binding PermissionBinding) error {
	values := []string{
		binding.Principal.Kind,
		binding.Principal.ID,
		binding.RuntimeEpoch,
		binding.Source,
		binding.WorkspaceRoot,
		binding.WorkspaceID,
		binding.ACPSessionID,
		binding.Provider,
	}
	for _, value := range values {
		if len(value) > maxIdentityBytes {
			return fmt.Errorf("%w: authorization identity exceeds bound", ErrState)
		}
	}
	return nil
}

func (s *Store) ApproveOnce(ctx context.Context, mutation Mutation) (ApprovalRecord, error) {
	if err := contextDone(ctx); err != nil {
		return ApprovalRecord{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	candidate := cloneState(s.state)
	index, record, err := approvalForMutation(candidate.History, mutation)
	if err != nil {
		return ApprovalRecord{}, err
	}
	if err := s.ensurePendingMutation(&candidate, index, record, mutation, now); err != nil {
		return ApprovalRecord{}, err
	}
	record = candidate.History[index]

	live, ok := s.live[record.ID]
	if !ok {
		return ApprovalRecord{}, fmt.Errorf("%w: prepared request is no longer live", ErrNotEligible)
	}
	if !stablePrincipal(live.prepared.Binding.Principal) {
		return ApprovalRecord{}, fmt.Errorf("%w: stable authenticated principal is required", ErrNotEligible)
	}
	if live.prepared.PolicyRevision != record.PolicyRevision ||
		live.prepared.RuntimeEpoch != s.epoch ||
		!permissionBindingEqual(live.prepared.Binding, live.facts.Binding) {
		return ApprovalRecord{}, fmt.Errorf("%w: live prepared request no longer matches approval", ErrNotEligible)
	}

	record.Version++
	record.Status = ApprovedOnce
	record.DecidedAt = timePtr(now)
	record.DecidedBy = strings.TrimSpace(mutation.Actor)
	record.GrantKind = "once"
	record.DispatchOutcome = NotDispatched
	candidate.History[index] = record
	candidate.Revision++

	if err := s.writeState(candidate); err != nil {
		return ApprovalRecord{}, err
	}
	s.state = candidate
	return record, nil
}

func approvalForMutation(history []ApprovalRecord, mutation Mutation) (int, ApprovalRecord, error) {
	id := strings.TrimSpace(mutation.ApprovalID)
	if id == "" {
		return -1, ApprovalRecord{}, ErrNotFound
	}
	index := approvalIndex(history, id)
	if index < 0 {
		return -1, ApprovalRecord{}, ErrNotFound
	}
	record := history[index]
	if mutation.ApprovalVersion != record.Version {
		return -1, ApprovalRecord{}, ErrVersion
	}
	if strings.TrimSpace(mutation.Actor) == "" {
		return -1, ApprovalRecord{}, fmt.Errorf("%w: mutation actor is required", ErrState)
	}
	return index, record, nil
}

func (s *Store) ensurePendingMutation(candidate *State, index int, record ApprovalRecord, mutation Mutation, now time.Time) error {
	if record.Status != Pending {
		if record.Status == Expired {
			return ErrExpired
		}
		return fmt.Errorf("%w: approval status is %s", ErrNotEligible, record.Status)
	}
	if !now.Before(record.ExpiresAt) {
		expireRecord(&candidate.History[index], now)
		candidate.Revision++
		if err := s.writeState(*candidate); err != nil {
			return err
		}
		s.state = *candidate
		delete(s.live, record.ID)
		return ErrExpired
	}
	if mutation.PolicyRevision != s.state.Policy.Revision {
		return ErrRevision
	}
	if record.PolicyRevision != s.state.Policy.Revision || record.RuntimeEpoch != s.epoch {
		invalidateRecord(&candidate.History[index], now, "core-policy-change")
		candidate.Revision++
		if err := s.writeState(*candidate); err != nil {
			return err
		}
		s.state = *candidate
		delete(s.live, record.ID)
		return ErrRevision
	}
	return nil
}

func (s *Store) Reject(ctx context.Context, mutation Mutation) (ApprovalRecord, error) {
	if err := contextDone(ctx); err != nil {
		return ApprovalRecord{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	candidate := cloneState(s.state)
	index, record, err := approvalForMutation(candidate.History, mutation)
	if err != nil {
		return ApprovalRecord{}, err
	}
	if err := s.ensurePendingMutation(&candidate, index, record, mutation, now); err != nil {
		return ApprovalRecord{}, err
	}

	record = candidate.History[index]
	record.Version++
	record.Status = Rejected
	record.DecidedAt = timePtr(now)
	record.DecidedBy = strings.TrimSpace(mutation.Actor)
	record.GrantKind = "none"
	record.DispatchOutcome = NotDispatched
	candidate.History[index] = record
	candidate.Revision++

	if err := s.writeState(candidate); err != nil {
		return ApprovalRecord{}, err
	}
	s.state = candidate
	delete(s.live, record.ID)
	return record, nil
}

func (s *Store) ConsumeOnce(ctx context.Context, input ConsumeInput) (ApprovalRecord, bool, error) {
	if err := contextDone(ctx); err != nil {
		return ApprovalRecord{}, false, err
	}
	if strings.TrimSpace(input.RetryCallID) == "" {
		return ApprovalRecord{}, false, fmt.Errorf("%w: retry call id is required", ErrState)
	}
	if err := validateBindingBounds(input.Prepared.Binding); err != nil {
		return ApprovalRecord{}, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	candidate := cloneState(s.state)
	changed := false
	drop := make([]string, 0)

	for i := range candidate.History {
		record := &candidate.History[i]
		if record.Status != ApprovedOnce {
			continue
		}
		if !now.Before(record.ExpiresAt) {
			expireRecord(record, now)
			changed = true
			drop = append(drop, record.ID)
			continue
		}
		if record.PolicyRevision != candidate.Policy.Revision || record.RuntimeEpoch != s.epoch {
			invalidateRecord(record, now, "core-policy-change")
			changed = true
			drop = append(drop, record.ID)
			continue
		}

		live, ok := s.live[record.ID]
		if !ok {
			invalidateRecord(record, now, "core-live-grant-missing")
			changed = true
			drop = append(drop, record.ID)
			continue
		}
		if !preparedRequestEqual(live.prepared, input.Prepared) {
			continue
		}
		if !stablePrincipal(input.Prepared.Binding.Principal) {
			continue
		}

		record.Version++
		record.Status = Consumed
		record.RetryCallID = strings.TrimSpace(input.RetryCallID)
		record.DispatchOutcome = DispatchPending
		changed = true
		candidate.Revision++

		if err := s.writeState(candidate); err != nil {
			return ApprovalRecord{}, false, err
		}
		s.state = candidate
		delete(s.live, record.ID)
		for _, id := range drop {
			delete(s.live, id)
		}
		return *record, true, nil
	}

	if changed {
		candidate.Revision++
		if err := s.writeState(candidate); err != nil {
			return ApprovalRecord{}, false, err
		}
		s.state = candidate
		for _, id := range drop {
			delete(s.live, id)
		}
	}
	return ApprovalRecord{}, false, nil
}

func preparedRequestEqual(a, b PreparedRequest) bool {
	if a.Fingerprint != b.Fingerprint ||
		a.Tool != b.Tool ||
		a.Action != b.Action ||
		a.PolicyRevision != b.PolicyRevision ||
		a.RuntimeEpoch != b.RuntimeEpoch ||
		!permissionBindingEqual(a.Binding, b.Binding) ||
		len(a.Generations) != len(b.Generations) {
		return false
	}
	for key, value := range a.Generations {
		if b.Generations[key] != value {
			return false
		}
	}
	return true
}

func permissionBindingEqual(a, b PermissionBinding) bool {
	return a.Principal == b.Principal &&
		a.RuntimeEpoch == b.RuntimeEpoch &&
		a.Source == b.Source &&
		a.WorkspaceRoot == b.WorkspaceRoot &&
		a.WorkspaceID == b.WorkspaceID &&
		a.TrustedWorkspace == b.TrustedWorkspace &&
		a.ACPSessionID == b.ACPSessionID &&
		a.Provider == b.Provider
}

func (s *Store) SettleDispatch(ctx context.Context, approvalID, outcome string) (ApprovalRecord, error) {
	if err := contextDone(ctx); err != nil {
		return ApprovalRecord{}, err
	}
	switch outcome {
	case Succeeded, Failed, Unknown:
	default:
		return ApprovalRecord{}, fmt.Errorf("%w: invalid dispatch outcome %q", ErrState, outcome)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	candidate := cloneState(s.state)
	index := approvalIndex(candidate.History, strings.TrimSpace(approvalID))
	if index < 0 {
		return ApprovalRecord{}, ErrNotFound
	}
	record := candidate.History[index]
	if record.Status != Consumed {
		return ApprovalRecord{}, fmt.Errorf("%w: approval status is %s", ErrNotEligible, record.Status)
	}
	if record.DispatchOutcome != DispatchPending && record.DispatchOutcome != Unknown {
		return ApprovalRecord{}, fmt.Errorf("%w: dispatch outcome already settled", ErrNotEligible)
	}
	record.Version++
	record.DispatchOutcome = outcome
	candidate.History[index] = record
	candidate.Revision++

	if err := s.writeState(candidate); err != nil {
		return ApprovalRecord{}, err
	}
	s.state = candidate
	return record, nil
}

func (s *Store) ApproveWorkspace(ctx context.Context, input WorkspaceGrantInput) (ApprovalRecord, Policy, error) {
	if err := contextDone(ctx); err != nil {
		return ApprovalRecord{}, Policy{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	candidate := cloneState(s.state)
	index, record, err := approvalForMutation(candidate.History, input.Mutation)
	if err != nil {
		return ApprovalRecord{}, Policy{}, err
	}
	if err := s.ensurePendingMutation(&candidate, index, record, input.Mutation, now); err != nil {
		return ApprovalRecord{}, Policy{}, err
	}
	record = candidate.History[index]

	live, ok := s.live[record.ID]
	if !ok {
		return ApprovalRecord{}, Policy{}, fmt.Errorf("%w: prepared request is no longer live", ErrNotEligible)
	}
	facts := live.facts
	binding := live.prepared.Binding
	if !facts.EffectsKnown || facts.OpaqueProviderExecution || !facts.WorkspaceRuleEligible {
		return ApprovalRecord{}, Policy{}, fmt.Errorf("%w: opaque or unenforceable effects cannot receive a workspace grant", ErrNotEligible)
	}
	if !binding.TrustedWorkspace || strings.TrimSpace(binding.WorkspaceID) == "" {
		return ApprovalRecord{}, Policy{}, fmt.Errorf("%w: trusted workspace binding is required", ErrNotEligible)
	}
	if !permissionBindingEqual(binding, facts.Binding) {
		return ApprovalRecord{}, Policy{}, fmt.Errorf("%w: workspace grant binding mismatch", ErrNotEligible)
	}

	workspaceID := binding.WorkspaceID
	if !policyHasScope(candidate.Policy, workspaceID) {
		candidate.Policy.Scopes = append(candidate.Policy.Scopes, WorkspaceScope{ID: workspaceID})
	}
	ruleID, err := newOpaqueID("rule")
	if err != nil {
		return ApprovalRecord{}, Policy{}, err
	}
	candidate.Policy.Rules = append(candidate.Policy.Rules, Rule{
		ID:          ruleID,
		Tool:        live.prepared.Tool,
		Action:      live.prepared.Action,
		WorkspaceID: workspaceID,
		Effect:      Allow,
		Reason:      "approved for this trusted workspace by local user",
	})
	candidate.Policy.Revision++
	candidate.Policy.SchemaVersion = SchemaVersion
	if err := ValidatePolicy(candidate.Policy); err != nil {
		return ApprovalRecord{}, Policy{}, err
	}

	decision, err := Evaluate(candidate.Policy, facts)
	if err != nil {
		return ApprovalRecord{}, Policy{}, err
	}
	if decision.Effect != Allow {
		return ApprovalRecord{}, Policy{}, fmt.Errorf("%w: workspace rule cannot override a stricter global decision", ErrNotEligible)
	}

	record.Version++
	record.Status = ApprovedWorkspace
	record.DecidedAt = timePtr(now)
	record.DecidedBy = strings.TrimSpace(input.Mutation.Actor)
	record.GrantKind = "workspace"
	record.GrantedRuleID = ruleID
	record.GrantedPolicyRevision = candidate.Policy.Revision
	record.DispatchOutcome = NotDispatched
	candidate.History[index] = record
	invalidatePolicyBoundApprovals(&candidate, now, candidate.Policy.Revision, record.ID)
	candidate.Revision++

	if err := s.writeState(candidate); err != nil {
		return ApprovalRecord{}, Policy{}, err
	}
	s.state = candidate
	s.dropInvalidatedLiveLocked()
	delete(s.live, record.ID)
	return record, candidate.Policy, nil
}

func policyHasScope(policy Policy, workspaceID string) bool {
	for _, scope := range policy.Scopes {
		if scope.ID == workspaceID {
			return true
		}
	}
	return false
}

func (s *Store) SweepExpired(ctx context.Context) error {
	if err := contextDone(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.expireLocked()
}

func newOpaqueID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
}

func approvalErrorIs(err, target error) bool {
	return errors.Is(err, target)
}

// Ensure time import remains part of the approval API contract through the
// ExpiresAt inputs even when callers use the default.
var _ = time.Time{}
