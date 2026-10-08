package browser

import (
	"context"
	"crypto/rand"
	"errors"
	"maps"
	"sync"
	"time"
)

// ExternalLeaseBackend owns one immutable connector generation per lease.
type ExternalLeaseBackend interface {
	StartExternal(context.Context, ExternalWorkerOptions) (WorkerInfo, error)
	CallExternal(context.Context, WorkerInfo, string, map[string]any) (map[string]any, error)
	Stop(string) error
}

var _ ExternalLeaseBackend = (*WorkerRegistry)(nil)

func externalLeaseError(code, id, reason string, cause error) error {
	return browserError(code, "external browser lease rejected: "+reason, "lease", &ErrorDetails{LeaseID: id, Reason: reason}, cause)
}

type externalLeasePolicy struct {
	IdleTTL        time.Duration
	CleanupTimeout time.Duration
}

func defaultExternalLeasePolicy() externalLeasePolicy {
	return externalLeasePolicy{IdleTTL: 5 * time.Minute, CleanupTimeout: 35 * time.Second}
}

type externalLease struct {
	managedLease
	worker     WorkerInfo
	peer       externalPeerIdentity
	peerLost   bool
	targetLost bool
}
type externalOrphanWorker struct {
	attempts       int
	nextRecoveryAt time.Time
	recovering     bool
}

type ExternalLeaseManager struct {
	mu                   sync.Mutex
	leases               map[string]*externalLease
	orphans              map[string]*externalOrphanWorker
	backend              ExternalLeaseBackend
	policy               externalLeasePolicy
	verifier             externalPeerVerifier
	verificationInFlight chan struct{}
}

func NewExternalLeaseManager(backend ExternalLeaseBackend) *ExternalLeaseManager {
	return newExternalLeaseManager(backend, defaultExternalLeasePolicy())
}

func newExternalLeaseManager(backend ExternalLeaseBackend, policy externalLeasePolicy) *ExternalLeaseManager {
	return &ExternalLeaseManager{backend: backend, leases: make(map[string]*externalLease), orphans: make(map[string]*externalOrphanWorker), policy: policy, verificationInFlight: make(chan struct{}, 1)}
}

func (m *ExternalLeaseManager) Acquire(ctx context.Context, route RouteDecision, url string) (LeaseMetadata, EngineBinding, error) {
	scope, start := route.Scope, route.Start
	if !validLeaseScope(scope) {
		return LeaseMetadata{}, EngineBinding{}, externalLeaseError(ErrScopeRequired, "", "trusted workspace and owner required", nil)
	}
	if err := validateExternalStart(route); err != nil {
		return LeaseMetadata{}, EngineBinding{}, err
	}
	if ctx.Err() != nil || !consumeExternalRouteGrant(route) {
		return LeaseMetadata{}, EngineBinding{}, externalLeaseError(ErrRequiredRouteUnavailable, "", "external route admission unavailable", nil)
	}
	worker, err := m.backend.StartExternal(ctx, ExternalWorkerOptions{Cwd: scope.CanonicalWorkspaceRoot, Route: route})
	var pageID float64
	proven := false
	fail := func(cause error) (LeaseMetadata, EngineBinding, error) {
		if worker.WorkerID != "" {
			// Cleanup must remain possible after acquisition cancellation.
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if proven {
				if closeErr := m.closeOwnedPage(cleanupCtx, worker, pageID); closeErr != nil {
					cause = errors.Join(cause, externalLeaseError(ErrActionFailed, "", "external owned target cleanup failed", closeErr))
				}
			}
			if stopErr := m.backend.Stop(worker.WorkerID); stopErr != nil {
				m.mu.Lock()
				m.orphans[worker.WorkerID] = &externalOrphanWorker{nextRecoveryAt: time.Now().UTC().Add(cleanupRecoveryBaseDelay)}
				m.mu.Unlock()
				cause = errors.Join(cause, externalLeaseError(ErrActionFailed, "", "external connector cleanup failed", stopErr))
			}
		}
		return LeaseMetadata{}, EngineBinding{}, cause
	}
	if err != nil {
		return fail(err)
	}
	if worker.WorkerID == "" || worker.State != WorkerReady {
		return fail(externalLeaseError(ErrLeaseTargetMismatch, "", "ready worker identity missing", nil))
	}
	if err := m.verifyAdmissionPeer(ctx, route.grant, worker); err != nil {
		return fail(err)
	}
	baselineResult, err := m.backend.CallExternal(ctx, worker, "list_pages", map[string]any{})
	if err != nil {
		return fail(err)
	}
	baseline, err := externalPages(baselineResult)
	if err != nil {
		return fail(externalLeaseError(ErrLeaseTargetMismatch, "", "external baseline unavailable", err))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if url == "" {
		url = "about:blank"
	}
	if err := m.verifyAdmissionPeer(ctx, route.grant, worker); err != nil {
		return fail(err)
	}
	result, createErr := m.backend.CallExternal(ctx, worker, "new_page", map[string]any{"url": url, "background": true})
	var pageString string
	pageID, pageString, err = externalNewPageID(baseline, result)
	if err != nil {
		return fail(externalLeaseError(ErrLeaseTargetMismatch, "", "ambiguous external target cleanup: no page closed; connector stopped only", errors.Join(createErr, err)))
	}
	proven = true
	// The peer can change while new_page is executing. A page ID from the
	// old peer is not authority to close a page in the new peer. Quarantine
	// this result and stop only our connector on any identity uncertainty.
	if err := m.verifyAdmissionPeer(ctx, route.grant, worker); err != nil {
		proven = false
		return fail(err)
	}
	if createErr != nil {
		return fail(createErr)
	}
	if err := ctx.Err(); err != nil {
		proven = false
		return fail(err)
	}
	now := time.Now().UTC()
	id, session := rand.Text(), rand.Text()
	meta := LeaseMetadata{BrowserLeaseID: id, BrowserSessionID: session, Scope: scope, WorkerID: worker.WorkerID, PageID: pageString,
		OwnerType: OwnerExternalPersistent, ProfileClass: start.ProfileClass, Ownership: start.Ownership, CDPEndpoint: start.Endpoint, ProfilePath: start.ProfilePath,
		ForegroundPolicy: ForegroundForbidden, LifecyclePolicy: LifecycleExternal, CreatedAt: now, LastActiveAt: now, ExpiresAt: now.Add(m.policy.IdleTTL), CleanupState: CleanupPending,
		ConnectorPID: PIDObservation{PID: worker.Session.PID, ObservedAt: now}}
	binding := EngineBinding{BrowserLeaseID: id, BrowserSessionID: session, WorkerID: worker.WorkerID, PageID: pageString, ConnectorID: start.ConnectorID, Engine: EngineChromeDevToolsMCP}
	m.mu.Lock()
	m.leases[id] = &externalLease{managedLease: managedLease{metadata: meta, binding: binding, pageID: pageID, idleTTL: m.policy.IdleTTL}, worker: worker, peer: route.grant.peer}
	m.mu.Unlock()
	return meta, binding, nil
}

func (m *ExternalLeaseManager) lease(id string) (*externalLease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.leases[id]
	if l == nil {
		return nil, externalLeaseError(ErrLeaseNotFound, id, "unknown external lease", nil)
	}
	return l, nil
}

// verifyTarget never substitutes another page for a missing lease target.
func (m *ExternalLeaseManager) externalPagesForWorker(ctx context.Context, worker WorkerInfo) (map[float64]externalPage, error) {
	result, err := m.backend.CallExternal(ctx, worker, "list_pages", map[string]any{})
	if err != nil {
		return nil, err
	}
	if err := leaseCallResultError(result); err != nil {
		return nil, err
	}
	pages, err := externalPages(result)
	if err != nil {
		return nil, externalLeaseError(ErrLeaseTargetMismatch, "", "external page catalog invalid", err)
	}
	return pages, nil
}

// closeOwnedPage is version-pinned compatibility logic. The connector is
// stopped immediately after release, so we never repair MCP selection by
// selecting or otherwise touching a user-owned page.
func (m *ExternalLeaseManager) closeOwnedPage(ctx context.Context, worker WorkerInfo, pageID float64) error {
	result, err := m.backend.CallExternal(ctx, worker, "close_page", map[string]any{"pageId": pageID})
	if err != nil {
		return err
	}
	return externalCloseResultError(result)
}

func (m *ExternalLeaseManager) verifyTarget(ctx context.Context, l *externalLease) error {
	id := l.metadata.BrowserLeaseID
	if l.targetLost {
		return externalLeaseError(ErrLeaseTargetMismatch, id, "external target identity lost", nil)
	}
	pages, err := m.externalPagesForWorker(ctx, l.worker)
	if err != nil {
		l.recordIdentityError(err)
		return err
	}
	page, exists := pages[l.pageID]
	if err != nil || !exists || page.isolated {
		l.targetLost = true
		return externalLeaseError(ErrLeaseTargetMismatch, id, "external page missing or identity evidence invalid", err)
	}
	return nil
}

func (l *externalLease) recordIdentityError(err error) {
	var e *Error
	if errors.As(err, &e) && e.Code == ErrLeaseTargetMismatch {
		l.targetLost = true
	}
}

// Caller holds l.mu. Once lost, peer identity cannot be restored for this lease.
func (m *ExternalLeaseManager) verifyLeasePeer(ctx context.Context, l *externalLease) error {
	if l.peerLost {
		return externalLeaseError(ErrLeaseTargetMismatch, l.metadata.BrowserLeaseID, "external peer identity lost", nil)
	}
	if err := m.verifyPeer(ctx, l.peer, l.worker); err != nil {
		l.peerLost = true
		return err
	}
	return nil
}

func (m *ExternalLeaseManager) Call(ctx context.Context, scope RequestScope, id, tool string, args map[string]any) (map[string]any, error) {
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
		return nil, externalLeaseError(ErrLeaseStateInvalid, id, "lease is not active", nil)
	}
	if !safeLeasePageTool(tool) {
		return nil, externalLeaseError(ErrActionInvalid, id, "tool outside lease operation contract", nil)
	}
	if _, exists := args["pageId"]; exists {
		return nil, externalLeaseError(ErrLeaseTargetMismatch, id, "caller pageId forbidden", nil)
	}
	if err := m.verifyLeasePeer(ctx, l); err != nil {
		return nil, err
	}
	if err := m.verifyTarget(ctx, l); err != nil {
		return nil, err
	}
	targetArgs := maps.Clone(args)
	if targetArgs == nil {
		targetArgs = make(map[string]any)
	}
	targetArgs["pageId"] = l.pageID
	result, err := m.backend.CallExternal(ctx, l.worker, tool, targetArgs)
	// Recheck after the operation before releasing any data. The action may
	// already have taken effect; this is a response quarantine, not atomic
	// peer fencing or rollback. Never return a stale peer's raw results.
	if peerErr := m.verifyLeasePeer(ctx, l); peerErr != nil {
		return nil, peerErr
	}
	if err == nil {
		err = leaseCallResultError(result)
	}
	if err == nil {
		l.metadata.LastActiveAt = time.Now().UTC()
		l.metadata.ExpiresAt = l.metadata.LastActiveAt.Add(l.idleTTL)
	}
	l.recordIdentityError(err)
	return sanitizeExternalResult(result), err
}

func (m *ExternalLeaseManager) Release(ctx context.Context, scope RequestScope, id string) (LeaseMetadata, error) {
	l, err := m.lease(id)
	if err != nil {
		return LeaseMetadata{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return m.releaseLocked(ctx, scope, l)
}

func (m *ExternalLeaseManager) releaseLocked(ctx context.Context, scope RequestScope, l *externalLease) (LeaseMetadata, error) {
	id := l.metadata.BrowserLeaseID
	if err := l.checkOwner(scope); err != nil {
		return LeaseMetadata{}, err
	}
	if l.metadata.CleanupState == CleanupComplete {
		return l.metadata, nil
	}
	if l.metadata.CleanupState == CleanupFailed {
		return l.metadata, externalLeaseError(ErrActionFailed, id, l.metadata.CleanupReason, errors.New(l.metadata.CleanupError))
	}
	if l.metadata.CleanupState != CleanupPending {
		return l.metadata, externalLeaseError(ErrLeaseStateInvalid, id, "lease cannot release", nil)
	}
	l.metadata.CleanupState = CleanupReleasing
	closeErr := m.verifyLeasePeer(ctx, l)
	if closeErr == nil {
		closeErr = m.verifyTarget(ctx, l)
	}
	if closeErr == nil {
		closeErr = m.closeOwnedPage(ctx, l.worker, l.pageID)
	}
	stopErr := m.backend.Stop(l.worker.WorkerID)
	if stopErr != nil {
		err := errors.Join(closeErr, stopErr)
		l.metadata.CleanupState = CleanupFailed
		l.metadata.CleanupReason = "external target or connector cleanup failed"
		l.metadata.CleanupError = err.Error()
		l.nextCleanupRecoveryAt = time.Now().UTC().Add(cleanupRecoveryBaseDelay)
		return l.metadata, externalLeaseError(ErrActionFailed, id, l.metadata.CleanupReason, err)
	}
	l.metadata.CleanupState = CleanupComplete
	if closeErr != nil {
		l.metadata.CleanupReason = "external connector stopped; owned page close unconfirmed"
		l.metadata.CleanupError = closeErr.Error()
		return l.metadata, nil
	}
	l.metadata.CleanupReason = "owned external page closed and connector stopped"
	return l.metadata, nil
}

// SweepExpired releases only AgentDock-owned page/connector resources. The
// external browser process and profile are never cleanup authority here.
func (m *ExternalLeaseManager) SweepExpired(now time.Time) error {
	m.mu.Lock()
	leases := make([]*externalLease, 0, len(m.leases))
	for _, l := range m.leases {
		leases = append(leases, l)
	}
	m.mu.Unlock()
	var failures []error
	for _, l := range leases {
		if !l.mu.TryLock() {
			continue
		}
		if l.metadata.CleanupState != CleanupPending || now.Before(l.metadata.ExpiresAt) {
			l.mu.Unlock()
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), m.policy.CleanupTimeout)
		_, err := m.releaseLocked(ctx, l.metadata.Scope, l)
		cancel()
		l.mu.Unlock()
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// RecoverFailed retries only the AgentDock-owned connector Stop. It never
// retries page operations after cleanup identity became uncertain and never
// terminates the external browser/profile.
func (m *ExternalLeaseManager) RecoverFailed(now time.Time) error {
	m.mu.Lock()
	leases := make([]*externalLease, 0, len(m.leases))
	for _, l := range m.leases {
		leases = append(leases, l)
	}
	orphanIDs := make([]string, 0, len(m.orphans))
	for id, orphan := range m.orphans {
		if orphan.recovering || orphan.attempts >= cleanupRecoveryMaxAttempts || now.Before(orphan.nextRecoveryAt) {
			continue
		}
		orphan.recovering = true
		orphanIDs = append(orphanIDs, id)
	}
	m.mu.Unlock()
	var failures []error
	for _, l := range leases {
		if !l.mu.TryLock() {
			continue
		}
		if l.metadata.CleanupState != CleanupFailed || l.cleanupRecoveryAttempts >= cleanupRecoveryMaxAttempts || now.Before(l.nextCleanupRecoveryAt) {
			l.mu.Unlock()
			continue
		}
		l.cleanupRecoveryAttempts++
		stopErr := m.backend.Stop(l.worker.WorkerID)
		if stopErr != nil {
			l.metadata.CleanupReason = "external connector cleanup recovery failed"
			l.metadata.CleanupError = stopErr.Error()
			l.nextCleanupRecoveryAt = now.Add(cleanupRecoveryDelay(l.cleanupRecoveryAttempts + 1))
			failures = append(failures, externalLeaseError(ErrActionFailed, l.metadata.BrowserLeaseID, l.metadata.CleanupReason, stopErr))
			l.mu.Unlock()
			continue
		}
		l.metadata.CleanupState = CleanupComplete
		l.metadata.CleanupReason = "external connector cleanup recovered; external browser/profile preserved"
		if l.peerLost {
			l.metadata.CleanupReason = "external connector cleanup recovered; owned page close unconfirmed; external browser/profile preserved"
		}
		l.nextCleanupRecoveryAt = time.Time{}
		l.mu.Unlock()
	}
	for _, workerID := range orphanIDs {
		stopErr := m.backend.Stop(workerID)
		m.mu.Lock()
		orphan := m.orphans[workerID]
		if orphan == nil {
			m.mu.Unlock()
			continue
		}
		orphan.recovering = false
		orphan.attempts++
		if stopErr == nil {
			delete(m.orphans, workerID)
			m.mu.Unlock()
			continue
		}
		orphan.nextRecoveryAt = now.Add(cleanupRecoveryDelay(orphan.attempts + 1))
		m.mu.Unlock()
		failures = append(failures, externalLeaseError(ErrActionFailed, "", "external orphan connector recovery failed", stopErr))
	}
	return errors.Join(failures...)
}
