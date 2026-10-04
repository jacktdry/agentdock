package browser

import (
	"context"
	"crypto/rand"
	"errors"
	"maps"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ManagedLeaseBackend owns exclusive workers; it is not a dynamic MCP registry.
// Worker IDs identify immutable sessions and must never be reused on restart.
type ManagedLeaseBackend interface {
	StartManaged(context.Context, ManagedWorkerOptions) (WorkerInfo, error)
	Call(context.Context, string, string, map[string]any) (map[string]any, error)
	Stop(string) error
}

var _ ManagedLeaseBackend = (*WorkerRegistry)(nil)

type managedLease struct {
	mu       sync.Mutex // Held across backend calls, including release.
	metadata LeaseMetadata
	binding  EngineBinding
	pageID   float64
}

// ManagedLeaseManager does not reuse workers between leases. No public adapters,
// expiration, queueing or concurrency limits are installed at this boundary.
type ManagedLeaseManager struct {
	mu      sync.Mutex
	leases  map[string]*managedLease
	backend ManagedLeaseBackend
}

func NewManagedLeaseManager(backend ManagedLeaseBackend) *ManagedLeaseManager {
	return &ManagedLeaseManager{backend: backend, leases: make(map[string]*managedLease)}
}
func leaseError(code, id, reason string, cause error) error {
	return browserError(code, "managed browser lease rejected", "lease", &ErrorDetails{LeaseID: id, Reason: reason}, cause)
}
func validLeaseScope(s RequestScope) bool {
	return strings.TrimSpace(s.WorkspaceID) != "" && filepath.IsAbs(s.CanonicalWorkspaceRoot) && filepath.Clean(s.CanonicalWorkspaceRoot) == s.CanonicalWorkspaceRoot &&
		(s.Provenance == ScopeRuntime || s.Provenance == ScopeUpstream || s.Provenance == ScopeACP) &&
		(strings.TrimSpace(s.OwnerTaskID) != "" || strings.TrimSpace(s.OwnerSessionID) != "" || strings.TrimSpace(s.OwnerACPSessionID) != "")
}
func (l *managedLease) checkOwner(s RequestScope) error {
	if !validLeaseScope(s) || s != l.metadata.Scope {
		return leaseError(ErrLeaseOwnerMismatch, l.metadata.BrowserLeaseID, "trusted owner scope differs", nil)
	}
	return nil
}

func (m *ManagedLeaseManager) Acquire(ctx context.Context, scope RequestScope, start ResolvedStart, url string) (LeaseMetadata, EngineBinding, error) {
	if !validLeaseScope(scope) {
		return LeaseMetadata{}, EngineBinding{}, leaseError(ErrScopeRequired, "", "trusted workspace and owner required", nil)
	}
	owned := ResourceOwnership{Process: OwnerAgentDockIsolated, Profile: OwnerAgentDockIsolated, Connector: OwnerAgentDockIsolated}
	if start.Browser != BrowserChrome || start.Engine != EngineChromeDevToolsMCP || start.EngineVersion != PreferredEngineVersion || start.ProfileClass != ProfileIsolated || !start.Headless || !start.BackgroundPage || start.ForegroundPolicy != ForegroundForbidden || start.LifecyclePolicy != LifecycleOwned || start.Ownership != owned || start.ConnectorID != "" || start.ProfileID != "" || start.Endpoint != "" || start.ProfilePath != "" {
		return LeaseMetadata{}, EngineBinding{}, leaseError(ErrPolicyConflict, "", "resolved start is not an exclusive managed route", nil)
	}
	worker, err := m.backend.StartManaged(ctx, ManagedWorkerOptions{Cwd: scope.CanonicalWorkspaceRoot})
	fail := func(err error) (LeaseMetadata, EngineBinding, error) {
		if worker.WorkerID != "" {
			err = errors.Join(err, m.backend.Stop(worker.WorkerID))
		}
		return LeaseMetadata{}, EngineBinding{}, err
	}
	if err != nil {
		return fail(err)
	}
	if worker.WorkerID == "" || worker.State != WorkerReady {
		return fail(leaseError(ErrLeaseTargetMismatch, "", "ready worker identity missing", nil))
	}
	id, session, isolation := rand.Text(), rand.Text(), "agentdock-"+rand.Text()
	if url == "" {
		url = "about:blank"
	}
	result, err := m.backend.Call(ctx, worker.WorkerID, "new_page", map[string]any{"url": url, "background": true, "isolatedContext": isolation})
	if err == nil {
		err = leaseCallResultError(result)
	}
	if err != nil {
		return fail(err)
	}
	pageID, pageString, err := isolatedPageID(result, isolation)
	if err != nil {
		return fail(leaseError(ErrLeaseTargetMismatch, id, "isolated page unavailable", err))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	now := time.Now().UTC()
	metadata := LeaseMetadata{BrowserLeaseID: id, BrowserSessionID: session, Scope: scope, WorkerID: worker.WorkerID, IsolationContextName: isolation, PageID: pageString, OwnerType: OwnerAgentDockIsolated, ProfileClass: ProfileIsolated, Ownership: owned, ForegroundPolicy: ForegroundForbidden, LifecyclePolicy: LifecycleOwned, CreatedAt: now, LastActiveAt: now, CleanupState: CleanupPending, ConnectorPID: PIDObservation{PID: worker.Session.PID, ObservedAt: now}}
	binding := EngineBinding{BrowserLeaseID: id, BrowserSessionID: session, WorkerID: worker.WorkerID, IsolationContextName: isolation, PageID: pageString, Engine: EngineChromeDevToolsMCP}
	m.mu.Lock()
	m.leases[id] = &managedLease{metadata: metadata, binding: binding, pageID: pageID}
	m.mu.Unlock()
	return metadata, binding, nil
}
func (m *ManagedLeaseManager) lease(id string) (*managedLease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.leases[id]
	if l == nil {
		return nil, leaseError(ErrLeaseNotFound, id, "unknown lease", nil)
	}
	return l, nil
}
func (m *ManagedLeaseManager) Call(ctx context.Context, scope RequestScope, id, tool string, args map[string]any) (map[string]any, error) {
	l, err := m.lease(id)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.checkOwner(scope); err != nil {
		return nil, err
	}
	if l.metadata.CleanupState != CleanupPending {
		return nil, leaseError(ErrLeaseStateInvalid, id, "lease is not active", nil)
	}
	switch tool {
	case "navigate_page", "take_snapshot", "take_screenshot", "evaluate_script", "click", "fill", "press_key":
	default:
		return nil, leaseError(ErrActionInvalid, id, "tool outside lease operation contract", nil)
	}
	if _, exists := args["pageId"]; exists {
		return nil, leaseError(ErrLeaseTargetMismatch, id, "caller pageId forbidden", nil)
	}
	targetArgs := maps.Clone(args)
	if targetArgs == nil {
		targetArgs = make(map[string]any)
	}
	targetArgs["pageId"] = l.pageID
	result, err := m.backend.Call(ctx, l.binding.WorkerID, tool, targetArgs)
	if err == nil {
		err = leaseCallResultError(result)
	}
	if err == nil {
		l.metadata.LastActiveAt = time.Now().UTC()
	}
	return result, err
}

// Stop is authoritative cleanup. A failed close_page is diagnostic only when
// Stop succeeds; failed worker cleanup returns an error and CleanupFailed.
func (m *ManagedLeaseManager) Release(ctx context.Context, scope RequestScope, id string) (LeaseMetadata, error) {
	l, err := m.lease(id)
	if err != nil {
		return LeaseMetadata{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.checkOwner(scope); err != nil {
		return LeaseMetadata{}, err
	}
	if l.metadata.CleanupState == CleanupComplete {
		return l.metadata, nil
	}
	if l.metadata.CleanupState == CleanupFailed {
		return l.metadata, leaseError(ErrActionFailed, id, l.metadata.CleanupReason, errors.New(l.metadata.CleanupError))
	}
	if l.metadata.CleanupState != CleanupPending {
		return l.metadata, leaseError(ErrLeaseStateInvalid, id, "lease cannot release", nil)
	}
	l.metadata.CleanupState = CleanupReleasing
	result, closeErr := m.backend.Call(ctx, l.binding.WorkerID, "close_page", map[string]any{"pageId": l.pageID})
	if closeErr == nil {
		closeErr = leaseCallResultError(result)
	}
	stopErr := m.backend.Stop(l.binding.WorkerID)
	l.metadata.CleanupState = CleanupComplete
	l.metadata.CleanupReason = "exclusive managed worker stopped"
	if diagnostic := errors.Join(closeErr, stopErr); diagnostic != nil {
		l.metadata.CleanupError = diagnostic.Error()
	}
	if stopErr != nil {
		l.metadata.CleanupState = CleanupFailed
		l.metadata.CleanupReason = "exclusive managed worker cleanup failed"
		return l.metadata, leaseError(ErrActionFailed, id, l.metadata.CleanupReason, stopErr)
	}
	return l.metadata, nil
}
func leaseCallResultError(result map[string]any) error {
	if failed, _ := result["isError"].(bool); failed {
		return leaseError(ErrActionFailed, "", "MCP tool reported failure", nil)
	}
	return nil
}
