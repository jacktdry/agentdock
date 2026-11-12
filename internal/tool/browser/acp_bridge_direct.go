package browser

import (
	"context"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
)

// The root is supplied only by Runtime configuration, never tool arguments.
// Direct actors have no ACP session/token and are admitted by Runtime.Call.
func (b *ACPBridge) directScope(ctx context.Context, root string) (RequestScope, error) {
	p, ok := requestmeta.AuthPrincipalFromContext(ctx)
	if !ok {
		return RequestScope{}, leaseError(ErrScopeRequired, "", "authenticated stable principal required", nil)
	}
	canonical, err := browserpolicy.CanonicalWorkspaceRoot(root)
	if err != nil {
		return RequestScope{}, leaseError(ErrScopeRequired, "", "configured workspace invalid", err)
	}
	if b == nil {
		return RequestScope{}, leaseError(ErrEngineUnavailable, "", "bridge unavailable", nil)
	}
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return RequestScope{}, leaseError(ErrEngineUnavailable, "", "bridge closed", nil)
	}
	// Include principal kind in the opaque identity to separate authentication domains.
	identity := requestmeta.NewStableAuthPrincipal("browser-direct", p.Kind, p.ID).ID
	return RequestScope{WorkspaceID: identity, CanonicalWorkspaceRoot: canonical, OwnerSessionID: identity, Provenance: ScopeUpstream}, nil
}

func (b *ACPBridge) DirectAcquire(ctx context.Context, root, url string) (LeaseMetadata, error) {
	scope, err := b.directScope(ctx, root)
	if err != nil {
		return LeaseMetadata{}, err
	}
	// Plan is passive; company routes require the Core-owned native runtime.
	plan, err := b.planner.Plan(scope, nil)
	if err != nil {
		return LeaseMetadata{}, err
	}
	if plan.Route == browserpolicy.RouteManaged && ((b.shared != nil && scope.CanonicalWorkspaceRoot != b.shared.options.Cwd) || (b.rotating != nil && scope.CanonicalWorkspaceRoot != b.rotating.options.Cwd)) {
		return LeaseMetadata{}, errManagedSharedUnavailable
	}
	if plan.Route != browserpolicy.RouteManaged && (b.planner.externalRuntime == nil || plan.Engine != EngineNativeCDP) {
		return LeaseMetadata{}, leaseError(ErrPolicyConflict, "", "direct actor requires managed route", nil)
	}
	decision, err := b.planner.Resolve(ctx, scope, nil)
	if err != nil {
		return LeaseMetadata{}, err
	}
	var meta LeaseMetadata
	if decision.Route == browserpolicy.RouteManaged {
		meta, _, err = b.managed.Acquire(ctx, decision.Scope, decision.Start, url)
	} else {
		meta, _, err = b.external.Acquire(ctx, decision, url)
	}
	if err != nil {
		return meta, err
	}
	b.mu.Lock()
	closed := b.closed
	if !closed && ctx.Err() == nil {
		b.directLeases[meta.BrowserLeaseID] = directBrowserLease{scope: scope, route: decision.Route}
	}
	b.mu.Unlock()
	if closed || ctx.Err() != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		meta, cleanupErr := b.releaseLease(cleanupCtx, scope, meta.BrowserLeaseID, decision.Route)
		if cleanupErr != nil {
			return meta, cleanupErr
		}
		if ctx.Err() != nil {
			return meta, ctx.Err()
		}
		return meta, leaseError(ErrEngineUnavailable, "", "bridge closed during acquire", nil)
	}
	return meta, nil
}

func (b *ACPBridge) DirectCall(ctx context.Context, root, id, tool string, args map[string]any) (map[string]any, error) {
	scope, err := b.directScope(ctx, root)
	if err != nil {
		return nil, err
	}
	if tool != "evaluate_script" && tool != "take_snapshot" && tool != "navigate_page" && tool != "take_screenshot" {
		return nil, leaseError(ErrActionInvalid, id, "unsupported direct operation", nil)
	}
	entry, err := b.directLease(scope, id)
	if err != nil {
		return nil, err
	}
	if entry.route == browserpolicy.RouteManaged {
		return b.managed.Call(ctx, scope, id, tool, args)
	}
	// The company Edge direct contract is intentionally read-only. Reject a
	// forbidden action before opening a fenced CDP operation: a known policy
	// denial is not transport identity loss and must not retire a healthy
	// owned lease. Principal and exact lease ownership were verified above.
	if tool == "evaluate_script" {
		return nil, leaseError(ErrActionInvalid, id, "company Edge does not permit JavaScript evaluation", nil)
	}
	if tool == "navigate_page" {
		plan, err := b.planner.Plan(scope, nil)
		if err != nil {
			return nil, err
		}
		u, ok := args["url"].(string)
		if plan.Engine != EngineNativeCDP || plan.Route != entry.route || !ok || !plan.OriginScope.Allows(u) {
			return nil, leaseError(ErrActionInvalid, id, "company Edge URL outside approved origins", nil)
		}
	}
	return b.external.Call(ctx, scope, id, tool, args)
}

func (b *ACPBridge) DirectRelease(ctx context.Context, root, id string) (LeaseMetadata, error) {
	scope, err := b.directScope(ctx, root)
	if err != nil {
		return LeaseMetadata{}, err
	}
	entry, err := b.directLease(scope, id)
	if err != nil {
		return LeaseMetadata{}, err
	}
	return b.releaseLease(ctx, scope, id, entry.route)
}

func (b *ACPBridge) directLease(scope RequestScope, id string) (directBrowserLease, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entry, ok := b.directLeases[id]
	if !ok || entry.scope != scope {
		return directBrowserLease{}, leaseError(ErrLeaseOwnerMismatch, id, "direct lease owner or workspace mismatch", nil)
	}
	return entry, nil
}

func (b *ACPBridge) ConnectExternal(ctx context.Context, root string, disconnect bool) error {
	scope, err := b.directScope(ctx, root)
	if err != nil {
		return err
	}
	plan, err := b.planner.Plan(scope, nil)
	if err != nil {
		return err
	}
	if b.planner.externalRuntime == nil || plan.Engine != EngineNativeCDP || plan.Route != browserpolicy.RouteRequiredExternal {
		return externalPeerVerificationError()
	}
	if disconnect {
		return b.planner.externalRuntime.Disconnect(ctx, plan.ConnectorID)
	}
	return b.planner.externalRuntime.Connect(ctx, plan.ConnectorID)
}
