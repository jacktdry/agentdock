package browser

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

type generationFixture struct {
	mu                    sync.Mutex
	sessions              []*generationSession
	live, peak, stops     int
	failStop              bool
	stopEntered, stopGate chan struct{}
}
type generationSession struct {
	*sharedBridgeSession
	f *generationFixture
}

func (s *generationSession) Close() error {
	f := s.f
	f.mu.Lock()
	f.stops++
	entered, gate, fail := f.stopEntered, f.stopGate, f.failStop
	f.mu.Unlock()
	if entered != nil {
		entered <- struct{}{}
	}
	if gate != nil {
		<-gate
	}
	if fail {
		return errors.New("private-token-process-canary")
	}
	err := s.fakeWorkerSession.Close()
	f.mu.Lock()
	f.live--
	f.mu.Unlock()
	return err
}
func rotatingBridgeFixture(t *testing.T) (*ACPBridge, *generationFixture, string) {
	t.Helper()
	f := &generationFixture{}
	// Build schema fixtures on the test goroutine; Open creates a fresh session.
	base := fakeManagedSession(t, "generation")
	registry := NewWorkerRegistry(WorkerDependencies{
		Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return ManagedEngineVersion, nil },
		Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			s := &generationSession{sharedBridgeSession: &sharedBridgeSession{fakeWorkerSession: &fakeWorkerSession{info: base.info, tools: base.tools}, pages: make(map[float64]string)}, f: f}
			f.sessions = append(f.sessions, s)
			f.live++
			if f.live > f.peak {
				f.peak = f.live
			}
			return s, nil
		},
	})
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := NewRoutePlanner(nil, catalog, nil)
	if err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	b, err := newACPBridgeRotatingSharedForWorkspace(planner, registry, cwd)
	if err != nil {
		t.Fatal(err)
	}
	b.lifecycle.Close()
	t.Cleanup(func() { _ = b.Close() })
	return b, f, cwd
}
func (f *generationFixture) check(t *testing.T, starts, stops, live int) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sessions) != starts || f.stops != stops || f.live != live || f.peak > 1 {
		t.Fatalf("starts=%d stops=%d live=%d peak=%d", len(f.sessions), f.stops, f.live, f.peak)
	}
}

func TestACPBridgeSharedGenerationReplacement(t *testing.T) {
	b, f, cwd := rotatingBridgeFixture(t)
	ctx := context.Background()
	other, err := b.RegisterSession("other", "codex", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Acquire(ctx, other, ""); err == nil {
		t.Fatal("cross workspace fallback")
	}
	if _, err := b.Acquire(ctx, "unknown", ""); err == nil {
		t.Fatal("missing owner accepted")
	}
	f.check(t, 0, 0, 0)
	a, am := sharedBridgeAcquire(t, b, "a", cwd)
	bt, bm := sharedBridgeAcquire(t, b, "b", cwd)
	f.check(t, 1, 0, 1)
	old := b.rotating.generation
	if err := b.rotating.ReapIdle(time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	f.check(t, 1, 0, 1) // Active borrows must protect their sibling browser.
	if _, err := b.Release(ctx, a, am.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Call(ctx, bt, bm.BrowserLeaseID, "click", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Release(ctx, bt, bm.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	if err := b.sweepLifecycle(time.Now()); err != nil {
		t.Fatal(err)
	}
	f.check(t, 1, 0, 1)
	for range 3 {
		if err := b.sweepLifecycle(time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	f.check(t, 1, 1, 0)
	var wg sync.WaitGroup
	metas := make(chan LeaseMetadata, 3)
	for i := range 3 {
		token, err := b.RegisterSession(fmt.Sprintf("next-%d", i), "codex", cwd)
		if err != nil {
			t.Fatal(err)
		}
		wg.Go(func() {
			meta, err := b.Acquire(ctx, token, "")
			if err != nil {
				t.Error(err)
			}
			metas <- meta
		})
	}
	wg.Wait()
	f.check(t, 2, 1, 1)
	current := b.rotating.generation
	if current == old || current.worker.WorkerID == old.worker.WorkerID {
		t.Fatal("generation ABA")
	}
	seen := map[string]bool{am.WorkerID: true, bm.WorkerID: true}
	for range 3 {
		meta := <-metas
		if seen[meta.WorkerID] {
			t.Fatal("borrow ABA")
		}
		seen[meta.WorkerID] = true
	}
	// Numeric page IDs deliberately repeat across sessions. Old capabilities
	// must still fail without selecting, closing or calling the new session.
	for _, tool := range []string{"click", "close_page", "select_page", "new_page"} {
		if _, err := b.rotating.Call(ctx, am.WorkerID, tool, map[string]any{"pageId": 2.0}); err == nil {
			t.Fatal("stale borrow accepted")
		}
	}
	if _, err := b.Call(ctx, a, am.BrowserLeaseID, "click", nil); err == nil {
		t.Fatal("old lease routed")
	}
	if _, err := old.Call(ctx, old.worker.WorkerID, "close_page", map[string]any{"pageId": 2.0}); err == nil {
		t.Fatal("retired generation routed")
	}
	if err := b.rotating.Stop(am.WorkerID); err != nil {
		t.Fatal("bounded cleanup tombstone missing", err)
	}
	f.check(t, 2, 1, 1)
	if _, err := b.Acquire(ctx, other, ""); err == nil {
		t.Fatal("replacement crossed workspace")
	}
	for range 3 {
		if err := b.Close(); err != nil {
			t.Fatal(err)
		}
	}
	f.check(t, 2, 2, 0)
	if _, err := b.rotating.StartManaged(ctx, b.rotating.options); err == nil {
		t.Fatal("shutdown reopened")
	}
}

func TestACPBridgeSharedGenerationStopFailure(t *testing.T) {
	b, f, cwd := rotatingBridgeFixture(t)
	ctx := context.Background()
	a, am := sharedBridgeAcquire(t, b, "a", cwd)
	if _, err := b.Release(ctx, a, am.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.failStop = true
	f.mu.Unlock()
	for range 3 {
		err := b.sweepLifecycle(time.Now().Add(time.Hour))
		if err == nil || strings.Contains(err.Error(), "canary") {
			t.Fatal("stop failure not opaque", err)
		}
		if _, err := b.Acquire(ctx, a, ""); err == nil {
			t.Fatal("unknown worker overlapped")
		}
	}
	for range 2 {
		if err := b.Close(); err == nil || strings.Contains(err.Error(), "canary") {
			t.Fatal("failed Stop hidden by Close")
		}
	}
	f.check(t, 1, 1, 1)
}

func TestManagedSharedGenerationShutdownDuringReap(t *testing.T) {
	b, f, cwd := rotatingBridgeFixture(t)
	ctx := context.Background()
	a, am := sharedBridgeAcquire(t, b, "a", cwd)
	if _, err := b.Release(ctx, a, am.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.stopEntered, f.stopGate = make(chan struct{}, 1), make(chan struct{})
	entered, gate := f.stopEntered, f.stopGate
	f.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- b.rotating.ReapIdle(time.Now().Add(time.Hour)) }()
	receive(t, entered)
	bounded, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := b.rotating.Shutdown(bounded); err == nil {
		t.Fatal("unfinished cleanup reported success")
	}
	close(gate)
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
	if _, err := b.rotating.StartManaged(ctx, b.rotating.options); err == nil {
		t.Fatal("late replacement after Shutdown")
	}
	for range 3 {
		if err := b.rotating.Shutdown(ctx); err != nil {
			t.Fatal(err)
		}
		if err := b.rotating.ReapIdle(time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	f.check(t, 1, 1, 0)
}

func TestACPBridgeSharedGenerationWaitsForStop(t *testing.T) {
	b, f, cwd := rotatingBridgeFixture(t)
	ctx := context.Background()
	a, am := sharedBridgeAcquire(t, b, "a", cwd)
	if _, err := b.Release(ctx, a, am.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.stopEntered, f.stopGate = make(chan struct{}, 1), make(chan struct{})
	entered, gate := f.stopEntered, f.stopGate
	f.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- b.sweepLifecycle(time.Now().Add(time.Hour)) }()
	receive(t, entered)
	bounded, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := b.Acquire(bounded, a, ""); err == nil {
		t.Fatal("acquire passed terminal stop")
	}
	f.check(t, 1, 1, 1)
	close(gate)
	if err := receive(t, done); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.stopEntered, f.stopGate = nil, nil
	f.mu.Unlock()
	if _, err := b.Acquire(ctx, a, ""); err != nil {
		t.Fatal(err)
	}
	f.check(t, 2, 1, 1)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	f.check(t, 2, 2, 0)
}
