package browser

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

func testCoordinator(t *testing.T, b *fakeLeaseBackend, p managedLeasePolicy) *managedLeaseCoordinator {
	t.Helper()
	c, err := newManagedLeaseCoordinator(b, p)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func coordinatorAcquire(t *testing.T, c *managedLeaseCoordinator) LeaseMetadata {
	t.Helper()
	m, _, err := c.Acquire(context.Background(), leaseScope(), leaseStart(t), "")
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func queueState(q *admissionQueue) (int, int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.active, len(q.waiters)
}
func waitQueue(t *testing.T, q *admissionQueue, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, queued := queueState(q); queued == n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("queue did not reach %d waiters", n)
}
func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("blocked result")
		var zero T
		return zero
	}
}
func backendStarts(b *fakeLeaseBackend) int { b.mu.Lock(); defer b.mu.Unlock(); return b.starts }

func TestConcurrencyFourWorkersFIFOAndOverflow(t *testing.T) {
	b := &fakeLeaseBackend{}
	p := defaultManagedLeasePolicy()
	p.QueueCapacity = 3
	c := testCoordinator(t, b, p)
	start := leaseStart(t)
	initial := make(chan LeaseMetadata, 4)
	for range 4 {
		go func() {
			meta, _, err := c.Acquire(context.Background(), leaseScope(), start, "")
			if err != nil {
				t.Error(err)
			}
			initial <- meta
		}()
	}
	leases := make([]LeaseMetadata, 4)
	for i := range leases {
		leases[i] = receive(t, initial)
	}
	if active, _ := queueState(c.queue); active != 4 || backendStarts(b) != 4 {
		t.Fatal("initial capacity")
	}
	queued := make([]chan LeaseMetadata, 3)
	for i := range queued {
		queued[i] = make(chan LeaseMetadata, 1)
		go func(ch chan LeaseMetadata) {
			meta, _, err := c.Acquire(context.Background(), leaseScope(), start, "")
			if err != nil {
				t.Error(err)
			}
			ch <- meta
		}(queued[i])
		waitQueue(t, c.queue, i+1)
	}
	_, _, err := c.Acquire(context.Background(), leaseScope(), start, "")
	assertBrowserCode(t, err, ErrQueueFull)
	if backendStarts(b) != 4 {
		t.Fatal("queued acquire spawned worker")
	}
	for i, ch := range queued {
		if _, err := c.Release(context.Background(), leaseScope(), leases[i].BrowserLeaseID); err != nil {
			t.Fatal(err)
		}
		meta := receive(t, ch)
		if meta.WorkerID != fmt.Sprint(5+i) {
			t.Fatal("FIFO admission violated")
		}
		if active, _ := queueState(c.queue); active != 4 {
			t.Fatal("capacity exceeded or leaked")
		}
		leases[i] = meta
	}
	for _, meta := range leases {
		if _, err := c.Release(context.Background(), leaseScope(), meta.BrowserLeaseID); err != nil {
			t.Fatal(err)
		}
	}
	if active, queued := queueState(c.queue); active != 0 || queued != 0 {
		t.Fatal("capacity not returned")
	}
}

func TestConcurrencyCancellationTimeoutAndAcquireFailure(t *testing.T) {
	for _, mode := range []string{"cancel", "policy timeout", "context deadline"} {
		t.Run(mode, func(t *testing.T) {
			b := &fakeLeaseBackend{}
			p := defaultManagedLeasePolicy()
			p.MaxConcurrency = 1
			if mode == "policy timeout" {
				p.QueueTimeout = 30 * time.Millisecond
			}
			c := testCoordinator(t, b, p)
			meta := coordinatorAcquire(t, c)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "context deadline" {
				ctx, cancel = context.WithTimeout(context.Background(), 30*time.Millisecond)
				defer cancel()
			}
			result := make(chan error, 1)
			start := leaseStart(t)
			go func() { _, _, err := c.Acquire(ctx, leaseScope(), start, ""); result <- err }()
			waitQueue(t, c.queue, 1)
			want := context.DeadlineExceeded
			if mode == "cancel" {
				want = context.Canceled
				cancel()
			}
			err := receive(t, result)
			if !errors.Is(err, want) {
				t.Fatalf("lost context error: %v", err)
			}
			if mode != "cancel" {
				assertBrowserCode(t, err, ErrQueueTimeout)
			}
			if active, n := queueState(c.queue); active != 1 || n != 0 || backendStarts(b) != 1 {
				t.Fatal("waiter leaked or spawned")
			}
			_, _ = c.Release(context.Background(), leaseScope(), meta.BrowserLeaseID)
			if backendStarts(b) != 1 {
				t.Fatal("canceled waiter later started")
			}
		})
	}
	for _, cleanupFails := range []bool{false, true} {
		b := &fakeLeaseBackend{newErr: errors.New("new page failed")}
		if cleanupFails {
			b.stopErr = errors.New("stop failed")
		}
		p := defaultManagedLeasePolicy()
		p.MaxConcurrency = 1
		p.QueueCapacity = 0
		c := testCoordinator(t, b, p)
		_, _, err := c.Acquire(context.Background(), leaseScope(), leaseStart(t), "")
		if err == nil {
			t.Fatal("expected acquire failure")
		}
		active, _ := queueState(c.queue)
		if cleanupFails && active != 1 || !cleanupFails && active != 0 {
			t.Fatal("failure capacity accounting")
		}
		if cleanupFails {
			_, _, err = c.Acquire(context.Background(), leaseScope(), leaseStart(t), "")
			assertBrowserCode(t, err, ErrQueueFull)
			if backendStarts(b) != 1 {
				t.Fatal("orphan allowed unbounded spawning")
			}
		}
	}
}

func TestConcurrencyReleaseAndCleanupFailure(t *testing.T) {
	for _, cleanupFails := range []bool{false, true} {
		b := &fakeLeaseBackend{}
		if cleanupFails {
			b.stopErr = errors.New("stop failed")
		}
		c := testCoordinator(t, b, defaultManagedLeasePolicy())
		meta := coordinatorAcquire(t, c)
		wrong := leaseScope()
		wrong.OwnerTaskID = "other"
		_, err := c.Release(context.Background(), wrong, meta.BrowserLeaseID)
		assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
		var wg sync.WaitGroup
		for range 12 {
			wg.Go(func() {
				m, err := c.Release(context.Background(), leaseScope(), meta.BrowserLeaseID)
				if cleanupFails {
					if err == nil || m.CleanupState != CleanupFailed {
						t.Error("cleanup failure hidden")
					}
				} else if err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		active, _ := queueState(c.queue)
		if cleanupFails && active != 1 || !cleanupFails && active != 0 {
			t.Fatal("double release capacity accounting")
		}
		if err := c.SweepExpired(time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		b.mu.Lock()
		stops := len(b.stopped)
		b.mu.Unlock()
		if stops != 1 {
			t.Fatal("cleanup retried or double stopped")
		}
	}
}

func TestConcurrencyIdleTTLAndSweeps(t *testing.T) {
	b := &fakeLeaseBackend{entered: make(chan struct{}, 1), gate: make(chan struct{})}
	c := testCoordinator(t, b, defaultManagedLeasePolicy())
	meta := coordinatorAcquire(t, c)
	if !meta.ExpiresAt.Equal(meta.LastActiveAt.Add(c.policy.IdleTTL)) {
		t.Fatal("initial TTL")
	}
	l, _ := c.manager.lease(meta.BrowserLeaseID)
	// Make the lease genuinely idle without sleeping for the production TTL.
	l.mu.Lock()
	l.metadata.LastActiveAt = time.Now().Add(-2 * c.policy.IdleTTL)
	l.metadata.ExpiresAt = l.metadata.LastActiveAt.Add(c.policy.IdleTTL)
	l.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := c.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "click", nil)
		done <- err
	}()
	receive(t, b.entered)
	if err := c.SweepExpired(time.Now()); err != nil {
		t.Fatal(err)
	}
	if active, _ := queueState(c.queue); active != 1 {
		t.Fatal("in-flight lease expired")
	}
	b.mu.Lock()
	stopped := len(b.stopped)
	b.mu.Unlock()
	if stopped != 0 {
		t.Fatal("in-flight worker stopped")
	}
	close(b.gate)
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
	l.mu.Lock()
	refreshed := l.metadata
	l.mu.Unlock()
	if !refreshed.LastActiveAt.After(meta.LastActiveAt) || !refreshed.ExpiresAt.Equal(refreshed.LastActiveAt.Add(c.policy.IdleTTL)) {
		t.Fatal("TTL not refreshed")
	}
	if err := c.SweepExpired(meta.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	if active, _ := queueState(c.queue); active != 1 {
		t.Fatal("stale expiry ignored refreshed activity")
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if err := c.SweepExpired(refreshed.ExpiresAt); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Go(func() {
		if _, err := c.Release(context.Background(), leaseScope(), meta.BrowserLeaseID); err != nil {
			t.Error(err)
		}
	})
	wg.Wait()
	if active, _ := queueState(c.queue); active != 0 {
		t.Fatal("expiry capacity leaked")
	}
	b.mu.Lock()
	stopped = len(b.stopped)
	b.mu.Unlock()
	if stopped != 1 {
		t.Fatal("concurrent sweeps double stopped")
	}
}

func TestConcurrencyExpiryFailureRetainsCapacity(t *testing.T) {
	b := &fakeLeaseBackend{stopErr: errors.New("stop failed")}
	c := testCoordinator(t, b, defaultManagedLeasePolicy())
	meta := coordinatorAcquire(t, c)
	if err := c.SweepExpired(meta.ExpiresAt); err == nil {
		t.Fatal("expiry failure hidden")
	}
	if active, _ := queueState(c.queue); active != 1 {
		t.Fatal("failed expiry returned capacity")
	}
	if err := c.SweepExpired(meta.ExpiresAt.Add(time.Hour)); err != nil {
		t.Fatal("sweep retries cleanup failure")
	}
}

func TestConcurrencyExpiryAloneReturnsCapacity(t *testing.T) {
	b := &fakeLeaseBackend{}
	c := testCoordinator(t, b, defaultManagedLeasePolicy())
	meta := coordinatorAcquire(t, c)
	if err := c.SweepExpired(meta.ExpiresAt.Add(-time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
	if active, _ := queueState(c.queue); active != 1 {
		t.Fatal("expired before idle deadline")
	}
	if err := c.SweepExpired(meta.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	if active, _ := queueState(c.queue); active != 0 {
		t.Fatal("expiry alone did not return capacity")
	}
	if _, err := c.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "click", nil); err == nil {
		t.Fatal("expired lease still usable")
	}
}

func TestConcurrencyFailedCallDoesNotRefreshTTL(t *testing.T) {
	b := &fakeLeaseBackend{callErr: errors.New("operation failed")}
	c := testCoordinator(t, b, defaultManagedLeasePolicy())
	meta := coordinatorAcquire(t, c)
	if _, err := c.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "click", nil); err == nil {
		t.Fatal("failed call hidden")
	}
	l, _ := c.manager.lease(meta.BrowserLeaseID)
	l.mu.Lock()
	after := l.metadata
	l.mu.Unlock()
	if after.LastActiveAt != meta.LastActiveAt || after.ExpiresAt != meta.ExpiresAt {
		t.Fatal("failed operation refreshed idle TTL")
	}
}

// Count live workers, not just admitted leases, to catch premature permit return.
type countingLeaseBackend struct {
	fakeLeaseBackend
	liveMu     sync.Mutex
	live, peak int
}

func (b *countingLeaseBackend) StartManaged(ctx context.Context, opts ManagedWorkerOptions) (WorkerInfo, error) {
	info, err := b.fakeLeaseBackend.StartManaged(ctx, opts)
	b.liveMu.Lock()
	b.live++
	if b.live > b.peak {
		b.peak = b.live
	}
	b.liveMu.Unlock()
	return info, err
}
func (b *countingLeaseBackend) Stop(id string) error {
	err := b.fakeLeaseBackend.Stop(id)
	b.liveMu.Lock()
	b.live--
	b.liveMu.Unlock()
	return err
}
func TestConcurrencyRepeatedAcquireReleaseStress(t *testing.T) {
	b := &countingLeaseBackend{}
	p := defaultManagedLeasePolicy()
	p.QueueCapacity = 32
	c, err := newManagedLeaseCoordinator(b, p)
	if err != nil {
		t.Fatal(err)
	}
	start := leaseStart(t)
	var wg sync.WaitGroup
	for range 24 {
		wg.Go(func() {
			for range 10 {
				meta, _, err := c.Acquire(context.Background(), leaseScope(), start, "")
				if err != nil {
					t.Error(err)
					return
				}
				if _, err := c.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "click", nil); err != nil {
					t.Error(err)
				}
				if _, err := c.Release(context.Background(), leaseScope(), meta.BrowserLeaseID); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	if b.peak > 4 || b.live != 0 || backendStarts(&b.fakeLeaseBackend) != 240 {
		t.Fatalf("peak=%d live=%d", b.peak, b.live)
	}
	if active, n := queueState(c.queue); active != 0 || n != 0 {
		t.Fatal("stress leaked admission")
	}
}

func TestConcurrencyCanceledWaiterAtRelease(t *testing.T) {
	b := &fakeLeaseBackend{}
	p := defaultManagedLeasePolicy()
	p.MaxConcurrency = 1
	c := testCoordinator(t, b, p)
	start := leaseStart(t)
	for range 30 {
		meta := coordinatorAcquire(t, c)
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() { _, _, err := c.Acquire(ctx, leaseScope(), start, ""); result <- err }()
		waitQueue(t, c.queue, 1)
		cancel() // Release may pump before the waiter itself processes cancellation.
		if _, err := c.Release(context.Background(), leaseScope(), meta.BrowserLeaseID); err != nil {
			t.Fatal(err)
		}
		if err := receive(t, result); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	if backendStarts(b) != 30 {
		t.Fatal("canceled waiters spawned")
	}
	if active, n := queueState(c.queue); active != 0 || n != 0 {
		t.Fatal("release/cancel race leaked")
	}
}

func TestResourceLocksCanonicalAtomicFIFO(t *testing.T) {
	locks, err := newResourceLocks(defaultResourceLockPolicy())
	if err != nil {
		t.Fatal(err)
	}
	owner := leaseScope()
	a, err := locks.Acquire(context.Background(), owner, "resource:B", " site:A ", "resource:B")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.permit.keys, []string{"resource:B", "site:A"}) {
		t.Fatal("noncanonical keys")
	}
	// A disjoint holder proceeds despite another holder; browser slots are irrelevant.
	b, err := locks.Acquire(context.Background(), owner, "site:C")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Release(owner); err != nil {
		t.Fatal(err)
	}
	wrong := owner
	wrong.OwnerSessionID = "other"
	assertBrowserCode(t, a.Release(wrong), ErrLockOwnerMismatch)
	chs := make([]chan *resourceLock, 2)
	for i := range chs {
		chs[i] = make(chan *resourceLock, 1)
		keys := []string{"site:A", "resource:B"}
		if i == 1 {
			keys = []string{"resource:B", "site:A"}
		}
		go func(ch chan *resourceLock, keys []string) {
			h, err := locks.Acquire(context.Background(), owner, keys...)
			if err != nil {
				t.Error(err)
			}
			ch <- h
		}(chs[i], keys)
		waitQueue(t, locks.queue, i+1)
	}
	if active, _ := queueState(locks.queue); active != 1 {
		t.Fatal("overlapping lock granted")
	}
	_ = a.Release(owner)
	first := receive(t, chs[0])
	_ = a.Release(owner) // Must not unlock the new holder's keys.
	if active, n := queueState(locks.queue); active != 1 || n != 1 {
		t.Fatal("idempotent release stole lock")
	}
	_ = first.Release(owner)
	second := receive(t, chs[1])
	_ = second.Release(owner)
	if active, n := queueState(locks.queue); active != 0 || n != 0 {
		t.Fatal("locks leaked")
	}
	assertBrowserCode(t, a.Release(wrong), ErrLockOwnerMismatch)
}

func TestResourceLocksCancelTimeoutOverflowAndValidation(t *testing.T) {
	p := resourceLockPolicy{1, 50 * time.Millisecond}
	locks, _ := newResourceLocks(p)
	owner := leaseScope()
	h, _ := locks.Acquire(context.Background(), owner, "site:A")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := locks.Acquire(ctx, owner, "site:A", "resource:B"); result <- err }()
	waitQueue(t, locks.queue, 1)
	_, err := locks.Acquire(context.Background(), owner, "site:A")
	assertBrowserCode(t, err, ErrQueueFull)
	cancel()
	if err := receive(t, result); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// Canceled atomic request never partially held resource:B.
	disjoint, err := locks.Acquire(context.Background(), owner, "resource:B")
	if err != nil {
		t.Fatal(err)
	}
	_ = disjoint.Release(owner)
	_, err = locks.Acquire(context.Background(), owner, "site:A")
	assertBrowserCode(t, err, ErrQueueTimeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lost timeout cause")
	}
	_ = h.Release(owner)
	if active, n := queueState(locks.queue); active != 0 || n != 0 {
		t.Fatal("lock waiter leaked")
	}
	for _, keys := range [][]string{nil, {""}, {"  "}} {
		_, err := locks.Acquire(context.Background(), owner, keys...)
		assertBrowserCode(t, err, ErrActionInvalid)
	}
	_, err = locks.Acquire(context.Background(), RequestScope{}, "key")
	assertBrowserCode(t, err, ErrScopeRequired)
}

func TestConcurrencyPolicyValidation(t *testing.T) {
	for _, mutate := range []func(*managedLeasePolicy){func(p *managedLeasePolicy) { p.MaxConcurrency = 0 }, func(p *managedLeasePolicy) { p.QueueCapacity = -1 }, func(p *managedLeasePolicy) { p.QueueTimeout = 0 }, func(p *managedLeasePolicy) { p.IdleTTL = 0 }, func(p *managedLeasePolicy) { p.CleanupTimeout = 0 }} {
		p := defaultManagedLeasePolicy()
		mutate(&p)
		_, err := newManagedLeaseCoordinator(&fakeLeaseBackend{}, p)
		assertBrowserCode(t, err, ErrPolicyConflict)
	}
	_, err := newManagedLeaseCoordinator(nil, defaultManagedLeasePolicy())
	assertBrowserCode(t, err, ErrPolicyConflict)
	for _, p := range []resourceLockPolicy{{-1, time.Second}, {1, 0}} {
		_, err := newResourceLocks(p)
		assertBrowserCode(t, err, ErrPolicyConflict)
	}
}
