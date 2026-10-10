package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func assertSharedBudgetSnapshot(t *testing.T, budget *managedSharedBrowserBudget, want SharedManagedBudgetDiagnostic) {
	t.Helper()
	if got := budget.snapshot(); got != want {
		t.Fatalf("budget snapshot = %+v; want %+v", got, want)
	}
}

func TestSharedBudgetSnapshotCountsAndUncertainRetention(t *testing.T) {
	budget, err := newManagedSharedBrowserBudget(2)
	if err != nil {
		t.Fatal(err)
	}
	assertSharedBudgetSnapshot(t, budget, SharedManagedBudgetDiagnostic{CapacitySlots: 2, AvailableSlots: 2})
	a, b := NewWorkerRegistry(WorkerDependencies{}), NewWorkerRegistry(WorkerDependencies{})
	claimA, err := budget.claim(a, "workspace-a")
	if err != nil {
		t.Fatal(err)
	}
	claimB, err := budget.claim(b, "workspace-b")
	if err != nil {
		t.Fatal(err)
	}
	first, err := budget.reserve()
	if err != nil {
		t.Fatal(err)
	}
	second, err := budget.reserve()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := budget.reserve(); err != errManagedSharedUnavailable {
		t.Fatalf("capacity overflow was accepted: %v", err)
	}
	assertSharedBudgetSnapshot(t, budget, SharedManagedBudgetDiagnostic{CapacitySlots: 2, OccupiedSlots: 2, ClaimedOwners: 2})
	first.markUncertain()
	assertSharedBudgetSnapshot(t, budget, SharedManagedBudgetDiagnostic{CapacitySlots: 2, OccupiedSlots: 2, ClaimedOwners: 2, UncertainSlots: 1})
	if first.release() || first.release() {
		t.Fatal("uncertain startup released occupancy")
	}
	if !second.release() || !second.release() {
		t.Fatal("verified release is not idempotent")
	}
	assertSharedBudgetSnapshot(t, budget, SharedManagedBudgetDiagnostic{CapacitySlots: 2, OccupiedSlots: 1, AvailableSlots: 1, ClaimedOwners: 2, UncertainSlots: 1})
	// The owner of an uncertain launch MUST retain its claim. Only B,
	// whose reservation was verified released, may safely relinquish ownership.
	budget.releaseOwner(b, claimB)
	assertSharedBudgetSnapshot(t, budget, SharedManagedBudgetDiagnostic{CapacitySlots: 2, OccupiedSlots: 1, AvailableSlots: 1, ClaimedOwners: 1, UncertainSlots: 1})
	_ = claimA // A remains latched: Close may not transfer an unknown process.
	if _, err := budget.reserve(); err != nil {
		t.Fatal(err)
	}
	if _, err := budget.reserve(); err != errManagedSharedUnavailable {
		t.Fatalf("uncertain slot disappeared: %v", err)
	}
}

func TestSharedBudgetSnapshotBridgeDiagnosticsAndRedaction(t *testing.T) {
	budget, p := testBrowserBudget(t), &budgetProcessFixture{}
	bridge, _, cwd := budgetBridgeFixture(t, budget, p, false)
	initial := SharedManagedBudgetDiagnostic{CapacitySlots: 2, AvailableSlots: 2, ClaimedOwners: 1}
	assertSharedBudgetSnapshot(t, budget, initial)
	if got := bridge.Diagnostics().SharedBudget; got == nil || *got != initial {
		t.Fatalf("missing private budget diagnostics: %+v", got)
	}
	payload, err := json.Marshal(bridge.Diagnostics())
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	raw, ok := decoded["shared_budget"].(map[string]any)
	if !ok || len(raw) != 5 {
		t.Fatalf("shared budget metrics shape: %v", raw)
	}
	for _, key := range []string{"capacity_slots", "occupied_slots", "available_slots", "claimed_owners", "uncertain_slots"} {
		if _, ok := raw[key].(float64); !ok {
			t.Fatalf("expected numeric-only %s", key)
		}
	}
	for _, forbidden := range []string{"workspace", "profile", "token", "pid", "path", "worker", "page", "root", "argv"} {
		for k := range raw {
			if strings.Contains(k, forbidden) {
				t.Fatalf("sensitive budget key %q", k)
			}
		}
	}
	token, meta := sharedBridgeAcquire(t, bridge, "snapshot-owner", cwd)
	wantBusy := SharedManagedBudgetDiagnostic{CapacitySlots: 2, OccupiedSlots: 1, AvailableSlots: 1, ClaimedOwners: 1}
	assertSharedBudgetSnapshot(t, budget, wantBusy)
	if got := bridge.Diagnostics().SharedBudget; got == nil || *got != wantBusy {
		t.Fatalf("bridge occupancy = %+v", got)
	}
	if _, err := bridge.Release(context.Background(), token, meta.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	if err := bridge.rotating.ReapIdle(time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	assertSharedBudgetSnapshot(t, budget, initial)
	if err := bridge.Close(); err != nil {
		t.Fatal(err)
	}
	assertSharedBudgetSnapshot(t, budget, SharedManagedBudgetDiagnostic{CapacitySlots: 2, AvailableSlots: 2})
	// Baseline exclusive ACPBridge is unaffected by the private diagnostics.
	exclusive, _, _, _ := sharedBridgeFixture(t, false)
	if exclusive.Diagnostics().SharedBudget != nil {
		t.Fatal("exclusive bridge leaked shared metrics")
	}
	exclusivePayload, err := json.Marshal(exclusive.Diagnostics())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(exclusivePayload), `"shared_budget"`) {
		t.Fatal("exclusive JSON changed")
	}
}

func TestSharedBudgetSnapshotConcurrentReadsAndRelease(t *testing.T) {
	budget, err := newManagedSharedBrowserBudget(4)
	if err != nil {
		t.Fatal(err)
	}
	reservations := make([]*managedSharedBudgetReservation, 4)
	for i := range reservations {
		reservations[i], err = budget.reserve()
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	failures := make(chan error, 512)
	for i := range 12 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 150; j++ {
				s := budget.snapshot()
				if s.CapacitySlots != 4 || s.AvailableSlots+s.OccupiedSlots != 4 || s.UncertainSlots > s.OccupiedSlots || s.OccupiedSlots < 0 || s.ClaimedOwners != 0 {
					failures <- fmt.Errorf("inconsistent snapshot: %+v", s)
					return
				}
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		reservations[0].markUncertain()
		for i := 1; i < 4; i++ {
			reservations[i].release()
		}
	}()
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	assertSharedBudgetSnapshot(t, budget, SharedManagedBudgetDiagnostic{CapacitySlots: 4, OccupiedSlots: 1, AvailableSlots: 3, UncertainSlots: 1})
}

func TestSharedBudgetSnapshotPendingStopKeepsCapacity(t *testing.T) {
	budget, processes := testBrowserBudget(t), &budgetProcessFixture{}
	bridge, fixture, cwd := budgetBridgeFixture(t, budget, processes, false)
	token, lease := sharedBridgeAcquire(t, bridge, "pending-stop", cwd)
	if _, err := bridge.Release(context.Background(), token, lease.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	entered, gate := make(chan struct{}, 1), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(gate) })
	fixture.mu.Lock()
	fixture.stopEntered, fixture.stopGate = entered, gate
	fixture.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- bridge.rotating.ReapIdle(time.Now().Add(time.Hour)) }()
	receive(t, entered)
	assertSharedBudgetSnapshot(t, budget, SharedManagedBudgetDiagnostic{CapacitySlots: 2, OccupiedSlots: 1, AvailableSlots: 1, ClaimedOwners: 1})
	once.Do(func() { close(gate) })
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
	assertSharedBudgetSnapshot(t, budget, SharedManagedBudgetDiagnostic{CapacitySlots: 2, AvailableSlots: 2, ClaimedOwners: 1})
	if err := bridge.Close(); err != nil {
		t.Fatal(err)
	}
	assertSharedBudgetSnapshot(t, budget, SharedManagedBudgetDiagnostic{CapacitySlots: 2, AvailableSlots: 2})
}
