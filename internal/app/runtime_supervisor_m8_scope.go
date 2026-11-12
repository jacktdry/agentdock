package app

import (
	"context"
	"strings"

	"github.com/uvwt/agentdock/internal/execution"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	"github.com/uvwt/agentdock/internal/observability"
	"github.com/uvwt/agentdock/internal/permission"
	processcontrol "github.com/uvwt/agentdock/internal/process"
)

// supervisorM8CallKey is private to Core Runtime. A user/ACP/MCP request
// cannot supply this marker through JSON, Unix IPC, or its tool arguments.
type supervisorM8CallKey struct{}

type supervisorM8CallScope struct {
	runtime                             *Runtime
	callID, parentCallID, epoch, source string
	principal                           requestmeta.AuthPrincipal
}

// bindSupervisorM8RuntimeScope runs ONLY after Runtime.execution.Begin.
// It does not itself admit or dispatch any tool or change existing Runtime
// routing. M8 Supervisor paths cannot be activated by an external caller.
func (r *Runtime) bindSupervisorM8RuntimeScope(ctx context.Context, call execution.Call) context.Context {
	if r == nil || r.execution == nil || call.ID == "" || call.Tool != "mcp_tool_call" ||
		call.Status != execution.StatusRunning || call.Source != string(observability.SourceFromContext(ctx)) {
		return ctx
	}
	principal, ok := requestmeta.AuthPrincipalFromContext(ctx)
	if !ok {
		return ctx
	}
	return context.WithValue(ctx, supervisorM8CallKey{}, supervisorM8CallScope{
		runtime: r, callID: call.ID, parentCallID: call.ParentCallID,
		epoch: r.execution.Epoch(), source: call.Source, principal: principal,
	})
}

// admitAndSignSupervisorM8FromRuntime is a source-only Core integration seam;
// only a live Runtime.Call scope and its ORIGINAL authenticated principal may
// mint permission-qualified MCP tool frames. It does not call a Supervisor
// socket, install daemon/Browser routing or touch Chrome/AGY processes.
func (r *Runtime) admitAndSignSupervisorM8FromRuntime(ctx context.Context,
	key []byte, challenge processcontrol.SupervisorHandoffChallenge,
	core, supervisor, worker processcontrol.BirthIdentity,
	sequence uint64, tool string, args map[string]any,
) (processcontrol.SupervisorToolCallFrame, permission.Admission, error) {
	var empty processcontrol.SupervisorToolCallFrame
	if r == nil || ctx == nil || ctx.Err() != nil || r.execution == nil ||
		r.permissions == nil || r.admission == nil || r.admission.Store() != r.permissions ||
		r.execution.Epoch() != r.permissions.RuntimeEpoch() {
		return empty, permission.Admission{}, mcpclient.ErrSupervisorM8NotAllowed
	}
	marker, ok := ctx.Value(supervisorM8CallKey{}).(supervisorM8CallScope)
	principal, valid := requestmeta.AuthPrincipalFromContext(ctx)
	scope := execution.ScopeFromContext(ctx)
	if !ok || !valid || marker.runtime != r ||
		marker.callID == "" || scope.CallID != marker.callID ||
		marker.principal != principal || marker.epoch != r.execution.Epoch() ||
		marker.source != string(observability.SourceFromContext(ctx)) ||
		strings.TrimSpace(marker.source) == "" {
		return empty, permission.Admission{}, mcpclient.ErrSupervisorM8NotAllowed
	}
	call, found := r.execution.Call(marker.callID)
	if !found || call.Status != execution.StatusRunning || call.Tool != "mcp_tool_call" ||
		call.ID != marker.callID || call.ParentCallID != marker.parentCallID || call.Source != marker.source {
		return empty, permission.Admission{}, mcpclient.ErrSupervisorM8NotAllowed
	}
	binding := permission.PermissionBinding{
		Principal: permission.AuthorizationPrincipal{
			Kind: principal.Kind, ID: principal.ID, Authenticated: principal.Authenticated, Stable: principal.Stable,
		},
		RuntimeEpoch: r.execution.Epoch(),
		Source:       marker.source, Provider: "supervisor-owned-mcp",
	}
	return mcpclient.AdmitAndSignSupervisorM8Call(ctx, mcpclient.CoreSupervisorM8Context{
		Gate: r.admission, Binding: binding,
		Audit: permission.AuditBinding{CallID: call.ID, ParentCallID: call.ParentCallID},
	}, key, challenge, core, supervisor, worker, sequence, tool, args)
}
