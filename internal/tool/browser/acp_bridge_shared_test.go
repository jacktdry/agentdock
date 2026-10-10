package browser

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

// Synthetic session behind the actual registry and ACPBridge, never npx.
type sharedBridgeSession struct {
	*fakeWorkerSession
	pages     map[float64]string
	next      float64
	calls     []leaseBackendCall
	failClose bool
}

func (s *sharedBridgeSession) Call(ctx context.Context, tool string, args map[string]any) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || ctx.Err() != nil {
		return nil, errors.New("session unavailable")
	}
	s.calls = append(s.calls, leaseBackendCall{tool: tool, args: maps.Clone(args)})
	switch tool {
	case "new_page":
		if args["background"] != true {
			return nil, errors.New("foreground page")
		}
		s.next++
		name := args["isolatedContext"].(string)
		s.pages[s.next] = name
		return leasePages(name, s.next), nil
	case "select_page":
		if !strings.HasPrefix(s.pages[args["pageId"].(float64)], "agentdock-pool-keeper-") || args["bringToFront"] != false {
			return nil, errors.New("unowned keeper selection")
		}
	case "close_page":
		page := args["pageId"].(float64)
		if s.failClose || len(s.pages) <= 1 || s.pages[page] == "" || strings.HasPrefix(s.pages[page], "agentdock-pool-keeper-") {
			return nil, errors.New("page cleanup failed")
		}
		delete(s.pages, page)
	default:
		if s.pages[args["pageId"].(float64)] == "" {
			return nil, errors.New("unowned page operation")
		}
	}
	return map[string]any{"page": args["pageId"]}, nil
}

func sharedBridgeFixture(t *testing.T, shared bool) (*ACPBridge, *sharedBridgeSession, *int, string) {
	t.Helper()
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := NewRoutePlanner(nil, catalog, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &sharedBridgeSession{fakeWorkerSession: fakeManagedSession(t, "shared"), pages: make(map[float64]string)}
	starts := 0 // Registry serializes the single shared startup; inspect after Acquire completes.
	registry := NewWorkerRegistry(WorkerDependencies{
		Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return ManagedEngineVersion, nil },
		Open:  func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) { starts++; return s, nil },
	})
	cwd := t.TempDir()
	var b *ACPBridge
	if shared {
		b, err = newACPBridgeSharedForWorkspace(planner, registry, cwd)
	} else {
		b, err = NewACPBridge(planner, registry)
	}
	if err != nil {
		t.Fatal(err)
	}
	// Disable only the test's background ticks, then invoke the same sweep manually.
	b.lifecycle.Close()
	t.Cleanup(func() {
		if err := b.Close(); err != nil {
			t.Error(err)
		}
	})
	return b, s, &starts, cwd
}

func sharedBridgeAcquire(t *testing.T, b *ACPBridge, session, cwd string) (string, LeaseMetadata) {
	t.Helper()
	token, err := b.RegisterSession(session, "codex", cwd)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := b.Acquire(context.Background(), token, "")
	if err != nil {
		t.Fatal(err)
	}
	return token, meta
}

func TestACPBridgeSharedOwnersAndLifecycle(t *testing.T) {
	b, s, starts, cwd := sharedBridgeFixture(t, true)
	ctx := context.Background()
	other, err := b.RegisterSession("other", "codex", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Acquire(ctx, other, ""); err == nil || *starts != 0 {
		t.Fatal("cross workspace started a worker")
	}
	a, am := sharedBridgeAcquire(t, b, "a", cwd)
	bt, bm := sharedBridgeAcquire(t, b, "b", cwd)
	if *starts != 1 || am.WorkerID == bm.WorkerID || am.PageID == bm.PageID || am.IsolationContextName == bm.IsolationContextName || am.BrowserLeaseID == bm.BrowserLeaseID {
		t.Fatal("shared worker or distinct lease identities missing")
	}
	for range 3 {
		for _, owner := range []struct {
			token string
			meta  LeaseMetadata
		}{{a, am}, {bt, bm}} {
			result, err := b.Call(ctx, owner.token, owner.meta.BrowserLeaseID, "take_snapshot", nil)
			if err != nil || fmt.Sprint(result["page"]) != owner.meta.PageID {
				t.Fatal("wrong page routing", err)
			}
		}
	}
	_, err = b.Call(ctx, a, am.BrowserLeaseID, "click", map[string]any{"pageId": 999.0})
	assertBrowserCode(t, err, ErrLeaseTargetMismatch)
	_, err = b.Call(ctx, bt, am.BrowserLeaseID, "click", nil)
	assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
	_, err = b.Call(ctx, a, am.BrowserLeaseID, "select_page", nil)
	assertBrowserCode(t, err, ErrActionInvalid)
	_, err = b.registry.Call(ctx, b.shared.worker.WorkerID, "select_page", map[string]any{"pageId": *b.shared.keeperPage, "bringToFront": false})
	assertBrowserCode(t, err, ErrEngineSchemaIncompatible)
	meta, err := b.Release(ctx, a, am.BrowserLeaseID)
	if err != nil || meta.CleanupState != CleanupComplete || meta.CleanupReason != "shared managed owned page closed and borrow ended" {
		t.Fatal("incorrect shared cleanup", meta, err)
	}
	if err := b.ReleaseSession(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Call(ctx, bt, bm.BrowserLeaseID, "click", nil); err != nil {
		t.Fatal("release killed sibling", err)
	}
	if err := b.managed.SweepExpired(time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := b.sweepLifecycle(time.Now()); err != nil {
		t.Fatal(err)
	} // Retention begins at actual page cleanup.
	s.mu.Lock()
	if s.closes != 0 || len(s.pages) != 1 {
		t.Error("premature worker stop or missing page cleanup")
	}
	selections := 0
	for _, call := range s.calls {
		if call.tool == "select_page" {
			selections++
			if call.args["pageId"] != *b.shared.keeperPage || call.args["bringToFront"] != false {
				t.Error("unsafe selection")
			}
		}
	}
	s.mu.Unlock()
	if selections != 2 {
		t.Fatal("private keeper selection missing")
	}
	if err := b.sweepLifecycle(time.Now().Add(2 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Acquire(ctx, bt, ""); err == nil {
		t.Fatal("terminal generation respawned")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if err := b.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closes != 1 || *starts != 1 {
		t.Fatal("worker must stop exactly once")
	}
}

func TestACPBridgeSharedPrivateSelectionRejectsUnexpectedUse(t *testing.T) {
	b, s, _, cwd := sharedBridgeFixture(t, true)
	_, meta := sharedBridgeAcquire(t, b, "a", cwd)
	a := b.shared.backend.(*sharedRegistryBackend)
	id := b.shared.worker.WorkerID
	var borrowed float64
	_, _ = fmt.Sscan(meta.PageID, &borrowed)
	for _, page := range []float64{math.NaN(), math.Inf(1), -1, 0.5, borrowed} {
		if _, err := a.Call(context.Background(), id, "select_page", map[string]any{"pageId": page, "bringToFront": false}); err == nil {
			t.Fatal("unexpected page accepted")
		}
	}
	for _, args := range []map[string]any{
		{"pageId": *b.shared.keeperPage, "bringToFront": true},
		{"pageId": *b.shared.keeperPage},
		{"pageId": *b.shared.keeperPage, "bringToFront": false, "extra": true},
	} {
		if _, err := a.Call(context.Background(), id, "select_page", args); err == nil {
			t.Fatal("unexpected selection accepted")
		}
	}
	if _, err := a.Call(context.Background(), "other-worker", "select_page", map[string]any{"pageId": *b.shared.keeperPage, "bringToFront": false}); err == nil {
		t.Fatal("worker substitution accepted")
	}
	w, _ := b.registry.worker(id)
	w.mu.Lock()
	original := w.info.Session
	w.info.Session.PID++
	w.mu.Unlock()
	_, err := a.Call(context.Background(), id, "select_page", map[string]any{"pageId": *b.shared.keeperPage, "bringToFront": false})
	w.mu.Lock()
	w.info.Session = original
	w.mu.Unlock()
	if err == nil {
		t.Fatal("mutable worker identity accepted")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, call := range s.calls {
		if call.tool == "select_page" {
			t.Fatal("rejected selection reached session")
		}
	}
}

func TestACPBridgeSharedFailureRecoveryAndClose(t *testing.T) {
	b, s, _, cwd := sharedBridgeFixture(t, true)
	a, meta := sharedBridgeAcquire(t, b, "a", cwd)
	s.mu.Lock()
	s.failClose = true
	s.mu.Unlock()
	cleanup, err := b.Release(context.Background(), a, meta.BrowserLeaseID)
	if err == nil || cleanup.CleanupState != CleanupFailed || strings.Contains(cleanup.CleanupReason, "exclusive") {
		t.Fatal("failed page cleanup reported complete", cleanup, err)
	}
	if err := b.sweepLifecycle(time.Now().Add(time.Hour)); err == nil {
		t.Fatal("failed recovery hidden")
	}
	// Reap has now stopped the owned worker; the subsequent recovery may complete.
	if err := b.sweepLifecycle(time.Now().Add(2 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	l, _ := b.managed.manager.lease(meta.BrowserLeaseID)
	l.mu.Lock()
	cleanup = l.metadata
	l.mu.Unlock()
	if cleanup.CleanupState != CleanupComplete || strings.Contains(cleanup.CleanupReason, "exclusive") {
		t.Fatal("shared recovery metadata", cleanup)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestACPBridgeSharedCloseActiveAndDefaultExclusive(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(fmt.Sprint(shared), func(t *testing.T) {
			b, s, _, cwd := sharedBridgeFixture(t, shared)
			_, meta := sharedBridgeAcquire(t, b, "a", cwd)
			if shared != (b.shared != nil) {
				t.Fatal("default constructor switched")
			}
			if !shared && b.managed.manager.backend != b.registry {
				t.Fatal("default no longer exclusive")
			}
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			l, _ := b.managed.manager.lease(meta.BrowserLeaseID)
			l.mu.Lock()
			cleanup := l.metadata
			l.mu.Unlock()
			if cleanup.CleanupState != CleanupComplete {
				t.Fatal("Close left active lease")
			}
			if !shared && cleanup.CleanupReason != "exclusive managed worker stopped" {
				t.Fatal("exclusive reason changed")
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.closes != 1 {
				t.Fatal("Close did not stop owned worker once")
			}
		})
	}
}
