package browser

import (
	"context"
	"sync"
	"sync/atomic"
)

// Source-only admission budget, shared explicitly by workspace owners. No
// product capacity is chosen here. Claims last for the budget's lifetime:
// neither a registry nor a canonical workspace may be assigned two owners.
type managedSharedBrowserBudget struct {
	mu             sync.Mutex
	maxSlots, used int
	owners         map[*WorkerRegistry]string
}

func newManagedSharedBrowserBudget(maxSlots int) (*managedSharedBrowserBudget, error) {
	if maxSlots <= 0 {
		return nil, errManagedSharedUnavailable
	}
	return &managedSharedBrowserBudget{maxSlots: maxSlots, owners: make(map[*WorkerRegistry]string)}, nil
}

func (b *managedSharedBrowserBudget) claim(registry *WorkerRegistry, root string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for r, cwd := range b.owners {
		if r == registry || cwd == root {
			return errManagedSharedUnavailable
		}
	}
	b.owners[registry] = root
	return nil
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
