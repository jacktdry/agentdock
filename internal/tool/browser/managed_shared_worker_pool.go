package browser

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// One private owner per workspace/registry. The owner gate covers admission,
// page operations and retirement. Each registry adapter pins exactly one
// generation forever; it is never repointed while an old call can finish.
type managedSharedRotatingBackend struct {
	registry   *WorkerRegistry
	options    ManagedWorkerOptions
	gate       chan struct{}
	closed     atomic.Bool
	generation *managedSharedWorkerBackend
	epoch      uint64
	borrows    map[string]string
	// Only the most recently retired generation's bounded tokens are kept for
	// cleanup recovery. Older/unknown tokens fail closed, never touch a worker.
	retired     map[string]string
	budget      *managedSharedBrowserBudget
	reservation *managedSharedBudgetReservation
}

var _ ManagedLeaseBackend = (*managedSharedRotatingBackend)(nil)

func (*managedSharedRotatingBackend) sharedManagedBorrowCleanup() {}

func newManagedSharedRotatingBackend(registry *WorkerRegistry, options ManagedWorkerOptions) *managedSharedRotatingBackend {
	b := &managedSharedRotatingBackend{registry: registry, options: options, gate: make(chan struct{}, 1)}
	b.gate <- struct{}{}
	return b
}

func (b *managedSharedRotatingBackend) lock(ctx context.Context) error {
	if ctx.Err() != nil {
		return errManagedSharedUnavailable
	}
	select {
	case <-ctx.Done():
		return errManagedSharedUnavailable
	case <-b.gate:
		return nil
	}
}
func (b *managedSharedRotatingBackend) unlock() { b.gate <- struct{}{} }

func (b *managedSharedRotatingBackend) StartManaged(ctx context.Context, options ManagedWorkerOptions) (WorkerInfo, error) {
	if err := b.lock(ctx); err != nil {
		return WorkerInfo{}, err
	}
	defer b.unlock()
	if b.closed.Load() || b.registry == nil || options != b.options {
		return WorkerInfo{}, errManagedSharedUnavailable
	}
	if g := b.generation; g != nil {
		g.mu.Lock()
		closed := g.closed
		g.mu.Unlock()
		if closed {
			// Deadline/cancellation never proves Stop completed. Keep the old
			// generation pinned until its finalizer explicitly succeeds.
			select {
			case <-g.done:
				g.mu.Lock()
				failed := g.stopErr != nil
				g.mu.Unlock()
				if failed {
					return WorkerInfo{}, errManagedSharedUnavailable
				}
				if !b.releaseBudget() {
					return WorkerInfo{}, errManagedSharedUnavailable
				}
				b.retired, b.borrows, b.generation = b.borrows, nil, nil
			default:
				return WorkerInfo{}, errManagedSharedUnavailable
			}
		}
	}
	if b.generation == nil {
		if b.epoch == ^uint64(0) {
			return WorkerInfo{}, errManagedSharedUnavailable
		}
		adapter := &sharedRegistryBackend{WorkerRegistry: b.registry}
		var backend ManagedLeaseBackend = adapter
		if b.budget != nil {
			reservation, err := b.budget.reserve()
			if err != nil {
				return WorkerInfo{}, errManagedSharedUnavailable
			}
			b.reservation = reservation
			backend = &managedSharedBudgetBackend{ManagedLeaseBackend: adapter, reservation: reservation}
		}
		g, err := newManagedSharedWorkerBackend(backend, b.options)
		if err != nil {
			b.releaseBudget() // No StartManaged has been dispatched.
			return WorkerInfo{}, errManagedSharedUnavailable
		}
		adapter.shared = g
		b.epoch++
		b.generation, b.borrows = g, make(map[string]string)
	}
	info, err := b.generation.StartManaged(ctx, options)
	if err != nil {
		return WorkerInfo{}, errManagedSharedUnavailable
	}
	id := fmt.Sprintf("%d:%s", b.epoch, info.WorkerID)
	b.borrows[id] = info.WorkerID
	info.WorkerID = id
	if b.closed.Load() {
		// Shutdown owns terminal cleanup; do not publish a late borrow.
		return WorkerInfo{}, errManagedSharedUnavailable
	}
	return info, nil
}

func (b *managedSharedRotatingBackend) Call(ctx context.Context, id, tool string, args map[string]any) (map[string]any, error) {
	if err := b.lock(ctx); err != nil {
		return nil, err
	}
	defer b.unlock()
	borrow := b.borrows[id]
	if b.closed.Load() || borrow == "" {
		return nil, errManagedSharedUnavailable
	}
	result, err := b.generation.Call(ctx, borrow, tool, args)
	if err != nil || b.closed.Load() {
		return nil, errManagedSharedUnavailable
	}
	return result, nil
}

func (b *managedSharedRotatingBackend) Stop(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.lock(ctx); err != nil {
		return err
	}
	defer b.unlock()
	if borrow := b.borrows[id]; borrow != "" {
		if err := b.generation.Stop(borrow); err != nil {
			return errManagedSharedUnavailable
		}
		return nil
	}
	if b.retired[id] != "" {
		return nil
	} // Successful retirement already cleaned this token.
	return errManagedSharedUnavailable
}

func (b *managedSharedRotatingBackend) ReapIdle(now time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.lock(ctx); err != nil {
		return err
	}
	defer b.unlock()
	if b.generation != nil {
		if err := b.generation.ReapIdle(now); err != nil {
			return errManagedSharedUnavailable
		}
		select {
		case <-b.generation.done:
			if !b.releaseBudget() {
				return errManagedSharedUnavailable
			}
		default:
		}
	}
	return nil
}

func (b *managedSharedRotatingBackend) Shutdown(ctx context.Context) error {
	b.closed.Store(true) // Revoke admission even while waiting on the owner gate.
	if err := b.lock(ctx); err != nil {
		return err
	}
	defer b.unlock()
	if b.generation != nil {
		if err := b.generation.Shutdown(ctx); err != nil {
			return errManagedSharedUnavailable
		}
		if !b.releaseBudget() {
			return errManagedSharedUnavailable
		}
	}
	return nil
}

// Owner gate held; call only after authoritative successful terminal Stop,
// or before startup has been dispatched at all.
func (b *managedSharedRotatingBackend) releaseBudget() bool {
	if b.reservation == nil {
		return true
	}
	if !b.reservation.release() {
		return false
	}
	b.reservation = nil
	return true
}
