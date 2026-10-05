package permission

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrStateCorrupt  = errors.New("permission state is corrupt")
	ErrStateTooLarge = errors.New("permission state exceeds size limit")
	ErrNotFound      = errors.New("permission approval not found")
	ErrRevision      = errors.New("permission policy revision mismatch")
	ErrVersion       = errors.New("permission approval version mismatch")
	ErrExpired       = errors.New("permission approval expired")
	ErrNotEligible   = errors.New("permission approval is not eligible for this grant")
	ErrLimit         = errors.New("permission approval limit reached")
	ErrState         = errors.New("invalid permission state")
)

type Store struct {
	mu sync.Mutex

	root      string
	stateDir  string
	statePath string
	epoch     string
	state     State
	live      map[string]liveApproval
	now       func() time.Time
}

type liveApproval struct {
	prepared PreparedRequest
	facts    PermissionFacts
}

func NewStore(root, runtimeEpoch string) (*Store, error) {
	root = strings.TrimSpace(root)
	runtimeEpoch = strings.TrimSpace(runtimeEpoch)
	if root == "" || runtimeEpoch == "" {
		return nil, fmt.Errorf("%w: root and runtime epoch are required", ErrState)
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("%w: root must be absolute", ErrState)
	}

	s := &Store{
		root:      filepath.Clean(root),
		stateDir:  filepath.Join(filepath.Clean(root), "permissions"),
		statePath: filepath.Join(filepath.Clean(root), "permissions", "state.json"),
		epoch:     runtimeEpoch,
		live:      make(map[string]liveApproval, MaxLiveRequests),
		now:       func() time.Time { return time.Now().UTC() },
	}
	if err := s.ensureStateDir(); err != nil {
		return nil, err
	}

	info, err := os.Lstat(s.statePath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		state := State{
			SchemaVersion: SchemaVersion,
			Revision:      1,
			RuntimeEpoch:  runtimeEpoch,
			Policy:        DefaultPolicy(),
			History:       []ApprovalRecord{},
		}
		if err := validateState(state); err != nil {
			return nil, err
		}
		if err := s.writeState(state); err != nil {
			return nil, err
		}
		s.state = state
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("lstat permission state: %w", err)
	case info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular():
		return nil, fmt.Errorf("%w: state.json must be a regular non-symlink file", ErrStateCorrupt)
	}
	if err := os.Chmod(s.statePath, 0o600); err != nil {
		return nil, fmt.Errorf("chmod permission state: %w", err)
	}

	state, err := s.readState()
	if err != nil {
		return nil, err
	}
	now := s.now()
	next := cloneState(state)
	changed := recoverState(&next, runtimeEpoch, now)
	if next.RuntimeEpoch != runtimeEpoch {
		next.RuntimeEpoch = runtimeEpoch
		changed = true
	}
	if changed {
		next.Revision++
		if err := s.writeState(next); err != nil {
			return nil, err
		}
	}
	s.state = next
	return s, nil
}

func (s *Store) ensureStateDir() error {
	if info, err := os.Lstat(s.stateDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: permissions directory must be a real directory", ErrStateCorrupt)
		}
		if err := os.Chmod(s.stateDir, 0o700); err != nil {
			return fmt.Errorf("chmod permissions directory: %w", err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("lstat permissions directory: %w", err)
	}
	if err := os.MkdirAll(s.stateDir, 0o700); err != nil {
		return fmt.Errorf("create permissions directory: %w", err)
	}
	info, err := os.Lstat(s.stateDir)
	if err != nil {
		return fmt.Errorf("lstat created permissions directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: permissions directory must be a real directory", ErrStateCorrupt)
	}
	return nil
}

func (s *Store) readState() (State, error) {
	file, err := os.Open(s.statePath)
	if err != nil {
		return State{}, fmt.Errorf("open permission state: %w", err)
	}
	defer file.Close()

	limited := io.LimitReader(file, MaxStateBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return State{}, fmt.Errorf("read permission state: %w", err)
	}
	if len(data) > MaxStateBytes {
		return State{}, ErrStateTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state State
	if err := decoder.Decode(&state); err != nil {
		return State{}, fmt.Errorf("%w: decode state: %v", ErrStateCorrupt, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return State{}, fmt.Errorf("%w: trailing data", ErrStateCorrupt)
	}
	if err := validateState(state); err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrStateCorrupt, err)
	}
	return state, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("unexpected second JSON value")
}

func validateState(state State) error {
	if state.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: schema_version=%d", ErrState, state.SchemaVersion)
	}
	if state.Revision == 0 {
		return fmt.Errorf("%w: revision must be positive", ErrState)
	}
	if strings.TrimSpace(state.RuntimeEpoch) == "" {
		return fmt.Errorf("%w: runtime epoch is required", ErrState)
	}
	if err := ValidatePolicy(state.Policy); err != nil {
		return err
	}
	if len(state.History) > MaxHistory {
		return fmt.Errorf("%w: history count %d exceeds %d", ErrState, len(state.History), MaxHistory)
	}
	pending := 0
	seen := make(map[string]struct{}, len(state.History))
	for i, record := range state.History {
		if err := validateApprovalRecord(record); err != nil {
			return fmt.Errorf("%w: history[%d]: %v", ErrState, i, err)
		}
		if _, exists := seen[record.ID]; exists {
			return fmt.Errorf("%w: duplicate approval id %q", ErrState, record.ID)
		}
		seen[record.ID] = struct{}{}
		if liveApprovalStatus(record.Status) {
			pending++
		}
	}
	if pending > MaxPending {
		return fmt.Errorf("%w: pending approval count %d exceeds %d", ErrState, pending, MaxPending)
	}
	return nil
}

func validateApprovalRecord(record ApprovalRecord) error {
	if record.SchemaVersion != SchemaVersion || record.Version == 0 {
		return errors.New("invalid approval schema/version")
	}
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.Tool) == "" {
		return errors.New("approval id and tool are required")
	}
	switch record.Status {
	case Pending, ApprovedOnce, Consumed, ApprovedWorkspace, Rejected, Expired, Invalidated:
	default:
		return fmt.Errorf("invalid approval status %q", record.Status)
	}
	switch record.DispatchOutcome {
	case NotDispatched, DispatchPending, Succeeded, Failed, Unknown:
	default:
		return fmt.Errorf("invalid dispatch outcome %q", record.DispatchOutcome)
	}
	if record.Decision != nil {
		if len(record.Decision.Sources) > MaxSources {
			return errors.New("approval trace exceeds bound")
		}
		for _, source := range record.Decision.Sources {
			if len(source.Kind) > 128 || len(source.ID) > 256 || len(source.Effect) > 64 || len(source.Reason) > MaxReasonBytes {
				return errors.New("approval trace field exceeds bound")
			}
		}
	}
	if len(record.Summary) > MaxSummaryBytes || len(record.Scope) > MaxScopeBytes || len(record.Reason) > MaxReasonBytes {
		return errors.New("approval display field exceeds bound")
	}
	if strings.TrimSpace(record.RuntimeEpoch) == "" || record.PolicyRevision == 0 {
		return errors.New("approval runtime epoch and policy revision are required")
	}
	if record.CreatedAt.IsZero() || record.ExpiresAt.IsZero() {
		return errors.New("approval timestamps are required")
	}
	return nil
}

func recoverState(state *State, runtimeEpoch string, now time.Time) bool {
	changed := false
	epochChanged := state.RuntimeEpoch != runtimeEpoch
	for i := range state.History {
		record := &state.History[i]
		switch record.Status {
		case Pending:
			if !now.Before(record.ExpiresAt) {
				expireRecord(record, now)
				changed = true
			} else if epochChanged || record.RuntimeEpoch != runtimeEpoch {
				invalidateRecord(record, now, "core-recovery")
				changed = true
			}
		case ApprovedOnce:
			// A persisted approved_once record has no reconstructible in-memory
			// PreparedRequest/grant in a new Store. Always fail closed.
			if !now.Before(record.ExpiresAt) {
				expireRecord(record, now)
			} else {
				invalidateRecord(record, now, "core-recovery")
			}
			changed = true
		case Consumed:
			if record.DispatchOutcome == DispatchPending {
				record.Version++
				record.DispatchOutcome = Unknown
				changed = true
			}
		}
	}
	return changed
}

func expireRecord(record *ApprovalRecord, now time.Time) {
	record.Version++
	record.Status = Expired
	record.GrantKind = "none"
	record.DispatchOutcome = NotDispatched
	record.DecidedAt = timePtr(now)
	if record.DecidedBy == "" {
		record.DecidedBy = "core-expiry"
	}
}

func invalidateRecord(record *ApprovalRecord, now time.Time, actor string) {
	record.Version++
	record.Status = Invalidated
	record.GrantKind = "none"
	record.DispatchOutcome = NotDispatched
	record.DecidedAt = timePtr(now)
	record.DecidedBy = actor
}

func (s *Store) writeState(state State) error {
	if err := validateState(state); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode permission state: %w", err)
	}
	data = append(data, '\n')
	if len(data) > MaxStateBytes {
		return ErrStateTooLarge
	}

	tmp := filepath.Join(s.stateDir, ".state.json.tmp."+randomSuffix())
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create permission state temp file: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write permission state temp file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("fsync permission state temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close permission state temp file: %w", err)
	}
	if err := os.Rename(tmp, s.statePath); err != nil {
		return fmt.Errorf("replace permission state: %w", err)
	}
	cleanup = false
	if err := os.Chmod(s.statePath, 0o600); err != nil {
		return fmt.Errorf("chmod permission state: %w", err)
	}
	if dir, err := os.Open(s.stateDir); err == nil {
		syncErr := dir.Sync()
		closeErr := dir.Close()
		if syncErr != nil {
			return fmt.Errorf("fsync permissions directory: %w", syncErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close permissions directory: %w", closeErr)
		}
	}
	return nil
}

func randomSuffix() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func cloneState(state State) State {
	data, err := json.Marshal(state)
	if err != nil {
		panic("permission: clone state marshal failed: " + err.Error())
	}
	var cloned State
	if err := json.Unmarshal(data, &cloned); err != nil {
		panic("permission: clone state unmarshal failed: " + err.Error())
	}
	return cloned
}

func clonePrepared(value PreparedRequest) PreparedRequest {
	cloned := value
	if value.Generations != nil {
		cloned.Generations = make(map[string]string, len(value.Generations))
		for key, item := range value.Generations {
			cloned.Generations[key] = item
		}
	}
	return cloned
}

func cloneFacts(value PermissionFacts) PermissionFacts {
	cloned := value
	cloned.Constraints = append([]DecisionSource(nil), value.Constraints...)
	return cloned
}

func (s *Store) Snapshot() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expireLocked(); err != nil {
		return State{}, err
	}
	return cloneState(s.state), nil
}

func (s *Store) Policy() Policy {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneState(State{
		SchemaVersion: SchemaVersion,
		Revision:      1,
		RuntimeEpoch:  s.epoch,
		Policy:        s.state.Policy,
		History:       []ApprovalRecord{},
	}).Policy
}

func (s *Store) ReplacePolicy(expectedRevision uint64, next Policy) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if expectedRevision != s.state.Policy.Revision {
		return Policy{}, ErrRevision
	}
	next.SchemaVersion = SchemaVersion
	next.Revision = s.state.Policy.Revision + 1
	if err := ValidatePolicy(next); err != nil {
		return Policy{}, err
	}

	candidate := cloneState(s.state)
	candidate.Policy = next
	now := s.now()
	invalidatePolicyBoundApprovals(&candidate, now, next.Revision, "")
	candidate.Revision++
	if err := s.writeState(candidate); err != nil {
		return Policy{}, err
	}
	s.state = candidate
	s.dropInvalidatedLiveLocked()
	return s.state.Policy, nil
}

func invalidatePolicyBoundApprovals(state *State, now time.Time, policyRevision uint64, exceptID string) {
	for i := range state.History {
		record := &state.History[i]
		if record.ID == exceptID {
			continue
		}
		if (record.Status == Pending || record.Status == ApprovedOnce) && record.PolicyRevision != policyRevision {
			invalidateRecord(record, now, "core-policy-change")
		}
	}
}

func (s *Store) dropInvalidatedLiveLocked() {
	for id := range s.live {
		record, ok := approvalByID(s.state.History, id)
		if !ok || (record.Status != Pending && record.Status != ApprovedOnce) {
			delete(s.live, id)
		}
	}
}

func (s *Store) History() ([]ApprovalRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expireLocked(); err != nil {
		return nil, err
	}
	result := cloneState(s.state).History
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func (s *Store) Approval(id string) (ApprovalRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expireLocked(); err != nil {
		return ApprovalRecord{}, err
	}
	record, ok := approvalByID(s.state.History, strings.TrimSpace(id))
	if !ok {
		return ApprovalRecord{}, ErrNotFound
	}
	return cloneState(State{History: []ApprovalRecord{record}}).History[0], nil
}

func (s *Store) expireLocked() error {
	now := s.now()
	candidate := cloneState(s.state)
	changed := false
	for i := range candidate.History {
		record := &candidate.History[i]
		if (record.Status == Pending || record.Status == ApprovedOnce) && !now.Before(record.ExpiresAt) {
			expireRecord(record, now)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	candidate.Revision++
	if err := s.writeState(candidate); err != nil {
		return err
	}
	s.state = candidate
	s.dropInvalidatedLiveLocked()
	return nil
}

func (s *Store) StatePath() string {
	return s.statePath
}

func (s *Store) RuntimeEpoch() string {
	return s.epoch
}

func approvalByID(history []ApprovalRecord, id string) (ApprovalRecord, bool) {
	for _, record := range history {
		if record.ID == id {
			return record, true
		}
	}
	return ApprovalRecord{}, false
}

func approvalIndex(history []ApprovalRecord, id string) int {
	for i := range history {
		if history[i].ID == id {
			return i
		}
	}
	return -1
}

func liveApprovalStatus(status string) bool {
	return status == Pending || status == ApprovedOnce
}

func pendingCount(history []ApprovalRecord) int {
	count := 0
	for _, record := range history {
		if liveApprovalStatus(record.Status) {
			count++
		}
	}
	return count
}

func pruneHistory(history []ApprovalRecord) []ApprovalRecord {
	for len(history) > MaxHistory {
		index := -1
		for i, record := range history {
			if !liveApprovalStatus(record.Status) {
				index = i
				break
			}
		}
		if index < 0 {
			break
		}
		history = append(history[:index], history[index+1:]...)
	}
	return history
}

func timePtr(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}

// Context is accepted by callers that need cancellation before a state mutation.
// Persistence itself remains an atomic local filesystem transaction once begun.
func contextDone(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
