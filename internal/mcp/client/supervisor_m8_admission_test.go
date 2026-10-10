package client

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/uvwt/agentdock/internal/permission"
	processcontrol "github.com/uvwt/agentdock/internal/process"
)

func m8CoreTestSetup(t *testing.T) (*permission.Store, *permission.AdmissionGate,
	CoreSupervisorM8Context, processcontrol.BirthIdentity, processcontrol.BirthIdentity,
	processcontrol.BirthIdentity, []byte, processcontrol.SupervisorHandoffChallenge) {
	t.Helper()
	root := t.TempDir()
	store, err := permission.NewStore(root, "core-epoch-1")
	if err != nil {
		t.Fatal(err)
	}
	gate, err := permission.NewAdmissionGate(store)
	if err != nil {
		t.Fatal(err)
	}
	core := processcontrol.BirthIdentity{PID: 50, ParentPID: 1, StartSec: 100, StartUsec: 2, EffectiveUID: 501, ProcessGroupID: 50}
	supervisor := processcontrol.BirthIdentity{PID: 60, ParentPID: 1, StartSec: 90, StartUsec: 2, EffectiveUID: 501, ProcessGroupID: 60}
	worker := processcontrol.BirthIdentity{PID: 70, ParentPID: 60, StartSec: 101, StartUsec: 2, EffectiveUID: 501, ProcessGroupID: 70}
	secret := bytes.Repeat([]byte{8}, 32)
	handoff, err := processcontrol.NewSupervisorOwnedReadOnlyHandoff(core, supervisor, worker, secret)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := handoff.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	input := CoreSupervisorM8Context{
		Gate: gate,
		Binding: permission.PermissionBinding{
			Principal:    permission.AuthorizationPrincipal{Kind: "test-sso", ID: "user-1", Authenticated: true, Stable: true},
			RuntimeEpoch: store.RuntimeEpoch(), Source: "supervisor_core", Provider: "local-mcp",
		},
		Audit: permission.AuditBinding{CallID: "call-original"},
	}
	return store, gate, input, core, supervisor, worker, secret, challenge
}
func m8RulePolicy(t *testing.T, store *permission.Store, mode, effect string) {
	t.Helper()
	p := store.Policy()
	p.GlobalMode = mode
	if effect != "" {
		p.Rules = []permission.Rule{{ID: "trusted-local-echo", Tool: "supervisor_mcp/echo", Action: "call", Effect: effect, Reason: "test trusted local permission"}}
	}
	if _, err := store.ReplacePolicy(p.Revision, p); err != nil {
		t.Fatal(err)
	}
}
func TestSupervisorM8PermissionGateAskApprovalOnceAndSettlement(t *testing.T) {
	store, gate, input, core, supervisor, worker, key, challenge := m8CoreTestSetup(t)
	m8RulePolicy(t, store, permission.Rules, "")
	ctx := context.Background()
	args := map[string]any{"text": "hello"}
	frame, decision, err := AdmitAndSignSupervisorM8Call(ctx, input, key, challenge, core, supervisor, worker, 1, "echo", args)
	if !errors.Is(err, ErrSupervisorM8NotAllowed) || frame.MAC != "" ||
		decision.Decision.Effect != permission.Ask || decision.Approval == nil ||
		decision.Approval.Status != permission.Pending || !decision.Approval.CanApproveOnce {
		t.Fatalf("Ask did not create a pending M8 approval without signed tool call: %+v %+v %v", frame, decision, err)
	}
	pending := decision.Approval
	approved, err := store.ApproveOnce(ctx, permission.Mutation{
		ApprovalID: pending.ID, ApprovalVersion: pending.Version,
		PolicyRevision: store.Policy().Revision, Actor: "trusted-desktop-confirmed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != permission.ApprovedOnce {
		t.Fatal(approved.Status)
	}
	input.Audit.CallID = "call-retry"
	frame, decision, err = AdmitAndSignSupervisorM8Call(ctx, input, key, challenge, core, supervisor, worker, 1, "echo", args)
	if err != nil || frame.Permission == nil || frame.Permission.ConsumedApprovalID != pending.ID ||
		decision.Decision.Effect != permission.Allow || decision.ConsumedApprovalID != pending.ID {
		t.Fatalf("approved exact once did not mint scoped MAC-bound permission: %+v %+v %v", frame, decision, err)
	}
	if err := SettleSupervisorM8Dispatch(ctx, gate, decision, permission.Unknown); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Approval(pending.ID)
	if err != nil || updated.DispatchOutcome != permission.Unknown {
		t.Fatalf("uncertain call not settled: %+v %v", updated, err)
	}
	// A consumed grant must never be emitted again even with a new call id.
	input.Audit.CallID = "call-again"
	another, decision, err := AdmitAndSignSupervisorM8Call(ctx, input, key, challenge, core, supervisor, worker, 2, "echo", args)
	if !errors.Is(err, ErrSupervisorM8NotAllowed) || another.MAC != "" || decision.Decision.Effect != permission.Ask {
		t.Fatalf("one-shot grant was reused: %+v %+v %v", another, decision, err)
	}
}
func TestSupervisorM8PermissionGateDenyRuleAndPrincipal(t *testing.T) {
	store, _, input, core, supervisor, worker, key, challenge := m8CoreTestSetup(t)
	m8RulePolicy(t, store, permission.Rules, permission.Deny)
	f, d, err := AdmitAndSignSupervisorM8Call(context.Background(), input, key, challenge, core, supervisor, worker, 1, "echo", map[string]any{"text": "deny"})
	if !errors.Is(err, ErrSupervisorM8NotAllowed) || f.MAC != "" || d.Decision.Effect != permission.Deny || d.Approval != nil {
		t.Fatalf("denied M8 rule dispatched: %+v %+v %v", f, d, err)
	}
	input.Binding.Principal.Authenticated = false
	f, _, err = AdmitAndSignSupervisorM8Call(context.Background(), input, key, challenge, core, supervisor, worker, 1, "echo", nil)
	if !errors.Is(err, ErrSupervisorM8NotAllowed) || f.MAC != "" {
		t.Fatal("unverified Core principal minted frame", err)
	}
}
func TestSupervisorM8PermissionGateExplicitAllowAndProofBinding(t *testing.T) {
	store, _, input, core, supervisor, worker, key, challenge := m8CoreTestSetup(t)
	m8RulePolicy(t, store, permission.Rules, permission.Allow)
	f, decision, err := AdmitAndSignSupervisorM8Call(context.Background(), input, key, challenge, core, supervisor, worker, 1, "echo", map[string]any{"text": "ok"})
	if err != nil || decision.Decision.Effect != permission.Allow ||
		!processcontrol.ValidSupervisorPermissionProof(f.Permission) ||
		f.Permission.PolicyRevision != store.Policy().Revision {
		t.Fatalf("allowed M8 policy did not mint correct scoped proof: %+v %+v %v", f, decision, err)
	}
	if f.Permission.PrincipalHash == input.Binding.Principal.ID {
		t.Fatal("raw principal leaked")
	}
	if len(f.Permission.Fingerprint) != 71 {
		t.Fatal("exact prepared request not SHA bound")
	}
	// The mandatory M8 endpoint must distinguish a legacy allowlist-only frame.
	legacy, err := processcontrol.SignSupervisorOwnedToolCallPrototype(key, challenge, core, supervisor, worker, 1, "echo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if processcontrol.ValidSupervisorPermissionProof(legacy.Permission) {
		t.Fatal("legacy frame has M8 proof")
	}
	changed := *f.Permission
	changed.PolicyRevision++
	if changed == *f.Permission {
		t.Fatal("proof mutation failed")
	}
	if _, err := os.Stat(store.StatePath()); err != nil {
		t.Fatal("permission state was not durable")
	}
}
