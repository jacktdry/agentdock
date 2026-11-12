package permission

import (
	"context"
	"fmt"
	"strings"
)

// AdmissionRequest is the Core-owned input to permission admission. Callers
// provide only validated, runtime-derived facts and a prepared request
// fingerprint. Raw tool arguments never enter this package.
type AdmissionRequest struct {
	Audit    AuditBinding
	Facts    PermissionFacts
	Prepared PreparedRequest
	Summary  string
	Scope    string
}

// Admission is the result of one Core permission decision. Approval is present
// only when the decision remains Ask and a pending record was created.
// ConsumedApprovalID is reserved for an approved-once retry that was consumed
// before dispatch.
type Admission struct {
	Decision           Decision
	Approval           *ApprovalRecord
	ConsumedApprovalID string
}

// AdmissionGate is the shared Core permission boundary. Runtime.Call is the
// first consumer; management and ACP bridge entrypoints reuse the same gate in
// later M8 phases.
type AdmissionGate struct {
	store *Store
}

func NewAdmissionGate(store *Store) (*AdmissionGate, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: permission store is required", ErrState)
	}
	return &AdmissionGate{store: store}, nil
}

func (g *AdmissionGate) Store() *Store {
	if g == nil {
		return nil
	}
	return g.store
}

// Admit evaluates policy and, when required, records a bounded pending
// approval. A matching approved-once grant is consumed atomically before an
// Ask can become Allow.
func (g *AdmissionGate) Admit(ctx context.Context, input AdmissionRequest) (Admission, error) {
	if g == nil || g.store == nil {
		return Admission{}, fmt.Errorf("%w: permission admission gate is unavailable", ErrState)
	}
	if err := contextDone(ctx); err != nil {
		return Admission{}, err
	}

	policy := g.store.Policy()
	facts := cloneFacts(input.Facts)
	prepared := clonePrepared(input.Prepared)

	epoch := g.store.RuntimeEpoch()
	if facts.Binding.RuntimeEpoch == "" {
		facts.Binding.RuntimeEpoch = epoch
	}
	if prepared.Binding.RuntimeEpoch == "" {
		prepared.Binding.RuntimeEpoch = epoch
	}
	prepared.PolicyRevision = policy.Revision
	prepared.RuntimeEpoch = epoch
	prepared.Tool = strings.TrimSpace(prepared.Tool)
	prepared.Action = strings.TrimSpace(prepared.Action)
	facts.Tool = strings.TrimSpace(facts.Tool)
	facts.Action = strings.TrimSpace(facts.Action)

	if prepared.Tool == "" {
		prepared.Tool = facts.Tool
	}
	if facts.Tool == "" {
		facts.Tool = prepared.Tool
	}
	if prepared.Action == "" {
		prepared.Action = facts.Action
	}
	if facts.Action == "" {
		facts.Action = prepared.Action
	}
	if prepared.Binding != facts.Binding {
		return Admission{}, fmt.Errorf("%w: prepared request and facts use different authorization bindings", ErrNotEligible)
	}

	decision, err := Evaluate(policy, facts)
	if err != nil {
		return Admission{}, err
	}
	admission := Admission{Decision: decision}
	if decision.Effect != Ask {
		return admission, nil
	}

	// A one-shot grant is optional. If no stable principal is available, or no
	// exact grant matches, ConsumeOnce simply leaves the request in Ask.
	if stablePrincipal(prepared.Binding.Principal) && strings.TrimSpace(input.Audit.CallID) != "" {
		record, consumed, consumeErr := g.store.ConsumeOnce(ctx, ConsumeInput{
			Prepared:    prepared,
			RetryCallID: input.Audit.CallID,
		})
		if consumeErr != nil {
			return Admission{}, consumeErr
		}
		if consumed {
			admission.Decision.Effect = Allow
			admission.Decision.RuleID = "approval-once"
			admission.Decision.Reason = "exact approved-once request matched the authenticated principal"
			admission.Decision.Sources = appendSource(admission.Decision.Sources, DecisionSource{
				Kind:   "approval_once",
				ID:     record.ID,
				Effect: Allow,
				Reason: "exact approved-once request consumed before dispatch",
			})
			admission.ConsumedApprovalID = record.ID
			return admission, nil
		}
	}

	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		summary = "Permission review for " + prepared.Tool
	}
	scope := strings.TrimSpace(input.Scope)
	if scope == "" {
		scope = "AgentDock Core request"
	}
	record, err := g.store.CreateApproval(ctx, CreateApprovalInput{
		Audit:    input.Audit,
		Binding:  facts.Binding,
		Facts:    facts,
		Prepared: prepared,
		Summary:  boundedUTF8(summary, MaxSummaryBytes),
		Scope:    boundedUTF8(scope, MaxScopeBytes),
		Reason:   boundedUTF8(decision.Reason, MaxReasonBytes),
		RuleID:   decision.RuleID,
	})
	if err != nil {
		return Admission{}, err
	}
	admission.Approval = &record
	return admission, nil
}
