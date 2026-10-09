package browser

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Decorates the existing fake, never opens a socket or launches a process.
type sharedProofBackend struct {
	fakeLeaseBackend
	proofMu                                          sync.Mutex
	pages                                            map[float64]string
	next                                             float64
	unownedCloses, wrongWorkers, inCall, peak        int
	startEntered, startGate, callEntered, callGate   chan struct{}
	ignoreStartCancel, ignoreCallCancel              bool
	newFailure, malformed, closeFailure, stopFailure bool
}

func (f *sharedProofBackend) StartManaged(ctx context.Context, opts ManagedWorkerOptions) (WorkerInfo, error) {
	if f.startEntered != nil {
		close(f.startEntered)
		if f.ignoreStartCancel {
			<-f.startGate
		} else {
			select {
			case <-ctx.Done():
				return WorkerInfo{}, ctx.Err()
			case <-f.startGate:
			}
		}
	}
	return f.fakeLeaseBackend.StartManaged(ctx, opts)
}

func (f *sharedProofBackend) Call(ctx context.Context, worker, tool string, args map[string]any) (map[string]any, error) {
	f.proofMu.Lock()
	f.inCall++
	if f.inCall > f.peak {
		f.peak = f.inCall
	}
	if worker != "1" {
		f.wrongWorkers++
	}
	f.proofMu.Unlock()
	defer func() { f.proofMu.Lock(); f.inCall--; f.proofMu.Unlock() }()
	if tool == "click" && f.callEntered != nil {
		f.callEntered <- struct{}{}
		if f.ignoreCallCancel {
			<-f.callGate
		} else {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-f.callGate:
			}
		}
	}
	if args["value"] == "fail" {
		return nil, errors.New("one lease operation failed")
	}
	if tool == "new_page" && f.newFailure {
		return nil, errors.New("unknown new page side effect")
	}
	if tool == "close_page" {
		f.proofMu.Lock()
		page, _ := args["pageId"].(float64)
		if _, ok := f.pages[page]; !ok {
			f.unownedCloses++
		}
		if !f.closeFailure {
			delete(f.pages, page)
		}
		f.proofMu.Unlock()
		if f.closeFailure {
			return nil, errors.New("close failed")
		}
	}
	return f.fakeLeaseBackend.Call(ctx, worker, tool, args)
}

func (f *sharedProofBackend) Stop(id string) error {
	err := f.fakeLeaseBackend.Stop(id)
	if f.stopFailure {
		return errors.New("owned worker stop failed")
	}
	f.proofMu.Lock()
	clear(f.pages)
	f.proofMu.Unlock()
	return err
}

func sharedProof(t *testing.T, f *sharedProofBackend) (*managedSharedWorkerBackend, *ManagedLeaseManager) {
	t.Helper()
	f.pages = make(map[float64]string)
	f.response = func(_ string, isolation string) map[string]any {
		f.proofMu.Lock()
		defer f.proofMu.Unlock()
		f.next++
		f.pages[f.next] = isolation
		if f.malformed {
			return map[string]any{}
		}
		return leasePages(isolation, f.next)
	}
	b, err := newManagedSharedWorkerBackend(f, ManagedWorkerOptions{Cwd: leaseScope().CanonicalWorkspaceRoot})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = b.Shutdown(ctx)
	})
	return b, NewManagedLeaseManager(b)
}

func sharedScope(i int) RequestScope {
	s := leaseScope()
	s.OwnerTaskID, s.OwnerSessionID, s.OwnerACPSessionID = fmt.Sprint(i), fmt.Sprint(i), fmt.Sprint(i)
	if i == 0 {
		s.Provenance = ScopeRuntime
	}
	return s
}

func sharedAcquire(t *testing.T, m *ManagedLeaseManager, scope RequestScope) LeaseMetadata {
	t.Helper()
	meta, _, err := m.Acquire(context.Background(), scope, leaseStart(t), "")
	if err != nil {
		t.Fatal(err)
	}
	return meta
}

func sharedStops(f *sharedProofBackend) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.stopped)
}

func TestManagedSharedThreeOwners(t *testing.T) {
	f := &sharedProofBackend{}
	b, m := sharedProof(t, f)
	start := leaseStart(t)
	metas := make(chan LeaseMetadata, 3)
	var wg sync.WaitGroup
	for i := range 3 {
		wg.Go(func() {
			meta, _, err := m.Acquire(context.Background(), sharedScope(i), start, "")
			if err != nil {
				t.Error(err)
			}
			metas <- meta
		})
	}
	wg.Wait()
	pages, contexts, tokens := map[string]bool{}, map[string]bool{}, map[string]bool{}
	leases := make([]LeaseMetadata, 3)
	for i := range leases {
		meta := receive(t, metas)
		leases[i] = meta
		if pages[meta.PageID] || contexts[meta.IsolationContextName] || tokens[meta.WorkerID] {
			t.Fatal("lease identity collision")
		}
		pages[meta.PageID], contexts[meta.IsolationContextName], tokens[meta.WorkerID] = true, true, true
	}
	if backendStarts(&f.fakeLeaseBackend) != 1 {
		t.Fatal("more than one underlying startup")
	}
	for _, meta := range leases {
		wg.Go(func() {
			result, err := m.Call(context.Background(), meta.Scope, meta.BrowserLeaseID, "click", nil)
			if err != nil || result["worker"] != "1" || fmt.Sprint(result["args"].(map[string]any)["pageId"]) != meta.PageID {
				t.Error("misrouted operation", err)
			}
		})
	}
	wg.Wait()
	a := leases[0]
	f.mu.Lock()
	before := len(f.calls)
	f.mu.Unlock()
	_, err := m.Call(context.Background(), a.Scope, a.BrowserLeaseID, "click", map[string]any{"pageId": 999.0})
	assertBrowserCode(t, err, ErrLeaseTargetMismatch)
	wrong := a.Scope
	wrong.CanonicalWorkspaceRoot = "/another"
	_, err = m.Call(context.Background(), wrong, a.BrowserLeaseID, "click", nil)
	assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
	_, err = m.Call(context.Background(), leases[1].Scope, a.BrowserLeaseID, "click", nil)
	assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
	if _, _, err := m.Acquire(context.Background(), wrong, start, ""); err == nil {
		t.Fatal("cross workspace acquire")
	}
	f.mu.Lock()
	after := len(f.calls)
	f.mu.Unlock()
	if after != before {
		t.Fatal("rejected ownership reached backend")
	}
	if _, err := m.Call(context.Background(), a.Scope, a.BrowserLeaseID, "click", map[string]any{"value": "fail"}); err == nil {
		t.Fatal("failure hidden")
	}
	if _, err := m.Call(context.Background(), leases[1].Scope, leases[1].BrowserLeaseID, "click", nil); err != nil {
		t.Fatal("one failure killed sibling", err)
	}
	for _, meta := range leases {
		for range 4 {
			wg.Go(func() {
				if _, err := m.Release(context.Background(), meta.Scope, meta.BrowserLeaseID); err != nil {
					t.Error(err)
				}
			})
		}
	}
	wg.Wait()
	if sharedStops(f) != 0 {
		t.Fatal("release stopped shared worker")
	}
	f.proofMu.Lock()
	if len(f.pages) != 0 || f.unownedCloses != 0 || f.wrongWorkers != 0 || f.peak != 1 {
		t.Error("ownership/serialization failure")
	}
	f.proofMu.Unlock()
	// A released borrow must not decrement a new holder or reuse a page.
	seq := sharedAcquire(t, m, sharedScope(4))
	if backendStarts(&f.fakeLeaseBackend) != 1 || pages[seq.PageID] {
		t.Fatal("idle reuse failed")
	}
	if err := b.Stop(a.WorkerID); err != nil {
		t.Fatal(err)
	}
	if err := b.ReapIdle(time.Now().Add(time.Hour)); err != nil || sharedStops(f) != 0 {
		t.Fatal("reaped active holder", err)
	}
	if _, err := m.Release(context.Background(), seq.Scope, seq.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	if err := b.ReapIdle(time.Now()); err != nil || sharedStops(f) != 0 {
		t.Fatal("retention missing", err)
	}
	for range 3 {
		if err := b.ReapIdle(time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if sharedStops(f) != 1 {
		t.Fatal("owned worker stop count")
	}
	if _, _, err := m.Acquire(context.Background(), a.Scope, start, ""); err == nil {
		t.Fatal("stopped generation reused")
	}
}

func TestManagedSharedAcquireCleanup(t *testing.T) {
	for _, mode := range []string{"new failure", "malformed", "cancel after page", "close failure", "start failure", "stop failure"} {
		t.Run(mode, func(t *testing.T) {
			f := &sharedProofBackend{}
			b, m := sharedProof(t, f)
			if mode == "start failure" {
				f.startErr = errors.New("start failed")
			}
			if mode == "stop failure" {
				f.stopFailure = true
			}
			var sibling LeaseMetadata
			if mode != "start failure" {
				sibling = sharedAcquire(t, m, sharedScope(0))
			}
			switch mode {
			case "new failure":
				f.newFailure = true
			case "malformed":
				f.malformed = true
			case "close failure":
				f.closeFailure = true
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel after page" {
				response := f.response
				f.response = func(worker, isolation string) map[string]any {
					result := response(worker, isolation)
					cancel()
					return result
				}
			}
			meta, _, err := m.Acquire(ctx, sharedScope(1), leaseStart(t), "")
			if mode != "close failure" && mode != "stop failure" && err == nil {
				t.Fatal("acquire failure hidden")
			}
			if mode == "close failure" {
				if _, err := m.Release(context.Background(), meta.Scope, meta.BrowserLeaseID); err == nil {
					t.Fatal("cleanup failure hidden")
				}
			}
			if sibling.BrowserLeaseID != "" {
				if sharedStops(f) != 0 {
					t.Fatal("failure killed sibling")
				}
				if _, err := m.Call(context.Background(), sibling.Scope, sibling.BrowserLeaseID, "click", nil); err != nil {
					t.Fatal(err)
				}
				f.closeFailure = false
				_, _ = m.Release(context.Background(), sibling.Scope, sibling.BrowserLeaseID)
			}
			if mode == "stop failure" {
				_, _ = m.Release(context.Background(), meta.Scope, meta.BrowserLeaseID)
			}
			err = b.ReapIdle(time.Now().Add(time.Hour))
			if mode == "stop failure" && err == nil {
				t.Fatal("underlying stop failure hidden")
			}
			if mode != "stop failure" && err != nil {
				t.Fatal(err)
			}
			_ = b.Shutdown(context.Background())
			if sharedStops(f) != 1 {
				t.Fatal("cleanup must stop owned generation exactly once")
			}
			f.proofMu.Lock()
			if f.unownedCloses != 0 || f.wrongWorkers != 0 || (!f.stopFailure && len(f.pages) != 0) {
				t.Error("unsafe/orphan cleanup")
			}
			f.proofMu.Unlock()
		})
	}
}

func TestManagedSharedStartupCancellation(t *testing.T) {
	for _, ignore := range []bool{false, true} {
		t.Run(fmt.Sprint(ignore), func(t *testing.T) {
			f := &sharedProofBackend{startEntered: make(chan struct{}), startGate: make(chan struct{}), ignoreStartCancel: ignore}
			b, _ := sharedProof(t, f)
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() { _, err := b.StartManaged(ctx, b.options); result <- err }()
			receive(t, f.startEntered)
			cancel()
			if err := receive(t, result); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer shutdownCancel()
			err := b.Shutdown(shutdownCtx)
			if ignore && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("stuck startup blocked shutdown", err)
			}
			close(f.startGate)
			if err := b.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			if sharedStops(f) > 1 {
				t.Fatal("double stop")
			}
			if ignore && sharedStops(f) != 1 {
				t.Fatal("late successful startup leaked")
			}
			if _, err := b.StartManaged(context.Background(), b.options); err == nil {
				t.Fatal("closed worker reused")
			}
		})
	}
}

func TestManagedSharedShutdownDuringCall(t *testing.T) {
	f := &sharedProofBackend{callEntered: make(chan struct{}, 1), callGate: make(chan struct{})}
	b, m := sharedProof(t, f)
	meta := sharedAcquire(t, m, sharedScope(0))
	queued := sharedAcquire(t, m, sharedScope(1))
	callDone := make(chan error, 1)
	go func() {
		_, err := m.Call(context.Background(), meta.Scope, meta.BrowserLeaseID, "click", nil)
		callDone <- err
	}()
	receive(t, f.callEntered)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Call(ctx, sharedScope(1), "unknown", "click", nil); err == nil {
		t.Fatal("canceled call accepted")
	}
	queuedDone := make(chan error, 1)
	go func() { _, err := m.Call(ctx, queued.Scope, queued.BrowserLeaseID, "click", nil); queuedDone <- err }()
	if err := receive(t, queuedDone); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := leaseStart(t)
	for range 12 {
		wg.Go(func() { _, _, _ = m.Acquire(context.Background(), sharedScope(2), start, "") })
		wg.Go(func() { _, _ = m.Release(context.Background(), meta.Scope, meta.BrowserLeaseID) })
		wg.Go(func() {
			if err := b.Shutdown(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	if err := receive(t, callDone); err == nil {
		t.Fatal("shutdown published stale result")
	}
	wg.Wait()
	if sharedStops(f) != 1 {
		t.Fatal("shutdown double stopped")
	}
	f.proofMu.Lock()
	if f.inCall != 0 || f.peak != 1 || f.unownedCloses != 0 || len(f.pages) != 0 {
		t.Error("shutdown cleanup/serialization")
	}
	f.proofMu.Unlock()
}

func TestManagedSharedStuckCall(t *testing.T) {
	f := &sharedProofBackend{callEntered: make(chan struct{}, 1), callGate: make(chan struct{}), ignoreCallCancel: true}
	b, m := sharedProof(t, f)
	meta := sharedAcquire(t, m, sharedScope(0))
	done := make(chan error, 1)
	go func() {
		_, err := m.Call(context.Background(), meta.Scope, meta.BrowserLeaseID, "click", nil)
		done <- err
	}()
	receive(t, f.callEntered)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := b.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if sharedStops(f) != 0 {
		t.Fatal("stop raced with in-flight call")
	}
	close(f.callGate)
	if err := receive(t, done); err == nil {
		t.Fatal("late result published")
	}
	if err := b.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sharedStops(f) != 1 {
		t.Fatal("late call leaked worker")
	}
}

func TestManagedSharedStartupWaiter(t *testing.T) {
	f := &sharedProofBackend{startEntered: make(chan struct{}), startGate: make(chan struct{})}
	b, _ := sharedProof(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := b.StartManaged(ctx, b.options); done <- err }()
	receive(t, f.startEntered)
	cancel()
	if err := receive(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(f.startGate)
	info, err := b.StartManaged(context.Background(), b.options)
	if err != nil {
		t.Fatal("first waiter cancellation killed shared startup", err)
	}
	if err := b.Stop(info.WorkerID); err != nil {
		t.Fatal(err)
	}
	if backendStarts(&f.fakeLeaseBackend) != 1 {
		t.Fatal("startup was retried")
	}
}
