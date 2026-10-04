package browser

import (
	"context"
	"slices"
	"strings"
	"time"
)

type resourceLockPolicy struct {
	QueueCapacity int
	QueueTimeout  time.Duration
}

func defaultResourceLockPolicy() resourceLockPolicy {
	// Bound contention independently from browser admission; no holder TTL or
	// implicit stealing. Callers must release or later lifecycle recovery owns it.
	return resourceLockPolicy{16, 30 * time.Second}
}

type resourceLocks struct{ queue *admissionQueue }
type resourceLock struct {
	owner  RequestScope
	permit *admissionWaiter
}

func newResourceLocks(policy resourceLockPolicy) (*resourceLocks, error) {
	if policy.QueueCapacity < 0 || policy.QueueTimeout <= 0 {
		return nil, leaseError(ErrPolicyConflict, "", "invalid resource lock policy", nil)
	}
	return &resourceLocks{queue: newAdmissionQueue(0, policy.QueueCapacity, policy.QueueTimeout)}, nil
}

// Acquire atomically holds all canonical keys. Keys are caller-defined exact
// site/resource identities (e.g. site:cms, resource:cms:article-A); no URL or
// resource hierarchy is inferred. Strict FIFO may block disjoint later waiters.
func (l *resourceLocks) Acquire(ctx context.Context, owner RequestScope, keys ...string) (*resourceLock, error) {
	if !validLeaseScope(owner) {
		return nil, leaseError(ErrScopeRequired, "", "trusted lock owner required", nil)
	}
	canonical := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, leaseError(ErrActionInvalid, "", "empty resource lock key", nil)
		}
		canonical = append(canonical, key)
	}
	if len(canonical) == 0 {
		return nil, leaseError(ErrActionInvalid, "", "resource lock keys required", nil)
	}
	slices.Sort(canonical)
	canonical = slices.Compact(canonical)
	permit, err := l.queue.acquire(ctx, canonical)
	if err != nil {
		return nil, err
	}
	return &resourceLock{owner: owner, permit: permit}, nil
}

// Release is idempotent for the exact trusted owner, including after release.
// Possession of the handle alone never bypasses owner verification.
func (l *resourceLock) Release(owner RequestScope) error {
	if !validLeaseScope(owner) || owner != l.owner {
		return leaseError(ErrLockOwnerMismatch, "", "trusted lock owner differs", nil)
	}
	l.permit.release()
	return nil
}
