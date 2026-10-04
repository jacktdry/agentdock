package browser

import (
	"context"
	"errors"
	"sync"
	"time"
)

// These policies are internal construction inputs, not env/config settings.
type managedLeasePolicy struct {
	MaxConcurrency int
	QueueCapacity  int
	QueueTimeout   time.Duration
	IdleTTL        time.Duration
	CleanupTimeout time.Duration
}

func defaultManagedLeasePolicy() managedLeasePolicy {
	// Four exclusive workers per lifecycle contract. Sixteen waiting requests
	// bound burst storage; 30s avoids indefinite admission waits. Five minutes
	// idle preserves short task pauses. Cleanup matches the worker Stop budget.
	return managedLeasePolicy{4, 16, 30 * time.Second, 5 * time.Minute, 35 * time.Second}
}

type managedAcquireCleanupError struct {
	workerID string
	cause    error
}

func (e *managedAcquireCleanupError) Error() string {
	return "managed acquire cleanup failed for worker " + e.workerID + ": " + e.cause.Error()
}
func (e *managedAcquireCleanupError) Unwrap() error { return e.cause }

type coordinatedLease struct {
	scope  RequestScope
	permit *admissionWaiter
}

type orphanManagedWorker struct {
	permit         *admissionWaiter
	attempts       int
	nextRecoveryAt time.Time
	recovering     bool
}

// The coordinator owns its manager exclusively. All managed admission must use
// this boundary; the public Broker and ACP adapters are deliberately unwired.
type managedLeaseCoordinator struct {
	manager *ManagedLeaseManager
	policy  managedLeasePolicy
	queue   *admissionQueue
	mu      sync.Mutex
	active  map[string]coordinatedLease
	orphans map[string]*orphanManagedWorker
}

func newManagedLeaseCoordinator(backend ManagedLeaseBackend, policy managedLeasePolicy) (*managedLeaseCoordinator, error) {
	if backend == nil || policy.MaxConcurrency <= 0 || policy.QueueCapacity < 0 || policy.QueueTimeout <= 0 || policy.IdleTTL <= 0 || policy.CleanupTimeout <= 0 {
		return nil, leaseError(ErrPolicyConflict, "", "invalid managed admission policy", nil)
	}
	return &managedLeaseCoordinator{manager: NewManagedLeaseManager(backend), policy: policy, queue: newAdmissionQueue(policy.MaxConcurrency, policy.QueueCapacity, policy.QueueTimeout), active: make(map[string]coordinatedLease), orphans: make(map[string]*orphanManagedWorker)}, nil
}

func (c *managedLeaseCoordinator) Acquire(ctx context.Context, scope RequestScope, start ResolvedStart, url string) (LeaseMetadata, EngineBinding, error) {
	permit, err := c.queue.acquire(ctx, nil)
	if err != nil {
		return LeaseMetadata{}, EngineBinding{}, err
	}
	if err = ctx.Err(); err != nil {
		permit.release()
		return LeaseMetadata{}, EngineBinding{}, err
	}
	meta, binding, err := c.manager.Acquire(ctx, scope, start, url)
	if err != nil {
		var cleanupErr *managedAcquireCleanupError
		if errors.As(err, &cleanupErr) {
			c.mu.Lock()
			c.orphans[cleanupErr.workerID] = &orphanManagedWorker{permit: permit, nextRecoveryAt: time.Now().UTC().Add(cleanupRecoveryBaseDelay)}
			c.mu.Unlock()
		} else {
			permit.release()
		}
		return meta, binding, err
	}
	l, _ := c.manager.lease(meta.BrowserLeaseID)
	l.mu.Lock()
	l.idleTTL = c.policy.IdleTTL
	l.metadata.ExpiresAt = l.metadata.LastActiveAt.Add(l.idleTTL)
	meta = l.metadata
	l.mu.Unlock()
	c.mu.Lock()
	c.active[meta.BrowserLeaseID] = coordinatedLease{scope: scope, permit: permit}
	c.mu.Unlock()
	return meta, binding, nil
}

func (c *managedLeaseCoordinator) Call(ctx context.Context, scope RequestScope, id, tool string, args map[string]any) (map[string]any, error) {
	return c.manager.Call(ctx, scope, id, tool, args)
}

func (c *managedLeaseCoordinator) returnCapacity(meta LeaseMetadata) {
	if meta.CleanupState != CleanupComplete {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.active[meta.BrowserLeaseID]; ok {
		delete(c.active, meta.BrowserLeaseID)
		entry.permit.release()
	}
}

func (c *managedLeaseCoordinator) Release(ctx context.Context, scope RequestScope, id string) (LeaseMetadata, error) {
	meta, err := c.manager.Release(ctx, scope, id)
	c.returnCapacity(meta)
	return meta, err
}

// SweepExpired is manual and bounded by MaxConcurrency. Busy leases are skipped
// using TryLock; activity is rechecked under the same mutex as Call/Release.
// Each attempted cleanup has a bounded context, with backend Stop authoritative.
// CleanupFailed is never retried here. No timer loop or global process cleanup.
func (c *managedLeaseCoordinator) SweepExpired(now time.Time) error {
	c.mu.Lock()
	entries := make(map[string]coordinatedLease, len(c.active))
	for id, entry := range c.active {
		entries[id] = entry
	}
	c.mu.Unlock()
	var failures []error
	for id, entry := range entries {
		l, err := c.manager.lease(id)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !l.mu.TryLock() {
			continue
		}
		if l.metadata.CleanupState != CleanupPending || now.Before(l.metadata.LastActiveAt.Add(c.policy.IdleTTL)) {
			l.mu.Unlock()
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), c.policy.CleanupTimeout)
		meta, err := c.manager.releaseLocked(ctx, entry.scope, l)
		cancel()
		l.mu.Unlock()
		c.returnCapacity(meta)
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// RecoverFailed performs bounded recovery for AgentDock-owned worker cleanup
// failures. It never touches user-owned browser processes or profiles. Failed
// acquire cleanup is tracked as an orphan because no lease ID was published.
func (c *managedLeaseCoordinator) RecoverFailed(now time.Time) error {
	c.mu.Lock()
	active := make(map[string]coordinatedLease, len(c.active))
	for id, entry := range c.active {
		active[id] = entry
	}
	orphanIDs := make([]string, 0, len(c.orphans))
	for id, orphan := range c.orphans {
		if orphan.recovering || orphan.attempts >= cleanupRecoveryMaxAttempts || now.Before(orphan.nextRecoveryAt) {
			continue
		}
		orphan.recovering = true
		orphanIDs = append(orphanIDs, id)
	}
	c.mu.Unlock()

	var failures []error
	for id := range active {
		l, err := c.manager.lease(id)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !l.mu.TryLock() {
			continue
		}
		if l.metadata.CleanupState != CleanupFailed || l.cleanupRecoveryAttempts >= cleanupRecoveryMaxAttempts || now.Before(l.nextCleanupRecoveryAt) {
			l.mu.Unlock()
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), c.policy.CleanupTimeout)
		meta, recoverErr := c.manager.recoverFailedLocked(ctx, l)
		cancel()
		l.mu.Unlock()
		c.returnCapacity(meta)
		if recoverErr != nil {
			failures = append(failures, recoverErr)
		}
	}

	for _, workerID := range orphanIDs {
		recoverErr := c.manager.backend.Stop(workerID)
		c.mu.Lock()
		orphan := c.orphans[workerID]
		if orphan == nil {
			c.mu.Unlock()
			continue
		}
		orphan.recovering = false
		orphan.attempts++
		if recoverErr == nil {
			delete(c.orphans, workerID)
			permit := orphan.permit
			c.mu.Unlock()
			permit.release()
			continue
		}
		orphan.nextRecoveryAt = now.Add(cleanupRecoveryDelay(orphan.attempts + 1))
		c.mu.Unlock()
		failures = append(failures, &managedAcquireCleanupError{workerID: workerID, cause: recoverErr})
	}
	return errors.Join(failures...)
}
