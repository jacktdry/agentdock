package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

type budgetProcessFixture struct {
	mu                 sync.Mutex
	live, peak, starts int
}

type budgetSession struct {
	*generationSession
	processes *budgetProcessFixture
	once      sync.Once
}

func (s *budgetSession) Close() error {
	err := s.generationSession.Close()
	if err == nil {
		s.once.Do(func() {
			s.processes.mu.Lock()
			s.processes.live--
			s.processes.mu.Unlock()
		})
	}
	return err
}

func budgetBridgeFixture(t *testing.T, budget *managedSharedBrowserBudget, processes *budgetProcessFixture, launchFailure bool) (*ACPBridge, *generationFixture, string) {
	t.Helper()
	return budgetBridgeFixtureForRoot(t, budget, processes, launchFailure, t.TempDir())
}

func budgetBridgeFixtureForRoot(t *testing.T, budget *managedSharedBrowserBudget, processes *budgetProcessFixture, launchFailure bool, cwd string) (*ACPBridge, *generationFixture, string) {
	t.Helper()
	f := &generationFixture{}
	base := fakeManagedSession(t, "budget")
	registry := NewWorkerRegistry(WorkerDependencies{
		Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return ManagedEngineVersion, nil },
		Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) {
			processes.mu.Lock()
			processes.starts++
			processes.live++
			if processes.live > processes.peak {
				processes.peak = processes.live
			}
			processes.mu.Unlock()
			if launchFailure {
				// Simulate a process side effect without a returned session handle.
				return nil, errors.New("uncertain-launch-canary")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			s := &generationSession{sharedBridgeSession: &sharedBridgeSession{fakeWorkerSession: &fakeWorkerSession{info: base.info, tools: base.tools}, pages: make(map[float64]string)}, f: f}
			f.sessions = append(f.sessions, s)
			f.live++
			if f.live > f.peak {
				f.peak = f.live
			}
			return &budgetSession{generationSession: s, processes: processes}, nil
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
	b, err := newACPBridgeBudgetedRotatingSharedForWorkspace(planner, registry, cwd, budget)
	if err != nil {
		t.Fatal(err)
	}
	b.lifecycle.Close()
	t.Cleanup(func() { _ = b.Close() })
	return b, f, cwd
}

func assertBudgetOwners(t *testing.T, budget *managedSharedBrowserBudget, count int) {
	t.Helper()
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if len(budget.owners) != count {
		t.Fatalf("owners=%d, want %d", len(budget.owners), count)
	}
}

func assertBudgetReplacementDenied(t *testing.T, b *ACPBridge, cwd string) {
	t.Helper()
	for _, registry := range []*WorkerRegistry{b.registry, NewWorkerRegistry(WorkerDependencies{})} {
		if replacement, err := newACPBridgeBudgetedRotatingSharedForWorkspace(b.planner, registry, cwd, b.rotating.budget); err != errManagedSharedUnavailable {
			if replacement != nil {
				_ = replacement.Close()
			}
			t.Fatalf("replacement error=%v", err)
		}
	}
}

func TestACPBridgeSharedBudgetOwnerSequentialReplacement(t *testing.T) {
	budget, p, cwd := testBrowserBudget(t), &budgetProcessFixture{}, t.TempDir()
	var previous *ACPBridge
	var oldToken string
	var oldMeta LeaseMetadata
	for i := range 75 {
		b, f, _ := budgetBridgeFixtureForRoot(t, budget, p, false, cwd)
		assertBudgetOwners(t, budget, 1)
		assertBudgetReplacementDenied(t, b, cwd)
		if previous != nil && previous.registry == b.registry {
			t.Fatal("replacement reused registry")
		}
		token, meta := sharedBridgeAcquire(t, b, "same-session", cwd)
		if previous != nil {
			ctx := context.Background()
			if _, err := b.Acquire(ctx, oldToken, ""); err == nil {
				t.Fatal("old session token accepted by replacement")
			}
			if _, err := previous.Acquire(ctx, oldToken, ""); err == nil {
				t.Fatal("closed bridge accepted old session")
			}
			if _, err := b.rotating.Call(ctx, oldMeta.WorkerID, "click", nil); err == nil {
				t.Fatal("old worker borrow routed to replacement")
			}
			// A stale claim/duplicate Close cannot delete the new root claimant.
			budget.releaseOwner(previous.registry, previous.budgetOwnerClaim)
			if err := previous.Close(); err != nil {
				t.Fatal(err)
			}
			assertBudgetOwners(t, budget, 1)
		}
		for range 2 {
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
		}
		assertBudgetOwners(t, budget, 0)
		assertBudget(t, budget, p, 0, i+1, 0)
		f.check(t, 1, 1, 0)
		previous, oldToken, oldMeta = b, token, meta
	}
}

func TestACPBridgeSharedBudgetOwnerClaimIdentity(t *testing.T) {
	budget := testBrowserBudget(t)
	registry := NewWorkerRegistry(WorkerDependencies{})
	root := t.TempDir()
	old, err := budget.claim(registry, root)
	if err != nil {
		t.Fatal(err)
	}
	budget.releaseOwner(registry, old)
	current, err := budget.claim(registry, root)
	if err != nil {
		t.Fatal(err)
	}
	budget.releaseOwner(registry, old)
	budget.releaseOwner(registry, &managedSharedBudgetOwnerClaim{root: root})
	assertBudgetOwners(t, budget, 1)
	budget.releaseOwner(registry, current)
	budget.releaseOwner(registry, current)
	assertBudgetOwners(t, budget, 0)
}

func TestACPBridgeSharedBudgetOwnerFailedClose(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(fmt.Sprint(uncertain), func(t *testing.T) {
			budget, p := testBrowserBudget(t), &budgetProcessFixture{}
			b, f, cwd := budgetBridgeFixture(t, budget, p, uncertain)
			if uncertain {
				token, err := b.RegisterSession("a", "codex", cwd)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := b.Acquire(context.Background(), token, ""); err == nil {
					t.Fatal("uncertain launch accepted")
				}
			} else {
				sharedBridgeAcquire(t, b, "a", cwd)
				f.mu.Lock()
				f.failStop = true
				f.mu.Unlock()
			}
			for range 2 {
				if err := b.Close(); err == nil {
					t.Fatal("failed cleanup reported success")
				}
				assertBudgetOwners(t, budget, 1)
				assertBudget(t, budget, p, 1, 1, 1)
				assertBudgetReplacementDenied(t, b, cwd)
			}
		})
	}
}

func TestACPBridgeSharedBudgetOwnerBlockedClose(t *testing.T) {
	for _, extraWorker := range []bool{false, true} {
		t.Run(fmt.Sprint(extraWorker), func(t *testing.T) {
			budget, p := testBrowserBudget(t), &budgetProcessFixture{}
			b, f, cwd := budgetBridgeFixture(t, budget, p, false)
			if extraWorker {
				// No rotating generation: registry Shutdown must still wait for
				// another live owned browser before releasing the owner claim.
				if _, err := b.registry.StartManaged(context.Background(), b.rotating.options); err != nil {
					t.Fatal(err)
				}
			} else {
				sharedBridgeAcquire(t, b, "a", cwd)
			}
			entered, gate := make(chan struct{}, 1), make(chan struct{})
			var unblock sync.Once
			defer unblock.Do(func() { close(gate) })
			f.mu.Lock()
			f.stopEntered, f.stopGate = entered, gate
			f.mu.Unlock()
			done := make(chan error, 2)
			go func() { done <- b.Close() }()
			receive(t, entered)
			go func() { done <- b.Close() }()
			var wg sync.WaitGroup
			for range 12 {
				wg.Go(func() { assertBudgetReplacementDenied(t, b, cwd) })
			}
			wg.Wait()
			assertBudgetOwners(t, budget, 1)
			used := 1
			if extraWorker {
				used = 0
			}
			assertBudget(t, budget, p, used, 1, 1)
			select {
			case err := <-done:
				t.Fatal("Close completed before Stop", err)
			default:
			}
			unblock.Do(func() { close(gate) })
			for range 2 {
				if err := receive(t, done); err != nil {
					t.Fatal(err)
				}
			}
			assertBudgetOwners(t, budget, 0)
			assertBudget(t, budget, p, 0, 1, 0)
			replacement, _, _ := budgetBridgeFixtureForRoot(t, budget, p, false, cwd)
			if err := replacement.Close(); err != nil {
				t.Fatal(err)
			}
			assertBudgetOwners(t, budget, 0)
		})
	}
}

func TestACPBridgeSharedBudgetOwnerInvalidConstruction(t *testing.T) {
	budget, p := testBrowserBudget(t), &budgetProcessFixture{}
	b, _, cwd := budgetBridgeFixture(t, budget, p, false)
	for _, root := range []string{"", filepath.Join(cwd, "missing")} {
		if _, err := newACPBridgeBudgetedRotatingSharedForWorkspace(b.planner, NewWorkerRegistry(WorkerDependencies{}), root, budget); err != errManagedSharedUnavailable {
			t.Fatalf("invalid root error=%v", err)
		}
		assertBudgetOwners(t, budget, 1)
	}
	assertBudget(t, budget, p, 0, 0, 0)
}

func testBrowserBudget(t *testing.T) *managedSharedBrowserBudget {
	t.Helper()
	b, err := newManagedSharedBrowserBudget(2)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func assertBudget(t *testing.T, budget *managedSharedBrowserBudget, processes *budgetProcessFixture, used, starts, live int) {
	t.Helper()
	budget.mu.Lock()
	defer budget.mu.Unlock()
	processes.mu.Lock()
	defer processes.mu.Unlock()
	if budget.used != used || processes.starts != starts || processes.live != live || processes.peak > 2 {
		t.Fatalf("used=%d starts=%d live=%d peak=%d", budget.used, processes.starts, processes.live, processes.peak)
	}
}

func TestACPBridgeSharedBudgetWorkspaceIsolation(t *testing.T) {
	budget, p := testBrowserBudget(t), &budgetProcessFixture{}
	a, af, acwd := budgetBridgeFixture(t, budget, p, false)
	b, bf, bcwd := budgetBridgeFixture(t, budget, p, false)
	c, cf, ccwd := budgetBridgeFixture(t, budget, p, false)
	ctx := context.Background()
	// Parent aliases pin the same canonical root, never a second process/profile.
	alias := filepath.Join(t.TempDir(), "parent")
	if err := os.Symlink(acwd, alias); err != nil {
		t.Fatal(err)
	}
	a1, am1 := sharedBridgeAcquire(t, a, "a1", alias)
	a2, am2 := sharedBridgeAcquire(t, a, "a2", acwd)
	b1, bm1 := sharedBridgeAcquire(t, b, "b1", bcwd)
	b2, bm2 := sharedBridgeAcquire(t, b, "b2", bcwd)
	root, err := browserpolicy.CanonicalWorkspaceRoot(acwd)
	if err != nil || a.rotating.options.Cwd != root {
		t.Fatal("alias lost canonical root", err)
	}
	af.check(t, 1, 0, 1)
	bf.check(t, 1, 0, 1)
	assertBudget(t, budget, p, 2, 2, 2)
	ct, err := c.RegisterSession("c", "codex", ccwd)
	if err != nil {
		t.Fatal(err)
	}
	denyThird := func() {
		t.Helper()
		if _, err := c.Acquire(ctx, ct, ""); err == nil {
			t.Fatal("full budget admitted third")
		}
		cf.check(t, 0, 0, 0)
		c.registry.mu.Lock()
		defer c.registry.mu.Unlock()
		if len(c.registry.workers) != 0 {
			t.Fatal("full budget dispatched registry StartManaged")
		}
	}
	denyThird()
	for _, x := range []struct {
		bridge *ACPBridge
		token  string
		meta   LeaseMetadata
	}{{a, a1, am1}, {a, a2, am2}, {b, b1, bm1}, {b, b2, bm2}} {
		result, err := x.bridge.Call(ctx, x.token, x.meta.BrowserLeaseID, "take_snapshot", nil)
		if err != nil || fmt.Sprint(result["page"]) != x.meta.PageID {
			t.Fatal("mixed page data", err)
		}
	}
	if am1.PageID == am2.PageID || am1.IsolationContextName == am2.IsolationContextName || am1.IsolationContextName == bm1.IsolationContextName {
		t.Fatal("page/context mixing")
	}
	if _, err := b.Call(ctx, a1, am1.BrowserLeaseID, "click", nil); err == nil {
		t.Fatal("foreign token accepted")
	}
	if _, err := b.rotating.Call(ctx, am1.WorkerID, "click", map[string]any{"pageId": 2.0}); err == nil {
		t.Fatal("foreign borrow accepted")
	}
	if _, err := a.Release(ctx, a1, am1.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	denyThird() // A borrow release is not a process release.
	if _, err := a.Release(ctx, a2, am2.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	denyThird() // Idle retention still owns the slot.
	g := a.rotating.generation
	g.mu.Lock()
	idleAt := g.idleAt
	g.mu.Unlock()
	if err := a.rotating.ReapIdle(idleAt.Add(30*time.Second - time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
	denyThird()
	for range 3 {
		if err := a.rotating.ReapIdle(idleAt.Add(30 * time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	assertBudget(t, budget, p, 1, 2, 1)
	cm, err := c.Acquire(ctx, ct, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Call(ctx, ct, cm.BrowserLeaseID, "click", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Acquire(ctx, a1, ""); err == nil {
		t.Fatal("replacement bypassed new reservation")
	}
	af.check(t, 1, 1, 0)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Acquire(ctx, a1, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.rotating.Call(ctx, am1.WorkerID, "click", map[string]any{"pageId": 2.0}); err == nil {
		t.Fatal("old borrow routed to replacement")
	}
	if _, err := g.Call(ctx, am1.WorkerID, "click", map[string]any{"pageId": 2.0}); err == nil {
		t.Fatal("old worker routed")
	}
	af.check(t, 2, 1, 1)
	assertBudget(t, budget, p, 2, 4, 2)
	for range 3 {
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
	}
	// A's Close must not stop B's independent registry or free B's slot.
	if _, err := b.Call(ctx, b1, bm1.BrowserLeaseID, "click", nil); err != nil {
		t.Fatal(err)
	}
	bf.check(t, 1, 0, 1)
	assertBudget(t, budget, p, 1, 4, 1)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	assertBudget(t, budget, p, 0, 4, 0)
}

func TestACPBridgeSharedBudgetConcurrentAdmission(t *testing.T) {
	budget, p := testBrowserBudget(t), &budgetProcessFixture{}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan bool, 12)
	var bridges []*ACPBridge
	for i := range 6 {
		b, _, cwd := budgetBridgeFixture(t, budget, p, false)
		bridges = append(bridges, b)
		for j := range 2 {
			token, err := b.RegisterSession(fmt.Sprintf("owner-%d-%d", i, j), "codex", cwd)
			if err != nil {
				t.Fatal(err)
			}
			wg.Go(func() { <-start; _, err := b.Acquire(context.Background(), token, ""); results <- err == nil })
		}
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for ok := range results {
		if ok {
			successes++
		}
	}
	if successes != 4 {
		t.Fatalf("admitted owners=%d, want two per admitted workspace", successes)
	}
	assertBudget(t, budget, p, 2, 2, 2)
	for _, b := range bridges {
		for range 2 {
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	assertBudget(t, budget, p, 0, 2, 0)
}

func TestACPBridgeSharedBudgetStopFailureAndPending(t *testing.T) {
	for _, blocking := range []bool{false, true} {
		t.Run(fmt.Sprint(blocking), func(t *testing.T) {
			budget, p := testBrowserBudget(t), &budgetProcessFixture{}
			a, af, cwd := budgetBridgeFixture(t, budget, p, false)
			b, _, bcwd := budgetBridgeFixture(t, budget, p, false)
			c, cf, ccwd := budgetBridgeFixture(t, budget, p, false)
			at, am := sharedBridgeAcquire(t, a, "a", cwd)
			bt, bm := sharedBridgeAcquire(t, b, "b", bcwd)
			ct, err := c.RegisterSession("c", "codex", ccwd)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if _, err := a.Release(ctx, at, am.BrowserLeaseID); err != nil {
				t.Fatal(err)
			}
			af.mu.Lock()
			af.failStop = !blocking
			entered, gate := make(chan struct{}, 1), make(chan struct{})
			if blocking {
				af.stopEntered, af.stopGate = entered, gate
			}
			af.mu.Unlock()
			done := make(chan error, 1)
			go func() { done <- a.rotating.ReapIdle(time.Now().Add(time.Hour)) }()
			if blocking {
				receive(t, entered)
				bounded, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
				if err := a.rotating.Shutdown(bounded); err == nil {
					t.Error("pending Stop reported success")
				}
				cancel()
			} else if err := receive(t, done); err == nil {
				t.Fatal("failed Stop reported success")
			}
			if _, err := c.Acquire(ctx, ct, ""); err == nil {
				t.Error("pending/failed Stop freed slot")
			}
			cf.check(t, 0, 0, 0)
			assertBudget(t, budget, p, 2, 2, 2)
			if _, err := b.Call(ctx, bt, bm.BrowserLeaseID, "click", nil); err != nil {
				t.Error("sibling stopped", err)
			}
			if blocking {
				close(gate)
				if err := receive(t, done); err != nil {
					t.Fatal(err)
				}
				for range 3 {
					if err := a.Close(); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := c.Acquire(ctx, ct, ""); err != nil {
					t.Fatal(err)
				}
				assertBudget(t, budget, p, 2, 3, 2)
			} else {
				for range 2 {
					if err := a.Close(); err == nil {
						t.Fatal("shutdown failure hidden")
					}
					if err := a.rotating.ReapIdle(time.Now().Add(time.Hour)); err == nil {
						t.Fatal("failed reap succeeded")
					}
				}
				assertBudget(t, budget, p, 2, 2, 2)
			}
		})
	}
}

func TestACPBridgeSharedBudgetUncertainStartup(t *testing.T) {
	budget, p := testBrowserBudget(t), &budgetProcessFixture{}
	a, _, cwd := budgetBridgeFixture(t, budget, p, true)
	b, _, bcwd := budgetBridgeFixture(t, budget, p, false)
	c, cf, ccwd := budgetBridgeFixture(t, budget, p, false)
	at, err := a.RegisterSession("a", "codex", cwd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Acquire(context.Background(), at, ""); err == nil {
		t.Fatal("failed launch accepted")
	}
	// Wait for the finalizer whose registry Stop succeeds on a nil session.
	receive(t, a.rotating.generation.done)
	sharedBridgeAcquire(t, b, "b", bcwd)
	for range 3 {
		if err := a.rotating.ReapIdle(time.Now().Add(time.Hour)); err == nil {
			t.Fatal("uncertain launch reported stopped")
		}
		if _, err := a.Acquire(context.Background(), at, ""); err == nil {
			t.Fatal("uncertain generation replaced")
		}
	}
	if err := a.Close(); err == nil {
		t.Fatal("uncertainty hidden by Close")
	}
	ct, err := c.RegisterSession("c", "codex", ccwd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Acquire(context.Background(), ct, ""); err == nil {
		t.Fatal("uncertain startup released slot")
	}
	cf.check(t, 0, 0, 0)
	assertBudget(t, budget, p, 2, 2, 2)
}

func TestManagedSharedBudgetConstructorOwnership(t *testing.T) {
	for _, size := range []int{-1, 0} {
		if _, err := newManagedSharedBrowserBudget(size); err == nil {
			t.Fatal("invalid capacity")
		}
	}
	budget, p := testBrowserBudget(t), &budgetProcessFixture{}
	b, _, cwd := budgetBridgeFixture(t, budget, p, false)
	if _, err := newACPBridgeBudgetedRotatingSharedForWorkspace(b.planner, b.registry, t.TempDir(), budget); err == nil {
		t.Fatal("shared registry lifecycle accepted")
	}
	if _, err := newACPBridgeBudgetedRotatingSharedForWorkspace(b.planner, NewWorkerRegistry(WorkerDependencies{}), cwd, budget); err == nil {
		t.Fatal("duplicate canonical owner accepted")
	}
	assertBudget(t, budget, p, 0, 0, 0)
}
