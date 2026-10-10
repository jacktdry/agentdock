package browser

import (
	"context"
	"sync"
)

// Source-only admission budget, shared explicitly by workspace owners. No
// product capacity is chosen here. Claims remain exclusive until authoritative
// bridge cleanup succeeds; uncertain cleanup never transfers ownership.
type managedSharedBrowserBudget struct {
	mu             sync.Mutex
	maxSlots, used int
	owners         map[*WorkerRegistry]*managedSharedBudgetOwnerClaim
	reservations   map[*managedSharedBudgetReservation]struct{}
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
	return &managedSharedBrowserBudget{maxSlots: maxSlots, owners: make(map[*WorkerRegistry]*managedSharedBudgetOwnerClaim), reservations: make(map[*managedSharedBudgetReservation]struct{})}, nil
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

// SharedManagedBudgetDiagnostic exposes aggregate occupancy only: never
// workspace roots, process IDs, browser profiles, lease identities or tokens.
// An uncertain startup continues to count as occupied until authoritative
// process cleanup exists; the snapshot does not infer Stop completion.
type SharedManagedBudgetDiagnostic struct {
	CapacitySlots  int `json:"capacity_slots"`
	OccupiedSlots  int `json:"occupied_slots"`
	AvailableSlots int `json:"available_slots"`
	ClaimedOwners  int `json:"claimed_owners"`
	UncertainSlots int `json:"uncertain_slots"`
}

func (b *managedSharedBrowserBudget) snapshot() SharedManagedBudgetDiagnostic {
	if b == nil {
		return SharedManagedBudgetDiagnostic{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	s := SharedManagedBudgetDiagnostic{
		CapacitySlots: b.maxSlots, OccupiedSlots: b.used,
		AvailableSlots: b.maxSlots - b.used, ClaimedOwners: len(b.owners),
	}
	for r := range b.reservations {
		if r.uncertain {
			s.UncertainSlots++
		}
	}
	return s
}

type managedSharedBudgetReservation struct {
	budget    *managedSharedBrowserBudget
	uncertain bool // guarded by budget.mu
}

func (b *managedSharedBrowserBudget) reserve() (*managedSharedBudgetReservation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used >= b.maxSlots {
		return nil, errManagedSharedUnavailable
	}
	b.used++
	r := &managedSharedBudgetReservation{budget: b}
	b.reservations[r] = struct{}{}
	return r, nil
}

// Caller must first observe generation.done and a nil stopErr. A failed
// registry startup can have unreported launch side effects, including a nil
// session. Registry Stop success alone cannot prove those effects stopped.
func (r *managedSharedBudgetReservation) release() bool {
	if r == nil || r.budget == nil {
		return false
	}
	b := r.budget
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.uncertain {
		return false
	}
	if _, active := b.reservations[r]; active {
		delete(b.reservations, r)
		b.used--
	}
	return true // idempotent after a verified successful release
}

func (r *managedSharedBudgetReservation) markUncertain() {
	b := r.budget
	b.mu.Lock()
	r.uncertain = true
	b.mu.Unlock()
}

type managedSharedBudgetBackend struct {
	ManagedLeaseBackend
	reservation *managedSharedBudgetReservation
}

func (b *managedSharedBudgetBackend) StartManaged(ctx context.Context, options ManagedWorkerOptions) (WorkerInfo, error) {
	info, err := b.ManagedLeaseBackend.StartManaged(ctx, options)
	if err != nil || info.WorkerID == "" || info.State != WorkerReady {
		b.reservation.markUncertain()
	}
	return info, err
}
