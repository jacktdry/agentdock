//go:build darwin

package client

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/permission"
)

func nextM8Fixture(t *testing.T, policyEffect string) (*NextStdioRegistryGate, NextRegistryLease, *permission.Store, NextStdioM8Context) {
	t.Helper()
	_, _, bridge, rev, gen := p5NextGateFixture(t)
	lease, err := bridge.acquireChecked(context.Background(), "trusted-session-one", "next-p5-echo", rev, gen, NextLeaseEphemeral, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := bridge.Release(context.Background(), lease); err != nil {
			t.Errorf("owned cleanup: %v", err)
		}
	})
	store, err := permission.NewStore(t.TempDir(), "core-epoch-42")
	if err != nil {
		t.Fatal(err)
	}
	gate, err := permission.NewAdmissionGate(store)
	if err != nil {
		t.Fatal(err)
	}
	pol := store.Policy()
	pol.GlobalMode = permission.Rules
	if policyEffect != "" {
		pol.Rules = []permission.Rule{{ID: "native-echo-rule", Tool: "next_stdio_mcp/next-p5-echo/echo", Action: "call", Effect: policyEffect, Reason: "test policy"}}
	}
	if _, err := store.ReplacePolicy(pol.Revision, pol); err != nil {
		t.Fatal(err)
	}
	auth := NextStdioM8Context{CoreSupervisorM8Context: CoreSupervisorM8Context{Gate: gate, Binding: permission.PermissionBinding{
		Principal:    permission.AuthorizationPrincipal{Kind: "core-auth", ID: "trusted-user", Authenticated: true, Stable: true},
		RuntimeEpoch: store.RuntimeEpoch(), ACPSessionID: "trusted-session-one", Source: "core-acp", Provider: "next-stdio-mcp",
	}, Audit: permission.AuditBinding{CallID: "call-1"}}}
	return bridge, lease, store, auth
}

func TestNextStdioM8ExplicitAllowAndDeny(t *testing.T) {
	bridge, lease, store, auth := nextM8Fixture(t, permission.Allow)
	got, admission, err := bridge.CallAdmitted(context.Background(), auth, lease, "echo", map[string]any{"text": "trusted"})
	if err != nil || admission.Decision.Effect != permission.Allow || got["structuredContent"].(map[string]any)["echo"] != "trusted" {
		t.Fatalf("allow result: %+v %+v %v", got, admission, err)
	}
	pol := store.Policy()
	pol.Rules = []permission.Rule{{ID: "blocked-native-echo", Tool: "next_stdio_mcp/next-p5-echo/echo", Effect: permission.Deny}}
	if _, err := store.ReplacePolicy(pol.Revision, pol); err != nil {
		t.Fatal(err)
	}
	result, decision, err := bridge.CallAdmitted(context.Background(), auth, lease, "echo", map[string]any{"text": "denied"})
	if !errors.Is(err, ErrNextM8NotAllowed) || result != nil || decision.Decision.Effect != permission.Deny {
		t.Fatalf("M8 denied after allow: %+v %+v %v", result, decision, err)
	}
}

func TestNextStdioM8AskApproveOnceAndSettle(t *testing.T) {
	bridge, lease, store, auth := nextM8Fixture(t, "")
	args := map[string]any{"text": "requires-approval"}
	result, decision, err := bridge.CallAdmitted(context.Background(), auth, lease, "echo", args)
	if !errors.Is(err, ErrNextM8NotAllowed) || result != nil || decision.Decision.Effect != permission.Ask || decision.Approval == nil || !decision.Approval.CanApproveOnce {
		t.Fatalf("ask did not stop dispatch: %+v %+v %v", result, decision, err)
	}
	approved, err := store.ApproveOnce(context.Background(), permission.Mutation{ApprovalID: decision.Approval.ID, ApprovalVersion: decision.Approval.Version, PolicyRevision: store.Policy().Revision, Actor: "test-approver"})
	if err != nil || approved.Status != permission.ApprovedOnce {
		t.Fatalf("approval once: %+v %v", approved, err)
	}
	auth.Audit.CallID = "call-2"
	result, decision, err = bridge.CallAdmitted(context.Background(), auth, lease, "echo", args)
	if err != nil || result["structuredContent"].(map[string]any)["echo"] != "requires-approval" || decision.ConsumedApprovalID != approved.ID {
		t.Fatalf("approved-once no dispatch: %+v %+v %v", result, decision, err)
	}
	stored, err := store.Approval(approved.ID)
	if err != nil || stored.DispatchOutcome != permission.Succeeded {
		t.Fatalf("one-shot not settled: %+v %v", stored, err)
	}
	auth.Audit.CallID = "call-3"
	result, decision, err = bridge.CallAdmitted(context.Background(), auth, lease, "echo", args)
	if result != nil || !errors.Is(err, ErrNextM8NotAllowed) || decision.Decision.Effect != permission.Ask {
		t.Fatalf("one-shot replay accepted: %+v %+v %v", result, decision, err)
	}
}

func TestNextStdioM8UntrustedIdentityAndMalformedArgs(t *testing.T) {
	bridge, lease, _, auth := nextM8Fixture(t, permission.Allow)
	original := auth
	cases := []struct {
		name   string
		mutate func(*NextStdioM8Context)
	}{
		{"untrusted principal", func(a *NextStdioM8Context) { a.Binding.Principal.Authenticated = false }},
		{"wrong ACP session", func(a *NextStdioM8Context) { a.Binding.ACPSessionID = "another-session" }},
		{"wrong epoch", func(a *NextStdioM8Context) { a.Binding.RuntimeEpoch = "old-epoch" }},
		{"missing gate", func(a *NextStdioM8Context) { a.Gate = nil }},
		{"missing audit", func(a *NextStdioM8Context) { a.Audit.CallID = "" }},
	}
	for _, tc := range cases {
		a := original
		tc.mutate(&a)
		if got, _, err := bridge.CallAdmitted(context.Background(), a, lease, "echo", map[string]any{"text": "no"}); got != nil || !errors.Is(err, ErrNextM8NotAllowed) {
			t.Fatalf("%s admitted: %+v %v", tc.name, got, err)
		}
	}
	for _, args := range []map[string]any{{"text": make(chan int)}, {"text": strings.Repeat("x", 70<<10)}} {
		if result, _, err := bridge.CallAdmitted(context.Background(), original, lease, "echo", args); result != nil || !errors.Is(err, ErrNextM8NotAllowed) {
			t.Fatalf("bad args admitted: %+v %v", result, err)
		}
	}
	if raw, err := json.Marshal(original); err == nil || raw != nil {
		t.Fatal("trusted Core auth context serialized")
	}
}

func TestNextStdioM8StartRequiresPermissionBeforeSpawn(t *testing.T) {
	_, ctl, g, rev, gen := p5NextGateFixture(t)
	store, err := permission.NewStore(t.TempDir(), "next-start-epoch")
	if err != nil {
		t.Fatal(err)
	}
	gate, err := permission.NewAdmissionGate(store)
	if err != nil {
		t.Fatal(err)
	}
	policy := store.Policy()
	policy.GlobalMode = permission.Rules
	if _, err := store.ReplacePolicy(policy.Revision, policy); err != nil {
		t.Fatal(err)
	}
	auth := NextStdioM8Context{CoreSupervisorM8Context: CoreSupervisorM8Context{Gate: gate, Binding: permission.PermissionBinding{
		Principal:    permission.AuthorizationPrincipal{Kind: "verified-core", ID: "user-1", Authenticated: true, Stable: true},
		RuntimeEpoch: store.RuntimeEpoch(), ACPSessionID: "session-start", Source: "next-core",
	}, Audit: permission.AuditBinding{CallID: "start-call-1"}}}
	started, denied, err := g.AcquireAdmitted(context.Background(), auth, "session-start", "next-p5-echo", rev, gen, NextLeaseEphemeral, 0)
	if !errors.Is(err, ErrNextM8NotAllowed) || started.lease.id != "" || denied.Decision.Effect != permission.Ask || denied.Approval == nil || ctl.Snapshot().Managed != 0 {
		t.Fatalf("Ask spawned child: %+v %+v %v", started, denied, err)
	}
	if _, err := store.ApproveOnce(context.Background(), permission.Mutation{ApprovalID: denied.Approval.ID, ApprovalVersion: denied.Approval.Version, PolicyRevision: store.Policy().Revision, Actor: "trusted-confirmation"}); err != nil {
		t.Fatal(err)
	}
	auth.Audit.CallID = "start-call-2"
	started, admitted, err := g.AcquireAdmitted(context.Background(), auth, "session-start", "next-p5-echo", rev, gen, NextLeaseEphemeral, 0)
	if err != nil || started.lease.id == "" || admitted.Decision.Effect != permission.Allow || admitted.ConsumedApprovalID != denied.Approval.ID || ctl.Snapshot().Ready != 1 {
		t.Fatalf("approved launch absent: %+v %+v %v", started, admitted, err)
	}
	record, err := store.Approval(denied.Approval.ID)
	if err != nil || record.DispatchOutcome != permission.Succeeded {
		t.Fatalf("approved startup not settled: %+v %v", record, err)
	}
	if err := g.Release(context.Background(), started); err != nil {
		t.Fatal(err)
	}
	if ctl.Snapshot().Managed != 0 {
		t.Fatal("approved owned process not waited")
	}
	auth.Audit.CallID = "start-call-3"
	if again, decision, err := g.AcquireAdmitted(context.Background(), auth, "session-start", "next-p5-echo", rev, gen, NextLeaseEphemeral, 0); !errors.Is(err, ErrNextM8NotAllowed) || again.lease.id != "" || decision.Decision.Effect != permission.Ask || ctl.Snapshot().Managed != 0 {
		t.Fatal("approved-once startup replayed")
	}
	policy = store.Policy()
	policy.Rules = []permission.Rule{{ID: "hard-deny-start", Tool: "next_stdio_mcp/next-p5-echo/start", Effect: permission.Deny}}
	if _, err := store.ReplacePolicy(policy.Revision, policy); err != nil {
		t.Fatal(err)
	}
	auth.Audit.CallID = "start-call-4"
	if _, decision, err := g.AcquireAdmitted(context.Background(), auth, "session-start", "next-p5-echo", rev, gen, NextLeaseEphemeral, 0); !errors.Is(err, ErrNextM8NotAllowed) || decision.Decision.Effect != permission.Deny || ctl.Snapshot().Managed != 0 {
		t.Fatal("explicit Deny started MCP")
	}
	auth.Binding.ACPSessionID = "wrong-session"
	if _, _, err := g.AcquireAdmitted(context.Background(), auth, "session-start", "next-p5-echo", rev, gen, NextLeaseEphemeral, 0); !errors.Is(err, ErrNextM8NotAllowed) {
		t.Fatal("forged ACP session owned child")
	}
}
