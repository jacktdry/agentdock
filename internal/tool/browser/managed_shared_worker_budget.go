package browser

import (
	"context"
	"sync"
	"sync/atomic"
)

// Source-only admission budget, shared explicitly by workspace owners. No
// product capacity is chosen here. Claims remain exclusive until authoritative
// bridge cleanup succeeds; uncertain cleanup never transfers ownership.
type managedSharedBrowserBudget struct {
	mu             sync.Mutex
	maxSlots, used int
	owners         map[*WorkerRegistry]*managedSharedBudgetOwnerClaim
}

// Nonzero-sized, unique identity prevents a stale release from deleting a later
// claim, even when the registry and canonical workspace are the same.
type managedSharedBudgetOwnerClaim struct {
	root string
}

func newManagedSharedBrowserBudget(maxSlots int) (*managedSharedBrowserBudget, error) {
	if maxSlots <= 0 {
		return nil, errManagedSharedUnavailable
	}
	return &managedSharedBrowserBudget{maxSlots: maxSlots, owners: make(map[*WorkerRegistry]*managedSharedBudgetOwnerClaim)}, nil
}

func (b *managedSharedBrowserBudget) claim(registry *WorkerRegistry, root string) (*managedSharedBudgetOwnerClaim, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for r, owner := range b.owners {
		if r == registry || owner.root == root {
			return nil, errManagedSharedUnavailable
		}
	}
	claim := &managedSharedBudgetOwnerClaim{root: root}
	b.owners[registry] = claim
	return claim, nil
}

// Caller must prove all bridge/registry cleanup succeeded, or that construction
// failed before any startup or lifecycle callback could be dispatched.
func (b *managedSharedBrowserBudget) releaseOwner(registry *WorkerRegistry, claim *managedSharedBudgetOwnerClaim) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if claim != nil && b.owners[registry] == claim {
		delete(b.owners, registry)
	}
}

type managedSharedBudgetReservation struct {
	budget    *managedSharedBrowserBudget
	once      sync.Once
	uncertain atomic.Bool
}

func (b *managedSharedBrowserBudget) reserve() (*managedSharedBudgetReservation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used >= b.maxSlots {
		return nil, errManagedSharedUnavailable
	}
	b.used++
	return &managedSharedBudgetReservation{budget: b}, nil
}

// Caller must first observe generation.done and a nil stopErr. A failed
// registry startup can have unreported launch side effects, including a nil
// session. Registry Stop success alone cannot prove those effects stopped.
func (r *managedSharedBudgetReservation) release() bool {
	if r.uncertain.Load() {
		return false
	}
	r.once.Do(func() {
		r.budget.mu.Lock()
		r.budget.used--
		r.budget.mu.Unlock()
	})
	return true
}

type managedSharedBudgetBackend struct {
	ManagedLeaseBackend
	reservation *managedSharedBudgetReservation
}

func (b *managedSharedBudgetBackend) StartManaged(ctx context.Context, options ManagedWorkerOptions) (WorkerInfo, error) {
	info, err := b.ManagedLeaseBackend.StartManaged(ctx, options)
	if err != nil || info.WorkerID == "" || info.State != WorkerReady {
		b.reservation.uncertain.Store(true)
	}
	return info, err
}
