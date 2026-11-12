package app

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/uvwt/agentdock/internal/execution"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	"github.com/uvwt/agentdock/internal/observability"
	"github.com/uvwt/agentdock/internal/permission"
	processcontrol "github.com/uvwt/agentdock/internal/process"
)

func supervisorRuntimeIdentities(t *testing.T) (processcontrol.BirthIdentity, processcontrol.BirthIdentity,
	processcontrol.BirthIdentity, []byte, processcontrol.SupervisorHandoffChallenge) {
	t.Helper()
	core := processcontrol.BirthIdentity{PID: 500, ParentPID: 1, StartSec: 200, StartUsec: 1, EffectiveUID: 501, ProcessGroupID: 500}
	supervisor := processcontrol.BirthIdentity{PID: 400, ParentPID: 1, StartSec: 150, StartUsec: 1, EffectiveUID: 501, ProcessGroupID: 400}
	worker := processcontrol.BirthIdentity{PID: 600, ParentPID: 400, StartSec: 201, StartUsec: 1, EffectiveUID: 501, ProcessGroupID: 600}
	secret := bytes.Repeat([]byte{0x42}, 32)
	h, err := processcontrol.NewSupervisorOwnedReadOnlyHandoff(core, supervisor, worker, secret)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := h.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	return core, supervisor, worker, secret, challenge
}

func supervisorRuntimeMCPCall(t *testing.T, r *Runtime, p requestmeta.AuthPrincipal, tool string) (context.Context, execution.Call) {
	t.Helper()
	ctx := requestmeta.WithAuthPrincipal(context.Background(), p)
	ctx = observability.WithSource(ctx, observability.SourceMCP)
	ctx, call := r.execution.Begin(ctx, execution.BeginInput{Tool: tool, Source: string(observability.SourceMCP)})
	return r.bindSupervisorM8RuntimeScope(ctx, call), call
}
func TestSupervisorM8RuntimeLivePrincipalBindingAndDenyCases(t *testing.T) {
	r := newPermissionRuntime(t)
	replaceRuntimePermissionPolicy(t, r, func(p *permission.Policy) {
		p.GlobalMode = permission.Rules
		p.Rules = []permission.Rule{{ID: "allow-exact-echo", Tool: "supervisor_mcp/echo", Action: "call", Effect: permission.Allow, Reason: "test"}}
	})
	core, supervisor, worker, key, challenge := supervisorRuntimeIdentities(t)
	principal := requestmeta.NewStableAuthPrincipal("static_bearer", "test-user")
	ctx, call := supervisorRuntimeMCPCall(t, r, principal, "mcp_tool_call")
	issue := func(c context.Context) (processcontrol.SupervisorToolCallFrame, permission.Admission, error) {
		return r.admitAndSignSupervisorM8FromRuntime(c, key, challenge, core, supervisor, worker, 1, "echo", map[string]any{"text": "trusted"})
	}
	frame, result, err := issue(ctx)
	if err != nil || result.Decision.Effect != permission.Allow ||
		!processcontrol.ValidSupervisorPermissionProof(frame.Permission) ||
		frame.Permission.RuntimeEpoch != r.execution.Epoch() {
		t.Fatalf("real runtime scope not admitted: %+v %+v %v", frame, result, err)
	}
	if frame.Permission.PrincipalHash == principal.ID {
		t.Fatal("principal ID included in wire proof")
	}
	negatives := []struct {
		name string
		ctx  context.Context
	}{
		{"no-runtime-marker", requestmeta.WithAuthPrincipal(execution.WithScope(context.Background(), execution.Scope{CallID: call.ID}), principal)},
		{"missing-principal", execution.WithScope(context.Background(), execution.Scope{CallID: call.ID})},
		{"principal-changed", requestmeta.WithAuthPrincipal(ctx, requestmeta.NewStableAuthPrincipal("static_bearer", "other-user"))},
		{"mismatched-call-id", execution.WithScope(ctx, execution.Scope{CallID: "forged-id"})},
		{"mismatched-source", observability.WithSource(ctx, observability.SourceNexus)},
	}
	for _, tc := range negatives {
		t.Run(tc.name, func(t *testing.T) {
			f, _, err := issue(tc.ctx)
			if !errors.Is(err, mcpclient.ErrSupervisorM8NotAllowed) || f.MAC != "" {
				t.Fatalf("invalid call scope minted tool permission: %+v %v", f, err)
			}
		})
	}
	other := newPermissionRuntime(t)
	if f, _, err := other.admitAndSignSupervisorM8FromRuntime(ctx, key, challenge, core, supervisor, worker, 1, "echo", nil); !errors.Is(err, mcpclient.ErrSupervisorM8NotAllowed) || f.MAC != "" {
		t.Fatal("execution from another Runtime epoch admitted")
	}
	if _, ok := r.execution.Finish(call.ID, execution.FinishInput{Status: execution.StatusCompleted}); !ok {
		t.Fatal("cannot finish simulated Core call")
	}
	if f, _, err := issue(ctx); !errors.Is(err, mcpclient.ErrSupervisorM8NotAllowed) || f.MAC != "" {
		t.Fatal("finished execution scope reused")
	}
}

func TestSupervisorM8RuntimeOnlyRealMCPCallAndPrincipal(t *testing.T) {
	r := newPermissionRuntime(t)
	replaceRuntimePermissionPolicy(t, r, func(p *permission.Policy) {
		p.GlobalMode = permission.Full
	})
	core, supervisor, worker, key, challenge := supervisorRuntimeIdentities(t)
	principal := requestmeta.NewStableAuthPrincipal("acp_bridge", "test-session", "agent")
	for _, tc := range []struct {
		name      string
		tool      string
		principal requestmeta.AuthPrincipal
	}{
		{"not-an-mcp-tool", "read_file", principal},
		{"unauthenticated", "mcp_tool_call", requestmeta.AuthPrincipal{Kind: "adversary", ID: "1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := supervisorRuntimeMCPCall(t, r, tc.principal, tc.tool)
			f, _, err := r.admitAndSignSupervisorM8FromRuntime(ctx, key, challenge, core, supervisor, worker, 1, "echo", nil)
			if !errors.Is(err, mcpclient.ErrSupervisorM8NotAllowed) || f.MAC != "" {
				t.Fatalf("unsafe source minted call: %+v %v", f, err)
			}
		})
	}
}

func TestSupervisorM8RuntimeExactApproveOnceAcrossCallScope(t *testing.T) {
	r := newPermissionRuntime(t)
	replaceRuntimePermissionPolicy(t, r, func(p *permission.Policy) { p.GlobalMode = permission.Rules })
	core, supervisor, worker, key, challenge := supervisorRuntimeIdentities(t)
	principal := requestmeta.NewStableAuthPrincipal("static_bearer", "trusted-operator")
	input := map[string]any{"text": "approval-conditional"}
	emit := func(ctx context.Context, seq uint64, args map[string]any) (processcontrol.SupervisorToolCallFrame, permission.Admission, error) {
		return r.admitAndSignSupervisorM8FromRuntime(ctx, key, challenge, core, supervisor, worker, seq, "echo", args)
	}
	firstCtx, firstCall := supervisorRuntimeMCPCall(t, r, principal, "mcp_tool_call")
	first, decision, err := emit(firstCtx, 1, input)
	if !errors.Is(err, mcpclient.ErrSupervisorM8NotAllowed) || first.MAC != "" ||
		decision.Decision.Effect != permission.Ask || decision.Approval == nil {
		t.Fatalf("missing approval created signed tool: %+v %+v %v", first, decision, err)
	}
	if _, ok := r.execution.Finish(firstCall.ID, execution.FinishInput{Status: execution.StatusCompleted}); !ok {
		t.Fatal("failed to finish caller")
	}
	approved, err := r.permissions.ApproveOnce(context.Background(), permission.Mutation{
		ApprovalID: decision.Approval.ID, ApprovalVersion: decision.Approval.Version,
		PolicyRevision: r.permissions.Policy().Revision, Actor: "local-tested-desktop",
	})
	if err != nil || approved.Status != permission.ApprovedOnce {
		t.Fatalf("approve once: %+v %v", approved, err)
	}
	otherCtx, _ := supervisorRuntimeMCPCall(t, r, requestmeta.NewStableAuthPrincipal("static_bearer", "different-principal"), "mcp_tool_call")
	forbidden, otherDecision, err := emit(otherCtx, 1, input)
	if !errors.Is(err, mcpclient.ErrSupervisorM8NotAllowed) || forbidden.MAC != "" || otherDecision.Decision.Effect != permission.Ask {
		t.Fatalf("cross-principal approved once escaped: %+v %+v %v", forbidden, otherDecision, err)
	}
	modifiedCtx, _ := supervisorRuntimeMCPCall(t, r, principal, "mcp_tool_call")
	wrongArgs, wrongDecision, err := emit(modifiedCtx, 1, map[string]any{"text": "wrong-arguments"})
	if !errors.Is(err, mcpclient.ErrSupervisorM8NotAllowed) || wrongArgs.MAC != "" || wrongDecision.Decision.Effect != permission.Ask {
		t.Fatalf("changed arguments consumed approval: %+v %+v %v", wrongArgs, wrongDecision, err)
	}
	ctx, _ := supervisorRuntimeMCPCall(t, r, principal, "mcp_tool_call")
	allowed, decision, err := emit(ctx, 1, input)
	if err != nil || allowed.Permission == nil || allowed.Permission.ConsumedApprovalID != approved.ID ||
		decision.Decision.Effect != permission.Allow {
		t.Fatalf("exact eligible retry did not receive M8 signed frame: %+v %+v %v", allowed, decision, err)
	}
	if err := mcpclient.SettleSupervisorM8Dispatch(context.Background(), r.admission, decision, permission.Unknown); err != nil {
		t.Fatal(err)
	}
	nextCtx, _ := supervisorRuntimeMCPCall(t, r, principal, "mcp_tool_call")
	second, _, err := emit(nextCtx, 2, input)
	if !errors.Is(err, mcpclient.ErrSupervisorM8NotAllowed) || second.MAC != "" {
		t.Fatal("consumed retry reused")
	}
}
