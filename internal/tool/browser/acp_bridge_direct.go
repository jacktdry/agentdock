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
	if (b.shared != nil && scope.CanonicalWorkspaceRoot != b.shared.options.Cwd) || (b.rotating != nil && scope.CanonicalWorkspaceRoot != b.rotating.options.Cwd) {
		return LeaseMetadata{}, errManagedSharedUnavailable
	}
	// Reject external/company intent before Resolve can probe a connector.
	plan, err := b.planner.Plan(scope, nil)
	if err != nil {
		return LeaseMetadata{}, err
	}
	if plan.Route != browserpolicy.RouteManaged {
		return LeaseMetadata{}, leaseError(ErrPolicyConflict, "", "direct actor requires managed route", nil)
	}
	decision, err := b.planner.Resolve(ctx, scope, nil)
	if err != nil {
		return LeaseMetadata{}, err
	}
	meta, _, err := b.managed.Acquire(ctx, decision.Scope, decision.Start, url)
	if err != nil {
		return meta, err
	}
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	if closed || ctx.Err() != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		meta, cleanupErr := b.managed.Release(cleanupCtx, scope, meta.BrowserLeaseID)
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
	if tool != "evaluate_script" && tool != "take_snapshot" {
		return nil, leaseError(ErrActionInvalid, id, "unsupported direct operation", nil)
	}
	return b.managed.Call(ctx, scope, id, tool, args)
}

func (b *ACPBridge) DirectRelease(ctx context.Context, root, id string) (LeaseMetadata, error) {
	scope, err := b.directScope(ctx, root)
	if err != nil {
		return LeaseMetadata{}, err
	}
	return b.managed.Release(ctx, scope, id)
}
