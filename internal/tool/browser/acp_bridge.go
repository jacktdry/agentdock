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
	"github.com/uvwt/agentdock/internal/permission"
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
	planner   *RoutePlanner
	registry  *WorkerRegistry
	managed   *managedLeaseCoordinator
	external  *ExternalLeaseManager
	lifecycle *browserLifecycleRunner
	shared    *managedSharedWorkerBackend // Private opt-in; runtime constructor stays exclusive.
	rotating  *managedSharedRotatingBackend
	closeOnce sync.Once
	closeErr  error

	mu            sync.Mutex
	byToken       map[string]*acpBrowserOwner
	bySession     map[string]*acpBrowserOwner
	admissionHook permission.HostAdmissionHook
	closed        bool
}

func NewACPBridge(planner *RoutePlanner, registry *WorkerRegistry) (*ACPBridge, error) {
	return newACPBridge(planner, registry, registry, nil, nil)
}

// Source-only spike: one canonical workspace and one terminal pool generation.
// No product setting or runtime caller enables this constructor.
func newACPBridgeSharedForWorkspace(planner *RoutePlanner, registry *WorkerRegistry, cwd string) (*ACPBridge, error) {
	if planner == nil || registry == nil {
		return nil, errManagedSharedUnavailable
	}
	root, err := browserpolicy.CanonicalWorkspaceRoot(cwd)
	if err != nil {
		return nil, errManagedSharedUnavailable
	}
	adapter := &sharedRegistryBackend{WorkerRegistry: registry}
	shared, err := newManagedSharedWorkerBackend(adapter, ManagedWorkerOptions{Cwd: root})
	if err != nil {
		return nil, err
	}
	adapter.shared = shared
	return newACPBridge(planner, registry, shared, shared, nil)
}

// Source-only opt-in: the owner replaces only successfully stopped generations.
func newACPBridgeRotatingSharedForWorkspace(planner *RoutePlanner, registry *WorkerRegistry, cwd string) (*ACPBridge, error) {
	if planner == nil || registry == nil {
		return nil, errManagedSharedUnavailable
	}
	root, err := browserpolicy.CanonicalWorkspaceRoot(cwd)
	if err != nil {
		return nil, errManagedSharedUnavailable
	}
	pool := newManagedSharedRotatingBackend(registry, ManagedWorkerOptions{Cwd: root})
	return newACPBridge(planner, registry, pool, nil, pool)
}

// Each owner gets a distinct registry lifecycle; only process admission is
// shared. The caller must supply one common budget for all participating roots.
func newACPBridgeBudgetedRotatingSharedForWorkspace(planner *RoutePlanner, registry *WorkerRegistry, cwd string, budget *managedSharedBrowserBudget) (*ACPBridge, error) {
	if planner == nil || registry == nil || budget == nil {
		return nil, errManagedSharedUnavailable
	}
	root, err := browserpolicy.CanonicalWorkspaceRoot(cwd)
	if err != nil {
		return nil, errManagedSharedUnavailable
	}
	if err := budget.claim(registry, root); err != nil {
		return nil, err
	}
	pool := newManagedSharedRotatingBackend(registry, ManagedWorkerOptions{Cwd: root})
	pool.budget = budget
	return newACPBridge(planner, registry, pool, nil, pool)
}

func newACPBridge(planner *RoutePlanner, registry *WorkerRegistry, backend ManagedLeaseBackend, shared *managedSharedWorkerBackend, rotating *managedSharedRotatingBackend) (*ACPBridge, error) {
	if planner == nil || registry == nil {
		return nil, browserError(ErrPolicyConflict, "ACP browser bridge requires planner and worker registry", "acp", nil, nil)
	}
	managed, err := newManagedLeaseCoordinator(backend, defaultManagedLeasePolicy())
	if err != nil {
		return nil, err
	}
	bridge := &ACPBridge{
		planner: planner, registry: registry, managed: managed, shared: shared, rotating: rotating, external: NewExternalLeaseManager(registry),
		byToken: make(map[string]*acpBrowserOwner), bySession: make(map[string]*acpBrowserOwner),
	}
	bridge.lifecycle = newBrowserLifecycleRunner(browserLifecycleSweepInterval, bridge.sweepLifecycle)
	return bridge, nil
}

func (b *ACPBridge) SetAdmissionHook(hook permission.HostAdmissionHook) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.admissionHook = hook
	b.mu.Unlock()
}

func (b *ACPBridge) admissionFor(ctx context.Context, owner *acpBrowserOwner, action string, payload any) (permission.HostOperationFinish, error) {
	if b == nil || owner == nil {
		return nil, nil
	}
	b.mu.Lock()
	hook := b.admissionHook
	b.mu.Unlock()
	if hook == nil {
		return nil, nil
	}
	return hook(ctx, permission.HostOperation{
		Tool:          "acp_browser",
		Action:        action,
		SessionID:     owner.sessionID,
		ProfileID:     owner.profileID,
		WorkspaceRoot: owner.scope.CanonicalWorkspaceRoot,
		Payload:       payload,
	})
}

func (b *ACPBridge) sweepLifecycle(now time.Time) error {
	if b == nil {
		return nil
	}
	err := errors.Join(
		b.managed.SweepExpired(now),
		b.external.SweepExpired(now),
		b.managed.RecoverFailed(now),
		b.external.RecoverFailed(now),
	)
	if b.shared != nil {
		err = errors.Join(err, b.shared.ReapIdle(now))
	}
	if b.rotating != nil {
		err = errors.Join(err, b.rotating.ReapIdle(now))
	}
	return err
}

func (b *ACPBridge) RegisterSession(sessionID, profileID, cwd string) (string, error) {
	return b.registerSession(sessionID, profileID, cwd, "")
}

// RegisterSessionWithToken lets the AgentDock parent bind multiple host-owned
// MCP capabilities to one ACP session token without merging their tool surfaces.
func (b *ACPBridge) RegisterSessionWithToken(sessionID, profileID, cwd, token string) (string, error) {
	return b.registerSession(sessionID, profileID, cwd, token)
}

func (b *ACPBridge) registerSession(sessionID, profileID, cwd, token string) (string, error) {
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
		if existing.scope.CanonicalWorkspaceRoot != root || existing.profileID != profileID || (token != "" && existing.token != token) {
			return "", browserError(ErrLeaseOwnerMismatch, "ACP browser owner scope changed", "acp", &ErrorDetails{SessionID: sessionID}, nil)
		}
		return existing.token, nil
	}
	if token == "" {
		token = rand.Text()
	}
	if existing := b.byToken[token]; existing != nil {
		return "", browserError(ErrLeaseOwnerMismatch, "ACP browser capability token already belongs to another session", "acp", &ErrorDetails{SessionID: existing.sessionID}, nil)
	}
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

func (b *ACPBridge) Acquire(ctx context.Context, token, url string) (meta LeaseMetadata, err error) {
	owner, err := b.owner(token)
	if err != nil {
		return LeaseMetadata{}, err
	}
	if (b.shared != nil && owner.scope.CanonicalWorkspaceRoot != b.shared.options.Cwd) || (b.rotating != nil && owner.scope.CanonicalWorkspaceRoot != b.rotating.options.Cwd) {
		return LeaseMetadata{}, errManagedSharedUnavailable
	}
	finish, err := b.admissionFor(ctx, owner, "acquire", map[string]any{"url": url})
	if err != nil {
		return LeaseMetadata{}, err
	}
	if finish != nil {
		defer func() { finish(err) }()
	}
	decision, err := b.planner.Resolve(ctx, owner.scope, nil)
	if err != nil {
		return LeaseMetadata{}, err
	}
	if (b.shared != nil || b.rotating != nil) && decision.Route != browserpolicy.RouteManaged {
		return LeaseMetadata{}, errManagedSharedUnavailable
	}
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

func (b *ACPBridge) Call(ctx context.Context, token, leaseID, tool string, args map[string]any) (result map[string]any, err error) {
	owner, entry, err := b.leaseOwner(token, leaseID)
	if err != nil {
		return nil, err
	}
	finish, err := b.admissionFor(ctx, owner, tool, map[string]any{"lease_id": leaseID, "arguments": args})
	if err != nil {
		return nil, err
	}
	if finish != nil {
		defer func() { finish(err) }()
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
	b.closeOnce.Do(func() { b.closeErr = b.close() })
	return b.closeErr
}

func (b *ACPBridge) close() error {
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
	if b.lifecycle != nil {
		b.lifecycle.Close()
	}
	var failures []error
	for _, id := range sessions {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		failures = append(failures, b.ReleaseSession(ctx, id))
		cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	if b.shared != nil {
		failures = append(failures, b.shared.Shutdown(ctx))
	}
	if b.rotating != nil {
		failures = append(failures, b.rotating.Shutdown(ctx))
	}
	registryErr := b.registry.Shutdown(ctx)
	if b.rotating != nil && registryErr != nil {
		registryErr = errManagedSharedUnavailable
	}
	failures = append(failures, registryErr)
	return errors.Join(failures...)
}
