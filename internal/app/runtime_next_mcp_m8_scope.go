package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/uvwt/agentdock/internal/execution"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	"github.com/uvwt/agentdock/internal/observability"
	"github.com/uvwt/agentdock/internal/permission"
)

// nextRuntimeMCPAdmissionEnv enables only the PRIVATE Core integration seam.
// No public tool/HTTP route is wired, and no MCP process is started just by
// setting this variable. Stable AgentDock must NEVER accept the opt-in.
const nextRuntimeMCPAdmissionEnv = "AGENTDOCK_NEXT_STDIO_MCP_ADMISSION"

func (r *Runtime) nextM8ContextForLoadedACP(ctx context.Context, profileID, ownerLocalID string) (mcpclient.NextStdioM8Context, error) {
	fail := mcpclient.NextStdioM8Context{}
	if r == nil || ctx == nil || ctx.Err() != nil || r.execution == nil || r.acp == nil || r.permissions == nil || r.admission == nil || r.admission.Store() != r.permissions || r.execution.Epoch() != r.permissions.RuntimeEpoch() ||
		runtime.GOOS != "darwin" || r.cfg.Stdio || !r.cfg.ACPEnabled || os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" || os.Getenv(nextRuntimeMCPAdmissionEnv) != "source-only" {
		return fail, mcpclient.ErrNextM8NotAllowed
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" || r.cfg.AgentDockHome != filepath.Join(home, ".agentdock-next") || profileID == "" || ownerLocalID == "" {
		return fail, mcpclient.ErrNextM8NotAllowed
	}
	marker, ok := ctx.Value(supervisorM8CallKey{}).(supervisorM8CallScope)
	p, present := requestmeta.AuthPrincipalFromContext(ctx)
	scope := execution.ScopeFromContext(ctx)
	if !ok || !present || marker.runtime != r || marker.epoch != r.execution.Epoch() || marker.callID == "" || scope.CallID != marker.callID || marker.principal != p || marker.source != string(observability.SourceFromContext(ctx)) || marker.source == "" {
		return fail, mcpclient.ErrNextM8NotAllowed
	}
	call, exists := r.execution.Call(marker.callID)
	if !exists || call.Status != execution.StatusRunning || call.Tool != "mcp_tool_call" || call.Source != marker.source || call.ParentCallID != marker.parentCallID || !r.acp.IsLiveManagedSession(profileID, ownerLocalID) {
		return fail, mcpclient.ErrNextM8NotAllowed
	}
	r.lifecycleMu.RLock()
	closed := r.closing
	r.lifecycleMu.RUnlock()
	if closed {
		return fail, mcpclient.ErrNextM8NotAllowed
	}
	return mcpclient.NextStdioM8Context{CoreSupervisorM8Context: mcpclient.CoreSupervisorM8Context{
		Gate:    r.admission,
		Binding: permission.PermissionBinding{Principal: permission.AuthorizationPrincipal{Kind: p.Kind, ID: p.ID, Authenticated: p.Authenticated, Stable: p.Stable}, RuntimeEpoch: r.execution.Epoch(), Source: marker.source, ACPSessionID: ownerLocalID, Provider: "next-owned-stdio-mcp"},
		Audit:   permission.AuditBinding{CallID: call.ID, ParentCallID: call.ParentCallID},
	}}, nil
}

// nextM8CallFromRuntime is intentionally NOT a Runtime.Call handler. Only a
// future P0 dispatch owner, after a cross-process P5/M8 mutation fence and
// native MCP PID ownership proof, may route requests through this seam.
func (r *Runtime) nextM8CallFromRuntime(ctx context.Context, gate *mcpclient.NextStdioRegistryGate, profileID, ownerLocalID string, lease mcpclient.NextRegistryLease, tool string, args map[string]any) (map[string]any, permission.Admission, error) {
	if gate == nil {
		return nil, permission.Admission{}, mcpclient.ErrNextM8NotAllowed
	}
	auth, err := r.nextM8ContextForLoadedACP(ctx, profileID, ownerLocalID)
	if err != nil {
		return nil, permission.Admission{}, err
	}
	result, decision, callErr := gate.CallAdmitted(ctx, auth, lease, tool, args)
	if callErr != nil {
		return nil, decision, callErr
	}
	// Session/Runtime may close during the SDK tool call. Recheck the Core
	// generation before releasing a result; this cannot undo a side effect.
	if _, err := r.nextM8ContextForLoadedACP(ctx, profileID, ownerLocalID); err != nil {
		return nil, decision, err
	}
	return result, decision, nil
}

// nextM8AcquireFromRuntime is a Core-private admission route for ONE verified,
// currently loaded local ACP session. It is not part of Runtime.Call's public
// router; the native Rust child ownership and cross-process write fences are
// still mandatory before enabling a production route.
func (r *Runtime) nextM8AcquireFromRuntime(ctx context.Context, gate *mcpclient.NextStdioRegistryGate, profileID, ownerLocalID, server, revision, generation string, policy mcpclient.NextLeasePolicy, ttl time.Duration) (mcpclient.NextRegistryLease, permission.Admission, error) {
	if gate == nil {
		return mcpclient.NextRegistryLease{}, permission.Admission{}, mcpclient.ErrNextM8NotAllowed
	}
	auth, err := r.nextM8ContextForLoadedACP(ctx, profileID, ownerLocalID)
	if err != nil {
		return mcpclient.NextRegistryLease{}, permission.Admission{}, err
	}
	return gate.AcquireAdmitted(ctx, auth, ownerLocalID, server, revision, generation, policy, ttl)
}
