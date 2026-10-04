package browser

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

type acpBrowserLease struct {
	route browserpolicy.RouteKind
}

type acpBrowserOwner struct {
	sessionID string
	profileID string
	scope     RequestScope
	token     string
	leases    map[string]acpBrowserLease
}

// ACPBridge is the only ACP-facing browser ownership boundary. ACP adapters own
// neither chrome-devtools-mcp nor a browser process; they receive only a scoped
// HTTP MCP capability token for this bridge.
type ACPBridge struct {
	planner  *RoutePlanner
	registry *WorkerRegistry
	managed  *managedLeaseCoordinator
	external *ExternalLeaseManager

	mu        sync.Mutex
	byToken   map[string]*acpBrowserOwner
	bySession map[string]*acpBrowserOwner
	closed    bool
}

func NewACPBridge(planner *RoutePlanner, registry *WorkerRegistry) (*ACPBridge, error) {
	if planner == nil || registry == nil {
		return nil, browserError(ErrPolicyConflict, "ACP browser bridge requires planner and worker registry", "acp", nil, nil)
	}
	managed, err := newManagedLeaseCoordinator(registry, defaultManagedLeasePolicy())
	if err != nil {
		return nil, err
	}
	return &ACPBridge{
		planner: planner, registry: registry, managed: managed, external: NewExternalLeaseManager(registry),
		byToken: make(map[string]*acpBrowserOwner), bySession: make(map[string]*acpBrowserOwner),
	}, nil
}

func (b *ACPBridge) RegisterSession(sessionID, profileID, cwd string) (string, error) {
	if b == nil {
		return "", browserError(ErrEngineUnavailable, "ACP browser bridge unavailable", "acp", nil, nil)
	}
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(profileID) == "" {
		return "", browserError(ErrScopeRequired, "ACP browser session/profile identity required", "acp", nil, nil)
	}
	root, err := browserpolicy.CanonicalWorkspaceRoot(cwd)
	if err != nil {
		return "", browserError(ErrScopeRequired, "ACP browser workspace invalid", "acp", &ErrorDetails{Path: cwd}, err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return "", browserError(ErrEngineUnavailable, "ACP browser bridge is closed", "acp", nil, nil)
	}
	if existing := b.bySession[sessionID]; existing != nil {
		if existing.scope.CanonicalWorkspaceRoot != root || existing.profileID != profileID {
			return "", browserError(ErrLeaseOwnerMismatch, "ACP browser owner scope changed", "acp", &ErrorDetails{SessionID: sessionID}, nil)
		}
		return existing.token, nil
	}
	token := rand.Text()
	owner := &acpBrowserOwner{
		sessionID: sessionID, profileID: profileID, token: token,
		scope:  RequestScope{WorkspaceID: sessionID, CanonicalWorkspaceRoot: root, OwnerACPSessionID: sessionID, Provenance: ScopeACP},
		leases: make(map[string]acpBrowserLease),
	}
	b.bySession[sessionID], b.byToken[token] = owner, owner
	return token, nil
}

func (b *ACPBridge) owner(token string) (*acpBrowserOwner, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, browserError(ErrEngineUnavailable, "ACP browser bridge is closed", "acp", nil, nil)
	}
	owner := b.byToken[token]
	if owner == nil {
		return nil, browserError(ErrLeaseOwnerMismatch, "ACP browser capability token invalid", "acp", nil, nil)
	}
	return owner, nil
}

func (b *ACPBridge) Acquire(ctx context.Context, token, url string) (LeaseMetadata, error) {
	owner, err := b.owner(token)
	if err != nil {
		return LeaseMetadata{}, err
	}
	decision, err := b.planner.Resolve(ctx, owner.scope, nil)
	if err != nil {
		return LeaseMetadata{}, err
	}
	var meta LeaseMetadata
	switch decision.Route {
	case browserpolicy.RouteManaged:
		meta, _, err = b.managed.Acquire(ctx, decision.Scope, decision.Start, url)
	case browserpolicy.RouteExternal, browserpolicy.RouteRequiredExternal:
		meta, _, err = b.external.Acquire(ctx, decision, url)
	default:
		err = browserError(ErrPolicyConflict, "ACP browser route unsupported", "acp", &ErrorDetails{WorkspaceID: owner.scope.WorkspaceID}, nil)
	}
	if err != nil {
		return LeaseMetadata{}, err
	}
	meta.OwnerProfileID = owner.profileID
	b.mu.Lock()
	current := b.bySession[owner.sessionID]
	if current == nil || current.token != token || b.closed {
		b.mu.Unlock()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		_, _ = b.releaseLease(cleanupCtx, owner.scope, meta.BrowserLeaseID, decision.Route)
		return LeaseMetadata{}, browserError(ErrLeaseOwnerMismatch, "ACP browser owner disappeared during acquire", "acp", &ErrorDetails{LeaseID: meta.BrowserLeaseID}, nil)
	}
	current.leases[meta.BrowserLeaseID] = acpBrowserLease{route: decision.Route}
	b.mu.Unlock()
	return meta, nil
}

func (b *ACPBridge) leaseOwner(token, leaseID string) (*acpBrowserOwner, acpBrowserLease, error) {
	owner, err := b.owner(token)
	if err != nil {
		return nil, acpBrowserLease{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	entry, ok := owner.leases[leaseID]
	if !ok {
		return nil, acpBrowserLease{}, browserError(ErrLeaseOwnerMismatch, "ACP browser lease does not belong to capability", "acp", &ErrorDetails{LeaseID: leaseID, SessionID: owner.sessionID}, nil)
	}
	return owner, entry, nil
}

func (b *ACPBridge) Call(ctx context.Context, token, leaseID, tool string, args map[string]any) (map[string]any, error) {
	owner, entry, err := b.leaseOwner(token, leaseID)
	if err != nil {
		return nil, err
	}
	switch entry.route {
	case browserpolicy.RouteManaged:
		return b.managed.Call(ctx, owner.scope, leaseID, tool, args)
	case browserpolicy.RouteExternal, browserpolicy.RouteRequiredExternal:
		return b.external.Call(ctx, owner.scope, leaseID, tool, args)
	default:
		return nil, browserError(ErrPolicyConflict, "ACP browser lease route unsupported", "acp", &ErrorDetails{LeaseID: leaseID}, nil)
	}
}

func (b *ACPBridge) releaseLease(ctx context.Context, scope RequestScope, leaseID string, route browserpolicy.RouteKind) (LeaseMetadata, error) {
	switch route {
	case browserpolicy.RouteManaged:
		return b.managed.Release(ctx, scope, leaseID)
	case browserpolicy.RouteExternal, browserpolicy.RouteRequiredExternal:
		return b.external.Release(ctx, scope, leaseID)
	default:
		return LeaseMetadata{}, browserError(ErrPolicyConflict, "ACP browser lease route unsupported", "acp", &ErrorDetails{LeaseID: leaseID}, nil)
	}
}

func (b *ACPBridge) Release(ctx context.Context, token, leaseID string) (LeaseMetadata, error) {
	owner, entry, err := b.leaseOwner(token, leaseID)
	if err != nil {
		return LeaseMetadata{}, err
	}
	meta, releaseErr := b.releaseLease(ctx, owner.scope, leaseID, entry.route)
	if meta.CleanupState == CleanupComplete {
		b.mu.Lock()
		if current := b.bySession[owner.sessionID]; current != nil && current.token == token {
			delete(current.leases, leaseID)
		}
		b.mu.Unlock()
	}
	return meta, releaseErr
}

func (b *ACPBridge) ReleaseSession(ctx context.Context, sessionID string) error {
	b.mu.Lock()
	owner := b.bySession[sessionID]
	if owner == nil {
		b.mu.Unlock()
		return nil
	}
	leases := make(map[string]acpBrowserLease, len(owner.leases))
	for id, entry := range owner.leases {
		leases[id] = entry
	}
	b.mu.Unlock()

	var failures []error
	for leaseID, entry := range leases {
		cleanupCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		meta, err := b.releaseLease(cleanupCtx, owner.scope, leaseID, entry.route)
		cancel()
		if meta.CleanupState == CleanupComplete {
			b.mu.Lock()
			if current := b.bySession[sessionID]; current == owner {
				delete(current.leases, leaseID)
			}
			b.mu.Unlock()
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("release lease %s: %w", leaseID, err))
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	b.mu.Lock()
	if current := b.bySession[sessionID]; current == owner && len(current.leases) == 0 {
		delete(b.byToken, owner.token)
		delete(b.bySession, sessionID)
	}
	b.mu.Unlock()
	return nil
}

func (b *ACPBridge) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	sessions := make([]string, 0, len(b.bySession))
	for id := range b.bySession {
		sessions = append(sessions, id)
	}
	b.mu.Unlock()
	var failures []error
	for _, id := range sessions {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		failures = append(failures, b.ReleaseSession(ctx, id))
		cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	failures = append(failures, b.registry.Shutdown(ctx))
	return errors.Join(failures...)
}
