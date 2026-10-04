package browser

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

type leaseBackendCall struct {
	worker, tool string
	args         map[string]any
}
type fakeLeaseBackend struct {
	mu                                           sync.Mutex
	calls                                        []leaseBackendCall
	starts                                       int
	stopped                                      []string
	response                                     func(string, string) map[string]any
	startErr, newErr, closeErr, stopErr, callErr error
	entered, gate                                chan struct{}
}

func (b *fakeLeaseBackend) StartManaged(_ context.Context, options ManagedWorkerOptions) (WorkerInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.starts++
	return WorkerInfo{WorkerID: fmt.Sprint(b.starts), State: WorkerReady, Session: mcpclient.SessionInfo{PID: 123}}, b.startErr
}
func (b *fakeLeaseBackend) Call(_ context.Context, worker, tool string, args map[string]any) (map[string]any, error) {
	b.mu.Lock()
	b.calls = append(b.calls, leaseBackendCall{worker, tool, args})
	b.mu.Unlock()
	switch tool {
	case "new_page":
		if b.newErr != nil {
			return nil, b.newErr
		}
		isolation := args["isolatedContext"].(string)
		if b.response != nil {
			return b.response(worker, isolation), nil
		}
		var id float64
		_, _ = fmt.Sscan(worker, &id)
		return leasePages(isolation, id), nil
	case "close_page":
		return nil, b.closeErr
	default:
		if b.entered != nil {
			b.entered <- struct{}{}
			<-b.gate
		}
		b.mu.Lock()
		b.calls = append(b.calls, leaseBackendCall{worker: worker, tool: "operation_complete"})
		b.mu.Unlock()
		return map[string]any{"worker": worker, "args": args}, b.callErr
	}
}
func (b *fakeLeaseBackend) Stop(worker string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopped = append(b.stopped, worker)
	b.calls = append(b.calls, leaseBackendCall{worker: worker, tool: "stop"})
	return b.stopErr
}
func leasePages(isolation string, id any) map[string]any {
	return map[string]any{"structuredContent": map[string]any{"pages": []any{map[string]any{"id": id, "isolatedContext": isolation}}}}
}
func leaseScope() RequestScope {
	return RequestScope{WorkspaceID: "workspace", CanonicalWorkspaceRoot: "/workspace", OwnerTaskID: "task", OwnerSessionID: "session", OwnerACPSessionID: "acp", Provenance: ScopeACP}
}
func leaseStart(t *testing.T) ResolvedStart {
	t.Helper()
	decision, err := ResolveRoute(RouteRequest{Scope: leaseScope(), Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceDefault, Route: browserpolicy.RouteManaged}})
	if err != nil {
		t.Fatal(err)
	}
	return decision.Start
}
func acquireLease(t *testing.T, m *ManagedLeaseManager) (LeaseMetadata, EngineBinding) {
	t.Helper()
	meta, binding, err := m.Acquire(context.Background(), leaseScope(), leaseStart(t), "")
	if err != nil {
		t.Fatal(err)
	}
	return meta, binding
}
func TestManagedLeaseIsolationAndOperations(t *testing.T) {
	b := &fakeLeaseBackend{}
	m := NewManagedLeaseManager(b)
	a, ab := acquireLease(t, m)
	c, cb := acquireLease(t, m)
	if a.BrowserLeaseID == c.BrowserLeaseID || a.BrowserSessionID == c.BrowserSessionID || a.IsolationContextName == c.IsolationContextName || a.WorkerID == c.WorkerID || a.PageID == c.PageID {
		t.Fatal("lease identities reused")
	}
	for _, pair := range []struct {
		meta    LeaseMetadata
		binding EngineBinding
	}{{a, ab}, {c, cb}} {
		meta, binding := pair.meta, pair.binding
		if meta.BrowserContextID != "" || binding.BrowserContextID != "" || meta.BrowserPID.PID != 0 || meta.ConnectorPID.PID != 123 || meta.CleanupState != CleanupPending || meta.CreatedAt.IsZero() || meta.ExpiresAt != (time.Time{}) || meta.OwnerType != OwnerAgentDockIsolated || meta.ProfileClass != ProfileIsolated || meta.LifecyclePolicy != LifecycleOwned || meta.ForegroundPolicy != ForegroundForbidden {
			t.Fatalf("metadata: %+v", meta)
		}
		for _, tool := range []string{"navigate_page", "take_snapshot", "take_screenshot", "evaluate_script", "click", "fill", "press_key"} {
			args := map[string]any{"value": "test"}
			result, err := m.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, tool, args)
			if err != nil || result["worker"] != binding.WorkerID {
				t.Fatalf("target: %v %v", result, err)
			}
			if _, exists := args["pageId"]; exists {
				t.Fatal("caller args mutated")
			}
			if fmt.Sprint(result["args"].(map[string]any)["pageId"]) != meta.PageID {
				t.Fatal("wrong page")
			}
		}
	}
	for _, call := range b.calls[:2] {
		if call.tool != "new_page" || call.args["background"] != true || call.args["url"] != "about:blank" {
			t.Fatalf("%+v", call)
		}
	}
	l, _ := m.lease(a.BrowserLeaseID)
	if !l.metadata.LastActiveAt.After(a.LastActiveAt) {
		t.Fatal("activity not recorded")
	}
	before := l.metadata.LastActiveAt
	b.callErr = errors.New("call failed")
	_, err := m.Call(context.Background(), leaseScope(), a.BrowserLeaseID, "click", nil)
	if err == nil || l.metadata.LastActiveAt != before {
		t.Fatal("failed operation updated activity")
	}
}
func TestManagedLeaseOwnerAndInputRejection(t *testing.T) {
	b := &fakeLeaseBackend{}
	m := NewManagedLeaseManager(b)
	a, _ := acquireLease(t, m)
	mutations := []func(*RequestScope){func(s *RequestScope) { s.WorkspaceID = "other" }, func(s *RequestScope) { s.CanonicalWorkspaceRoot = "/other" }, func(s *RequestScope) { s.Provenance = ScopeRuntime }, func(s *RequestScope) { s.OwnerTaskID = "other" }, func(s *RequestScope) { s.OwnerSessionID = "other" }, func(s *RequestScope) { s.OwnerACPSessionID = "other" }, func(s *RequestScope) { s.OwnerTaskID = ""; s.OwnerSessionID = ""; s.OwnerACPSessionID = "" }}
	for _, mutate := range mutations {
		scope := leaseScope()
		mutate(&scope)
		_, err := m.Call(context.Background(), scope, a.BrowserLeaseID, "click", map[string]any{"pageId": 5})
		assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
		_, err = m.Release(context.Background(), scope, a.BrowserLeaseID)
		assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
	}
	for _, tool := range []string{"new_page", "list_pages", "close_page", "select_page", "unknown"} {
		_, err := m.Call(context.Background(), leaseScope(), a.BrowserLeaseID, tool, nil)
		assertBrowserCode(t, err, ErrActionInvalid)
	}
	_, err := m.Call(context.Background(), leaseScope(), a.BrowserLeaseID, "click", map[string]any{"pageId": nil})
	assertBrowserCode(t, err, ErrLeaseTargetMismatch)
	_, err = m.Call(context.Background(), leaseScope(), "missing", "click", nil)
	assertBrowserCode(t, err, ErrLeaseNotFound)
	_, err = m.Release(context.Background(), leaseScope(), "missing")
	assertBrowserCode(t, err, ErrLeaseNotFound)
	if len(b.calls) != 1 {
		t.Fatal("rejected requests reached worker")
	}
}
func TestManagedLeaseAcquireFailureCleanup(t *testing.T) {
	responses := []map[string]any{nil, {"content": "## Pages"}, {"structuredContent": map[string]any{"pages": "bad"}}, leasePages("other", float64(1)), {"structuredContent": map[string]any{"pages": []any{nil}}}}
	for _, id := range []any{1.5, float64(-1), math.NaN(), math.Inf(1), math.Inf(-1), "1", nil, float64(9007199254740992)} {
		responses = append(responses, leasePages("context", id))
	}
	duplicate := leasePages("context", float64(1))
	pages := duplicate["structuredContent"].(map[string]any)
	pages["pages"] = append(pages["pages"].([]any), map[string]any{"id": float64(2), "isolatedContext": "context"})
	responses = append(responses, duplicate)
	for i, response := range responses {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			b := &fakeLeaseBackend{response: func(_, isolation string) map[string]any {
				if structured, ok := response["structuredContent"].(map[string]any); ok {
					if pages, ok := structured["pages"].([]any); ok {
						for _, entry := range pages {
							if page, ok := entry.(map[string]any); ok && page["isolatedContext"] == "context" {
								page["isolatedContext"] = isolation
							}
						}
					}
				}
				return response
			}}
			m := NewManagedLeaseManager(b)
			meta, _, err := m.Acquire(context.Background(), leaseScope(), leaseStart(t), "")
			assertBrowserCode(t, err, ErrLeaseTargetMismatch)
			if meta.BrowserLeaseID != "" || len(m.leases) != 0 || !reflect.DeepEqual(b.stopped, []string{"1"}) {
				t.Fatal("failed acquisition published or leaked")
			}
		})
	}
	for _, phase := range []string{"start", "new", "mcp"} {
		t.Run(phase, func(t *testing.T) {
			b := &fakeLeaseBackend{}
			switch phase {
			case "start":
				b.startErr = errors.New("start failed")
			case "new":
				b.newErr = errors.New("new failed")
			case "mcp":
				b.response = func(string, string) map[string]any { return map[string]any{"isError": true} }
			}
			m := NewManagedLeaseManager(b)
			_, _, err := m.Acquire(context.Background(), leaseScope(), leaseStart(t), "")
			if err == nil || len(m.leases) != 0 || len(b.stopped) != 1 {
				t.Fatal("acquire failure leaked")
			}
		})
	}
}
func TestManagedLeaseRejectConflictingStart(t *testing.T) {
	mutations := []func(*ResolvedStart){func(s *ResolvedStart) { s.Engine = EngineNativeCDP }, func(s *ResolvedStart) { s.EngineVersion = "other" }, func(s *ResolvedStart) { s.Browser = BrowserEdge }, func(s *ResolvedStart) { s.ProfileClass = ProfileExternal }, func(s *ResolvedStart) { s.Endpoint = "external" }, func(s *ResolvedStart) { s.ProfilePath = "external" }, func(s *ResolvedStart) { s.ProfileID = "external" }, func(s *ResolvedStart) { s.ConnectorID = "external" }, func(s *ResolvedStart) { s.Ownership.Process = OwnerExternalPersistent }, func(s *ResolvedStart) { s.Headless = false }, func(s *ResolvedStart) { s.BackgroundPage = false }, func(s *ResolvedStart) { s.LifecyclePolicy = LifecycleExternal }, func(s *ResolvedStart) { s.ForegroundPolicy = "allowed" }}
	b := &fakeLeaseBackend{}
	m := NewManagedLeaseManager(b)
	for _, mutate := range mutations {
		start := leaseStart(t)
		mutate(&start)
		_, _, err := m.Acquire(context.Background(), leaseScope(), start, "")
		assertBrowserCode(t, err, ErrPolicyConflict)
	}
	_, _, err := m.Acquire(context.Background(), RequestScope{}, leaseStart(t), "")
	assertBrowserCode(t, err, ErrScopeRequired)
	if b.starts != 0 {
		t.Fatal("rejected route started worker")
	}
}
func TestManagedLeaseReleaseCleanup(t *testing.T) {
	for _, failure := range []string{"none", "close", "stop", "both"} {
		t.Run(failure, func(t *testing.T) {
			b := &fakeLeaseBackend{}
			if failure == "close" || failure == "both" {
				b.closeErr = errors.New("close failed")
			}
			if failure == "stop" || failure == "both" {
				b.stopErr = errors.New("stop failed")
			}
			m := NewManagedLeaseManager(b)
			a, _ := acquireLease(t, m)
			meta, err := m.Release(context.Background(), leaseScope(), a.BrowserLeaseID)
			if b.stopErr != nil {
				assertBrowserCode(t, err, ErrActionFailed)
				if meta.CleanupState != CleanupFailed {
					t.Fatal("not failed")
				}
			} else if err != nil || meta.CleanupState != CleanupComplete {
				t.Fatalf("%+v %v", meta, err)
			}
			if failure != "none" && (meta.CleanupError == "" || meta.CleanupReason == "") {
				t.Fatal("missing diagnostic")
			}
			if b.stopErr != nil {
				repeated, err := m.Release(context.Background(), leaseScope(), a.BrowserLeaseID)
				assertBrowserCode(t, err, ErrActionFailed)
				if repeated != meta || len(b.calls) != 3 {
					t.Fatal("failed cleanup lost its diagnostic or retried implicitly")
				}
			}
			if b.calls[1].tool != "close_page" || fmt.Sprint(b.calls[1].args["pageId"]) != a.PageID || b.calls[2].tool != "stop" || b.calls[2].worker != a.WorkerID {
				t.Fatal("cleanup order/target")
			}
			_, err = m.Call(context.Background(), leaseScope(), a.BrowserLeaseID, "click", nil)
			assertBrowserCode(t, err, ErrLeaseStateInvalid)
			if b.stopErr == nil {
				_, err = m.Release(context.Background(), leaseScope(), a.BrowserLeaseID)
				if err != nil || len(b.calls) != 3 {
					t.Fatal("not idempotent")
				}
			}
		})
	}
}
func TestManagedLeaseReleaseSerializesWithOperation(t *testing.T) {
	b := &fakeLeaseBackend{entered: make(chan struct{}, 2), gate: make(chan struct{})}
	m := NewManagedLeaseManager(b)
	a, _ := acquireLease(t, m)
	other, _ := acquireLease(t, m)
	callDone := make(chan error, 1)
	go func() {
		_, err := m.Call(context.Background(), leaseScope(), a.BrowserLeaseID, "click", nil)
		callDone <- err
	}()
	<-b.entered
	// Another lease reaches its backend while the first is blocked.
	otherDone := make(chan error, 1)
	go func() {
		_, err := m.Call(context.Background(), leaseScope(), other.BrowserLeaseID, "click", nil)
		otherDone <- err
	}()
	select {
	case <-b.entered:
	case <-time.After(time.Second):
		t.Fatal("leases globally serialized")
	}
	releaseStarted := make(chan struct{})
	releaseDone := make(chan error, 1)
	go func() {
		close(releaseStarted)
		_, err := m.Release(context.Background(), leaseScope(), a.BrowserLeaseID)
		releaseDone <- err
	}()
	<-releaseStarted
	select {
	case <-releaseDone:
		t.Fatal("release raced call")
	case <-time.After(20 * time.Millisecond):
	}
	b.mu.Lock()
	if len(b.stopped) != 0 {
		t.Error("stopped during operation")
	}
	b.mu.Unlock()
	close(b.gate)
	for _, done := range []chan error{callDone, otherDone, releaseDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("deadlock")
		}
	}
	// The event sequence verifies cleanup follows completion regardless of scheduling.
	completed := false
	for _, call := range b.calls {
		if call.worker != a.WorkerID {
			continue
		}
		if call.tool == "operation_complete" {
			completed = true
		}
		if (call.tool == "close_page" || call.tool == "stop") && !completed {
			t.Fatal("cleanup preceded operation completion")
		}
	}
	l, _ := m.lease(a.BrowserLeaseID)
	l.mu.Lock()
	l.metadata.CleanupState = CleanupReleasing
	l.mu.Unlock()
	_, err := m.Call(context.Background(), leaseScope(), a.BrowserLeaseID, "click", nil)
	assertBrowserCode(t, err, ErrLeaseStateInvalid)
}

func TestManagedLeaseConcurrentAcquireCallRelease(t *testing.T) {
	b := &fakeLeaseBackend{}
	m := NewManagedLeaseManager(b)
	start := leaseStart(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			meta, _, err := m.Acquire(context.Background(), leaseScope(), start, "https://example.test")
			if err != nil {
				t.Error(err)
				return
			}
			result, err := m.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "take_snapshot", nil)
			if err != nil {
				t.Error(err)
			} else if result["worker"] != meta.WorkerID || fmt.Sprint(result["args"].(map[string]any)["pageId"]) != meta.PageID {
				t.Error("concurrent operation migrated across lease targets")
			}
			var releases sync.WaitGroup
			for j := 0; j < 2; j++ {
				releases.Add(1)
				go func() {
					defer releases.Done()
					_, err := m.Release(context.Background(), leaseScope(), meta.BrowserLeaseID)
					if err != nil {
						t.Error(err)
					}
				}()
			}
			releases.Wait()
		}()
	}
	wg.Wait()
	if b.starts != 16 || len(b.stopped) != 16 {
		t.Fatalf("starts=%d stops=%d", b.starts, len(b.stopped))
	}
	workers := map[string]bool{}
	for _, id := range b.stopped {
		if workers[id] {
			t.Fatal("worker stopped twice")
		}
		workers[id] = true
	}
}
func TestManagedLeaseStructuredPageZero(t *testing.T) {
	result := leasePages("context", float64(0))
	pages := result["structuredContent"].(map[string]any)
	pages["pages"] = append(pages["pages"].([]any), map[string]any{"id": float64(9), "isolatedContext": "other", "selected": true})
	id, opaque, err := isolatedPageID(result, "context")
	if err != nil || id != 0 || opaque != "0" {
		t.Fatalf("%v %q %v", id, opaque, err)
	}
}

func TestManagedLeaseRejectAddedOwnerIdentity(t *testing.T) {
	b := &fakeLeaseBackend{}
	m := NewManagedLeaseManager(b)
	scope := leaseScope()
	scope.OwnerTaskID, scope.OwnerSessionID = "", ""
	meta, _, err := m.Acquire(context.Background(), scope, leaseStart(t), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RequestScope){
		func(s *RequestScope) { s.OwnerTaskID = "other" },
		func(s *RequestScope) { s.OwnerSessionID = "other" },
	} {
		other := scope
		mutate(&other)
		_, err := m.Call(context.Background(), other, meta.BrowserLeaseID, "click", nil)
		assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
		_, err = m.Release(context.Background(), other, meta.BrowserLeaseID)
		assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
	}
	if len(b.calls) != 1 || len(b.stopped) != 0 {
		t.Fatal("owner mismatch reached backend")
	}
}

func TestManagedLeaseRejectAmbiguousPageIdentity(t *testing.T) {
	for _, extra := range []map[string]any{
		{"id": float64(1), "isolatedContext": "other"},
		{"id": float64(2), "isolatedContext": true},
		{"id": float64(2), "isolatedContext": ""},
		{"id": "2", "isolatedContext": "other"},
	} {
		b := &fakeLeaseBackend{response: func(_, isolation string) map[string]any {
			result := leasePages(isolation, float64(1))
			structured := result["structuredContent"].(map[string]any)
			structured["pages"] = append(structured["pages"].([]any), extra)
			return result
		}}
		m := NewManagedLeaseManager(b)
		_, _, err := m.Acquire(context.Background(), leaseScope(), leaseStart(t), "")
		assertBrowserCode(t, err, ErrLeaseTargetMismatch)
		if len(m.leases) != 0 || len(b.stopped) != 1 {
			t.Fatal("ambiguous target published or leaked")
		}
	}
}

func TestManagedLeaseCanceledAcquireCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := &fakeLeaseBackend{response: func(_, isolation string) map[string]any {
		cancel()
		return leasePages(isolation, float64(1))
	}}
	m := NewManagedLeaseManager(b)
	_, _, err := m.Acquire(ctx, leaseScope(), leaseStart(t), "")
	if !errors.Is(err, context.Canceled) || len(m.leases) != 0 || len(b.stopped) != 1 {
		t.Fatalf("canceled acquisition: %v", err)
	}
}

type leaseWorkerSession struct{ *fakeWorkerSession }

func (s *leaseWorkerSession) Call(ctx context.Context, tool string, args map[string]any) (map[string]any, error) {
	if tool == "new_page" {
		// Separate MCP generations may both issue page 1.
		return leasePages(args["isolatedContext"].(string), float64(1)), nil
	}
	return s.fakeWorkerSession.Call(ctx, tool, args)
}

func TestManagedLeaseStaleWorkerCannotMigratePage(t *testing.T) {
	r := NewWorkerRegistry(WorkerDependencies{
		Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return ManagedEngineVersion, nil },
		Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) {
			return &leaseWorkerSession{fakeManagedSession(t, "managed")}, nil
		},
	})
	t.Cleanup(func() { _ = r.Shutdown(context.Background()) })
	m := NewManagedLeaseManager(r)
	old, _ := acquireLease(t, m)
	if err := r.Stop(old.WorkerID); err != nil {
		t.Fatal(err)
	}
	current, _ := acquireLease(t, m)
	if old.WorkerID == current.WorkerID || old.PageID != current.PageID {
		t.Fatal("test requires distinct generations with reused numeric ID")
	}
	_, err := m.Call(context.Background(), leaseScope(), old.BrowserLeaseID, "click", nil)
	assertBrowserCode(t, err, ErrWorkerNotReady)
	if _, err := m.Release(context.Background(), leaseScope(), old.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	result, err := m.Call(context.Background(), leaseScope(), current.BrowserLeaseID, "click", nil)
	if err != nil || result["args"].(map[string]any)["pageId"] != float64(1) {
		t.Fatalf("new generation affected: %v %v", result, err)
	}
}
