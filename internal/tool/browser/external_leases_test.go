package browser

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

func externalRoute(start ResolvedStart) RouteDecision {
	return grantExternalRouteForTest(RouteDecision{Scope: leaseScope(), Route: browserpolicy.RouteExternal, Start: start})
}

func externalStart() ResolvedStart {
	return ResolvedStart{Browser: BrowserEdge, Engine: EngineChromeDevToolsMCP, EngineVersion: PreferredEngineVersion,
		ProfileID: "user-edge", ConnectorID: "edge-ws", Endpoint: "ws://127.0.0.1:9222/devtools/browser/registered",
		ProfileClass: ProfileAuthenticatedExternal, BackgroundPage: true, ForegroundPolicy: ForegroundForbidden, LifecyclePolicy: LifecycleExternal,
		Ownership: ResourceOwnership{Process: OwnerExternalPersistent, Profile: OwnerExternalPersistent, Connector: OwnerAgentDockIsolated}}
}

func externalPageEntry(id float64, selected bool) map[string]any {
	return map[string]any{"id": id, "url": "about:blank", "title": "", "selected": selected}
}
func externalPageResult(entries ...any) map[string]any {
	return map[string]any{"structuredContent": map[string]any{"pages": entries}}
}

type externalFakeBackend struct {
	mu                                               sync.Mutex
	pages                                            map[string]map[string]any
	actionResult                                     map[string]any
	actionErr                                        error
	workers                                          map[string]WorkerInfo
	calls                                            []leaseBackendCall
	starts                                           int
	startErr, baselineErr, newErr, closeErr, stopErr error
	baseline, created                                map[string]any
	afterCreate                                      func()
	entered, gate                                    chan struct{}
}

func (b *externalFakeBackend) StartExternal(_ context.Context, options ExternalWorkerOptions) (WorkerInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.starts++
	w := WorkerInfo{WorkerID: fmt.Sprint(b.starts), CreatedAt: time.Now().UTC(), State: WorkerReady, Session: mcpclient.SessionInfo{PID: b.starts, ServerName: ManagedEngineServerName, ServerVersion: PreferredEngineVersion}}
	if b.workers == nil {
		b.workers = make(map[string]WorkerInfo)
	}
	b.workers[w.WorkerID] = w
	b.calls = append(b.calls, leaseBackendCall{worker: w.WorkerID, tool: "start"})
	return w, b.startErr
}
func (b *externalFakeBackend) CallExternal(ctx context.Context, expected WorkerInfo, tool string, args map[string]any) (map[string]any, error) {
	b.mu.Lock()
	w := b.workers[expected.WorkerID]
	if w.WorkerID != expected.WorkerID || w.CreatedAt != expected.CreatedAt || w.Session != expected.Session || w.State != WorkerReady {
		b.mu.Unlock()
		return nil, leaseError(ErrLeaseTargetMismatch, "", "stale worker", nil)
	}
	b.calls = append(b.calls, leaseBackendCall{expected.WorkerID, tool, maps.Clone(args)})
	switch tool {
	case "list_pages":
		defer b.mu.Unlock()
		if b.baseline != nil {
			return b.baseline, b.baselineErr
		}
		if pages := b.pages[expected.WorkerID]; pages != nil {
			return pages, b.baselineErr
		}
		return externalPageResult(externalPageEntry(100, false)), b.baselineErr
	case "new_page":
		defer b.mu.Unlock()
		if b.afterCreate != nil {
			b.afterCreate()
		}
		created := b.created
		if created == nil {
			created = externalPageResult(externalPageEntry(100, false), externalPageEntry(float64(w.Session.PID), true))
		}
		if b.pages == nil {
			b.pages = make(map[string]map[string]any)
		}
		b.pages[expected.WorkerID] = created
		return created, b.newErr
	case "close_page":
		defer b.mu.Unlock()
		if b.closeErr != nil {
			return nil, b.closeErr
		}
		pageID, _ := args["pageId"].(float64)
		if current := b.pages[expected.WorkerID]; current != nil {
			structured, _ := current["structuredContent"].(map[string]any)
			entries, _ := structured["pages"].([]any)
			kept := make([]any, 0, len(entries))
			for _, entry := range entries {
				p, _ := entry.(map[string]any)
				id, _ := p["id"].(float64)
				if id != pageID {
					kept = append(kept, entry)
				}
			}
			b.pages[expected.WorkerID] = externalPageResult(kept...)
		}
		return map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": "The selected page has been closed. Call list_pages to see open pages."}}}, nil
	default:
		entered, gate, actionErr, actionResult := b.entered, b.gate, b.actionErr, b.actionResult
		b.mu.Unlock()
		if entered != nil {
			entered <- struct{}{}
			<-gate
		}
		b.mu.Lock()
		b.calls = append(b.calls, leaseBackendCall{worker: expected.WorkerID, tool: "operation_complete"})
		b.mu.Unlock()
		if actionResult != nil {
			return actionResult, errors.Join(actionErr, ctx.Err())
		}
		return map[string]any{"worker": expected.WorkerID, "args": args}, errors.Join(actionErr, ctx.Err())
	}
}

func (b *externalFakeBackend) Stop(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, leaseBackendCall{worker: id, tool: "stop"})
	w := b.workers[id]
	w.State = WorkerStopped
	b.workers[id] = w
	return b.stopErr
}
func acquireExternal(t *testing.T, m *ExternalLeaseManager) (LeaseMetadata, EngineBinding) {
	t.Helper()
	meta, binding, err := m.Acquire(context.Background(), externalRoute(externalStart()), "")
	if err != nil {
		t.Fatal(err)
	}
	return meta, binding
}

func TestExternalStartValidation(t *testing.T) {
	mutations := map[string]func(*ResolvedStart){
		"engine": func(s *ResolvedStart) { s.Engine = EngineNativeCDP }, "version": func(s *ResolvedStart) { s.EngineVersion = "latest" },
		"browser":       func(s *ResolvedStart) { s.Browser = "safari" },
		"profile class": func(s *ResolvedStart) { s.ProfileClass = ProfileIsolated }, "profile": func(s *ResolvedStart) { s.ProfileID = " " }, "connector": func(s *ResolvedStart) { s.ConnectorID = "" },
		"headless": func(s *ResolvedStart) { s.Headless = true }, "background": func(s *ResolvedStart) { s.BackgroundPage = false }, "foreground": func(s *ResolvedStart) { s.ForegroundPolicy = "allowed" }, "lifecycle": func(s *ResolvedStart) { s.LifecyclePolicy = LifecycleOwned },
		"process": func(s *ResolvedStart) { s.Ownership.Process = OwnerAgentDockIsolated }, "profile owner": func(s *ResolvedStart) { s.Ownership.Profile = OwnerAdapter }, "connector owner": func(s *ResolvedStart) { s.Ownership.Connector = OwnerExternalPersistent }, "adapter": func(s *ResolvedStart) { s.Ownership.Connector = OwnerAdapter },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			s := externalStart()
			mutate(&s)
			b := &externalFakeBackend{}
			m := newTestExternalLeaseManager(b)
			_, _, err := m.Acquire(context.Background(), externalRoute(s), "")
			assertBrowserCode(t, err, ErrPolicyConflict)
			if b.starts != 0 {
				t.Fatal("invalid start reached backend")
			}
			r := NewWorkerRegistry(WorkerDependencies{Probe: func(context.Context, mcpclient.ServerConfig) (string, error) {
				t.Fatal("invalid start probed")
				return "", nil
			}})
			_, err = r.StartExternal(context.Background(), ExternalWorkerOptions{Route: externalRoute(s)})
			assertBrowserCode(t, err, ErrPolicyConflict)
		})
	}
	for _, endpoint := range []string{"", "http://127.0.0.1:9222", "ws://example.com:9222/devtools/browser", "ws://127.0.0.1:9222/devtools/page/1", "ws://127.0.0.1:9222/devtools/browser?", "ws://127.0.0.1:9222/devtools/browser#", "ws://user@127.0.0.1:9222/devtools/browser", "ws://LOCALHOST:09222/devtools/browser", "ws://127.0.0.1:0/devtools/browser", "ws://127.0.0.1:9222/devtools/browser/%61"} {
		s := externalStart()
		s.Endpoint = endpoint
		assertBrowserCode(t, validateExternalStart(externalRoute(s)), ErrPolicyConflict)
	}
	for _, kind := range []Kind{BrowserChrome, BrowserChromium, BrowserEdge} {
		s := externalStart()
		s.ProfileClass = ProfileExternal
		s.Browser = kind
		if err := validateExternalStart(externalRoute(s)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExternalPageProof(t *testing.T) {
	baseline, err := externalPages(externalPageResult(externalPageEntry(100, true)))
	if err != nil {
		t.Fatal(err)
	}
	valid := func() map[string]any {
		return externalPageResult(externalPageEntry(100, false), externalPageEntry(7, true))
	}
	mutations := map[string]func(map[string]any){
		"missing structured":  func(r map[string]any) { delete(r, "structuredContent") },
		"malformed pages":     func(r map[string]any) { r["structuredContent"].(map[string]any)["pages"] = "text" },
		"reconnected":         func(r map[string]any) { r["structuredContent"].(map[string]any)["reconnected"] = true },
		"malformed reconnect": func(r map[string]any) { r["structuredContent"].(map[string]any)["reconnected"] = "false" },
		"error result":        func(r map[string]any) { r["isError"] = true },
		"duplicate": func(r map[string]any) {
			r["structuredContent"].(map[string]any)["pages"] = []any{externalPageEntry(7, true), externalPageEntry(7, true)}
		},
		"two delta": func(r map[string]any) {
			r["structuredContent"].(map[string]any)["pages"] = []any{externalPageEntry(7, true), externalPageEntry(8, false)}
		},
		"zero delta": func(r map[string]any) {
			r["structuredContent"].(map[string]any)["pages"] = []any{externalPageEntry(100, true)}
		},
		"non entry": func(r map[string]any) { r["structuredContent"].(map[string]any)["pages"] = []any{"text"} },
	}
	for _, field := range []string{"id", "url", "title", "selected"} {
		f := field
		mutations["missing "+f] = func(r map[string]any) {
			delete(r["structuredContent"].(map[string]any)["pages"].([]any)[1].(map[string]any), f)
		}
	}
	for name, value := range map[string]any{"string id": "7", "fraction": 1.5, "negative": -1.0, "unsafe": float64(9007199254740992), "nan": math.NaN(), "infinite": math.Inf(1), "isolated": "incognito", "empty isolation": "", "malformed isolation": true, "bad selected": "true", "bad url": 3, "bad title": false} {
		name, value := name, value
		mutations[name] = func(r map[string]any) {
			p := r["structuredContent"].(map[string]any)["pages"].([]any)[1].(map[string]any)
			field := "id"
			switch name {
			case "unselected", "bad selected":
				field = "selected"
			case "isolated", "empty isolation", "malformed isolation":
				field = "isolatedContext"
			case "bad url":
				field = "url"
			case "bad title":
				field = "title"
			}
			p[field] = value
		}
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			r := valid()
			mutate(r)
			if _, _, err := externalNewPageID(baseline, r); err == nil {
				t.Fatal("ambiguous page accepted")
			}
			if name != "two delta" && name != "zero delta" && name != "isolated" {
				if _, err := externalPages(r); err == nil {
					t.Fatal("malformed baseline accepted")
				}
			}
		})
	}
	id, opaque, err := externalNewPageID(baseline, valid())
	if err != nil || id != 7 || opaque != "7" {
		t.Fatalf("%v %s %v", id, opaque, err)
	}
	if _, err := externalPages(externalPageResult()); err != nil {
		t.Fatal("empty browser baseline rejected", err)
	}
}

func TestExternalLeaseOperationsAndRelease(t *testing.T) {
	b := &externalFakeBackend{}
	m := newTestExternalLeaseManager(b)
	a, ab := acquireExternal(t, m)
	c, cb := acquireExternal(t, m)
	if a.WorkerID == c.WorkerID || a.PageID == c.PageID || a.BrowserLeaseID == c.BrowserLeaseID || a.BrowserSessionID == c.BrowserSessionID {
		t.Fatal("leases share targets")
	}
	for _, pair := range []struct {
		meta    LeaseMetadata
		binding EngineBinding
	}{{a, ab}, {c, cb}} {
		meta, binding := pair.meta, pair.binding
		if meta.OwnerType != OwnerExternalPersistent || meta.CDPEndpoint != externalStart().Endpoint || meta.ProfileClass != ProfileAuthenticatedExternal || meta.Ownership != externalStart().Ownership || meta.IsolationContextName != "" || meta.BrowserContextID != "" || binding.IsolationContextName != "" || binding.BrowserContextID != "" || binding.ConnectorID != externalStart().ConnectorID || meta.BrowserPID.PID != 0 {
			t.Fatalf("bad external metadata: %+v %+v", meta, binding)
		}
		for _, tool := range []string{"navigate_page", "take_snapshot", "take_screenshot", "evaluate_script", "click", "fill", "press_key"} {
			args := map[string]any{"value": "test"}
			result, err := m.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, tool, args)
			if err != nil || result["worker"] != binding.WorkerID || fmt.Sprint(result["args"].(map[string]any)["pageId"]) != meta.PageID {
				t.Fatalf("cross target: %v %v", result, err)
			}
			if _, exists := args["pageId"]; exists {
				t.Fatal("mutated caller input")
			}
		}
		before := len(b.calls)
		for _, tool := range []string{"list_pages", "new_page", "close_page", "select_page", "resize_page", "activate", "bringToFront", "window"} {
			_, err := m.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, tool, nil)
			assertBrowserCode(t, err, ErrActionInvalid)
		}
		_, err := m.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "click", map[string]any{"pageId": nil})
		assertBrowserCode(t, err, ErrLeaseTargetMismatch)
		for _, mutate := range []func(*RequestScope){func(s *RequestScope) { s.WorkspaceID = "other" }, func(s *RequestScope) { s.CanonicalWorkspaceRoot = "/other" }, func(s *RequestScope) { s.OwnerTaskID = "other" }, func(s *RequestScope) { s.OwnerSessionID = "other" }, func(s *RequestScope) { s.OwnerACPSessionID = "other" }, func(s *RequestScope) { s.Provenance = ScopeRuntime }} {
			s := leaseScope()
			mutate(&s)
			_, err := m.Call(context.Background(), s, meta.BrowserLeaseID, "click", nil)
			assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
			_, err = m.Release(context.Background(), s, meta.BrowserLeaseID)
			assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
		}
		if len(b.calls) != before {
			t.Fatal("rejected operation reached worker")
		}
		cleanup, err := m.Release(context.Background(), leaseScope(), meta.BrowserLeaseID)
		if err != nil || cleanup.CleanupState != CleanupComplete {
			t.Fatalf("%+v %v", cleanup, err)
		}
		last := b.calls[len(b.calls)-2:]
		if last[0].tool != "close_page" || fmt.Sprint(last[0].args["pageId"]) != meta.PageID || last[1].tool != "stop" || last[1].worker != meta.WorkerID {
			t.Fatalf("unsafe release: %+v", last)
		}
		before = len(b.calls)
		_, err = m.Release(context.Background(), leaseScope(), meta.BrowserLeaseID)
		if err != nil || len(b.calls) != before {
			t.Fatal("release not idempotent")
		}
		_, err = m.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "click", nil)
		assertBrowserCode(t, err, ErrLeaseStateInvalid)
	}
	for _, call := range b.calls {
		if call.tool == "new_page" && !reflect.DeepEqual(call.args, map[string]any{"url": "about:blank", "background": true}) {
			t.Fatalf("unsafe creation: %+v", call)
		}
		if call.tool == "close_page" && call.args["pageId"] == float64(100) {
			t.Fatal("closed user tab")
		}
	}
}

func TestExternalAcquireFailureCleanup(t *testing.T) {
	for _, name := range []string{"start", "baseline call", "baseline malformed", "baseline reconnect", "ambiguous", "new error", "proven canceled", "proven error", "cleanup close failure", "cleanup stop failure"} {
		t.Run(name, func(t *testing.T) {
			b := &externalFakeBackend{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			proven := false
			switch name {
			case "start":
				b.startErr = errors.New("start")
			case "baseline call":
				b.baselineErr = errors.New("baseline")
			case "baseline malformed":
				b.baseline = map[string]any{}
			case "baseline reconnect":
				b.baseline = externalPageResult(externalPageEntry(100, false))
				b.baseline["structuredContent"].(map[string]any)["reconnected"] = true
			case "ambiguous":
				b.created = externalPageResult(externalPageEntry(1, true), externalPageEntry(2, false))
			case "new error":
				b.created = map[string]any{}
				b.newErr = errors.New("lost response")
			case "proven canceled":
				b.afterCreate = cancel
				// Cancellation invalidates post-create peer proof; do not
				// close a possibly replaced browser's page.
			case "proven error":
				b.newErr = errors.New("error with proof")
				proven = true
			case "cleanup close failure":
				b.newErr = errors.New("post-create tool failure")
				b.closeErr = errors.New("close failed")
				proven = true
			case "cleanup stop failure":
				b.newErr = errors.New("post-create tool failure")
				b.stopErr = errors.New("stop failed")
				proven = true
			}
			m := newTestExternalLeaseManager(b)
			_, _, err := m.Acquire(ctx, externalRoute(externalStart()), "")
			if err == nil {
				t.Fatal("failure published lease")
			}
			if len(m.leases) != 0 || b.calls[len(b.calls)-1].tool != "stop" {
				t.Fatalf("failed acquire leaked: %+v", b.calls)
			}
			closes := 0
			for _, call := range b.calls {
				if call.tool == "close_page" {
					closes++
					if call.args["pageId"] != float64(1) {
						t.Fatal("wrong cleanup target")
					}
				}
			}
			if proven && closes != 1 || !proven && closes != 0 {
				t.Fatalf("unsafe cleanup %d: %+v", closes, b.calls)
			}
			if (name == "ambiguous" || name == "new error") && !strings.Contains(err.Error(), "ambiguous external target cleanup") {
				t.Fatal("ambiguity hidden", err)
			}
		})
	}
}

func TestExternalReleaseFailure(t *testing.T) {
	for _, name := range []string{"close", "stop", "both", "canceled"} {
		t.Run(name, func(t *testing.T) {
			b := &externalFakeBackend{}
			m := newTestExternalLeaseManager(b)
			meta, _ := acquireExternal(t, m)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if name == "close" || name == "both" {
				b.closeErr = errors.New("close failed")
			}
			if name == "stop" || name == "both" {
				b.stopErr = errors.New("stop failed")
			}
			if name == "canceled" {
				cancel()
				b.closeErr = ctx.Err()
			}
			cleanup, err := m.Release(ctx, leaseScope(), meta.BrowserLeaseID)
			wantState := CleanupComplete
			if b.stopErr != nil {
				wantState = CleanupFailed
				assertBrowserCode(t, err, ErrActionFailed)
			} else if err != nil {
				t.Fatal(err)
			}
			if cleanup.CleanupState != wantState || cleanup.CleanupError == "" || b.calls[len(b.calls)-1].tool != "stop" {
				t.Fatalf("cleanup failure hidden: %+v", cleanup)
			}
			before := len(b.calls)
			_, err = m.Release(context.Background(), leaseScope(), meta.BrowserLeaseID)
			if (err != nil) != (b.stopErr != nil) || len(b.calls) != before {
				t.Fatal("failed release retried unsafe cleanup")
			}
		})
	}
}

func TestExternalLeaseStaleWorker(t *testing.T) {
	for _, name := range []string{"stopped", "restarted", "cross worker"} {
		t.Run(name, func(t *testing.T) {
			b := &externalFakeBackend{}
			m := newTestExternalLeaseManager(b)
			meta, _ := acquireExternal(t, m)
			b.mu.Lock()
			w := b.workers[meta.WorkerID]
			switch name {
			case "stopped":
				w.State = WorkerStopped
			case "restarted":
				w.CreatedAt = w.CreatedAt.Add(time.Second)
			case "cross worker":
				w.WorkerID = "other"
			}
			b.workers[meta.WorkerID] = w
			b.mu.Unlock()
			_, err := m.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "click", nil)
			assertBrowserCode(t, err, ErrLeaseTargetMismatch)
			cleanup, err := m.Release(context.Background(), leaseScope(), meta.BrowserLeaseID)
			if err != nil || cleanup.CleanupState != CleanupComplete || cleanup.CleanupError == "" {
				t.Fatal("stale worker cleanup accepted")
			}
			for _, call := range b.calls {
				if call.tool == "click" || call.tool == "close_page" {
					t.Fatal("stale worker touched target")
				}
			}
		})
	}
}

func TestExternalLeaseRaceSerialization(t *testing.T) {
	b := &externalFakeBackend{}
	m := newTestExternalLeaseManager(b)
	meta, _ := acquireExternal(t, m)
	b.entered = make(chan struct{}, 1)
	b.gate = make(chan struct{})
	callDone := make(chan error, 1)
	go func() {
		_, err := m.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "click", nil)
		callDone <- err
	}()
	<-b.entered
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.Release(context.Background(), leaseScope(), meta.BrowserLeaseID)
			errs <- err
		}()
	}
	close(b.gate)
	if err := <-callDone; err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var operations []string
	for _, call := range b.calls {
		operations = append(operations, call.tool)
	}
	if !reflect.DeepEqual(operations, []string{"start", "list_pages", "new_page", "list_pages", "click", "operation_complete", "list_pages", "close_page", "stop"}) {
		t.Fatalf("release raced operation: %v", operations)
	}
}

func TestExternalConcurrentAcquisition(t *testing.T) {
	b := &externalFakeBackend{}
	m := newTestExternalLeaseManager(b)
	var wg sync.WaitGroup
	results := make(chan LeaseMetadata, 16)
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			meta, _, err := m.Acquire(context.Background(), externalRoute(externalStart()), "")
			if err == nil {
				_, err = m.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "take_snapshot", nil)
			}
			if err == nil {
				_, err = m.Release(context.Background(), leaseScope(), meta.BrowserLeaseID)
			}
			results <- meta
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for meta := range results {
		if seen[meta.WorkerID] {
			t.Fatal("shared connector")
		}
		seen[meta.WorkerID] = true
	}
}

func TestExternalResolvedRouteValidation(t *testing.T) {
	for _, route := range []browserpolicy.RouteKind{browserpolicy.RouteExternal, browserpolicy.RouteRequiredExternal} {
		for _, browser := range []Kind{BrowserChrome, BrowserChromium, BrowserEdge} {
			for _, profile := range []ProfileClass{ProfileExternal, ProfileAuthenticatedExternal} {
				d := externalRoute(externalStart())
				d.Route, d.Start.Browser, d.Start.ProfileClass = route, browser, profile
				err := validateExternalStart(d)
				if route == browserpolicy.RouteRequiredExternal && profile == ProfileExternal {
					assertBrowserCode(t, err, ErrPolicyConflict)
				} else if err != nil {
					t.Fatalf("generic trusted route rejected: %+v %v", d, err)
				}
			}
		}
	}
	for _, route := range []browserpolicy.RouteKind{"", browserpolicy.RouteManaged, "invalid"} {
		d := externalRoute(externalStart())
		d.Route = route
		assertBrowserCode(t, validateExternalStart(d), ErrPolicyConflict)
	}
}

func TestExternalActionFailureKeepsReleaseTarget(t *testing.T) {
	b := &externalFakeBackend{}
	m := newTestExternalLeaseManager(b)
	meta, _ := acquireExternal(t, m)
	b.mu.Lock()
	b.actionErr = context.DeadlineExceeded
	b.mu.Unlock()
	_, err := m.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "click", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	cleanup, err := m.Release(context.Background(), leaseScope(), meta.BrowserLeaseID)
	if err != nil || cleanup.CleanupState != CleanupComplete || cleanup.CleanupError != "" {
		t.Fatalf("action error poisoned release: %+v %v", cleanup, err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.calls[len(b.calls)-2].tool != "close_page" {
		t.Fatal("release did not attempt owned page close")
	}
}

func TestExternalPageIdentityLoss(t *testing.T) {
	for _, name := range []string{"missing", "isolated", "duplicate", "reconnected", "malformed"} {
		t.Run(name, func(t *testing.T) {
			b := &externalFakeBackend{}
			m := newTestExternalLeaseManager(b)
			meta, _ := acquireExternal(t, m)
			pages := externalPageResult(externalPageEntry(1, false), externalPageEntry(100, true))
			s := pages["structuredContent"].(map[string]any)
			switch name {
			case "missing":
				s["pages"] = []any{externalPageEntry(100, true)}
			case "isolated":
				s["pages"].([]any)[0].(map[string]any)["isolatedContext"] = "other"
			case "duplicate":
				s["pages"] = []any{externalPageEntry(1, false), externalPageEntry(1, true)}
			case "reconnected":
				s["reconnected"] = true
			case "malformed":
				s["pages"] = "text"
			}
			b.mu.Lock()
			b.pages[meta.WorkerID] = pages
			b.mu.Unlock()
			_, err := m.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "click", nil)
			assertBrowserCode(t, err, ErrLeaseTargetMismatch)
			cleanup, err := m.Release(context.Background(), leaseScope(), meta.BrowserLeaseID)
			if err != nil || cleanup.CleanupState != CleanupComplete || cleanup.CleanupError == "" {
				t.Fatalf("%+v %v", cleanup, err)
			}
			for _, c := range b.calls {
				if c.tool == "click" || c.tool == "close_page" {
					t.Fatal("lost target touched", c)
				}
			}
		})
	}
}

func TestExternalResultSanitization(t *testing.T) {
	r := externalPageResult(externalPageEntry(100, true))
	r["content"] = []any{map[string]any{"type": "text", "text": "user secret tab catalog"}, map[string]any{"type": "image", "data": "image"}}
	s := r["structuredContent"].(map[string]any)
	s["navigatedToUrl"], s["snapshot"], s["extensionPages"] = "about:blank", "leased snapshot", []any{"user extension"}
	clean := sanitizeExternalResult(r)
	fields := clean["structuredContent"].(map[string]any)
	if fields["pages"] != nil || fields["extensionPages"] != nil || fields["snapshot"] != "leased snapshot" || fields["navigatedToUrl"] != "about:blank" || len(clean["content"].([]any)) != 1 {
		t.Fatalf("catalog leaked or operation fields lost: %v", clean)
	}
	if s["pages"] == nil || len(r["content"].([]any)) != 2 {
		t.Fatal("sanitizer mutated backend result")
	}
}

func TestExternalProofIgnoresSelection(t *testing.T) {
	baseline, err := externalPages(externalPageResult(externalPageEntry(100, true)))
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := externalNewPageID(baseline, externalPageResult(externalPageEntry(100, true), externalPageEntry(0, false)))
	if err != nil || id != 0 {
		t.Fatalf("selection used as ownership: %v %v", id, err)
	}
}

func TestExternalOperationSanitizesAndRecoversFromToolError(t *testing.T) {
	b := &externalFakeBackend{}
	m := newTestExternalLeaseManager(b)
	meta, _ := acquireExternal(t, m)
	b.mu.Lock()
	b.actionResult = externalPageResult(externalPageEntry(100, true))
	b.actionResult["content"] = []any{map[string]any{"type": "text", "text": "user tabs"}}
	b.actionResult["isError"] = true
	b.actionResult["structuredContent"].(map[string]any)["message"] = "ordinary action error"
	b.mu.Unlock()
	r, err := m.Call(context.Background(), leaseScope(), meta.BrowserLeaseID, "click", nil)
	assertBrowserCode(t, err, ErrActionFailed)
	if r["content"] != nil || r["structuredContent"].(map[string]any)["pages"] != nil || r["structuredContent"].(map[string]any)["message"] != "ordinary action error" {
		t.Fatal("raw operation catalog exposed", r)
	}
	cleanup, err := m.Release(context.Background(), leaseScope(), meta.BrowserLeaseID)
	if err != nil || cleanup.CleanupError != "" {
		t.Fatalf("%+v %v", cleanup, err)
	}
}

func TestExternalAcquireTrustedResolvedChrome(t *testing.T) {
	r := requiredRouteFixture(t)
	r.Policy.Class = browserpolicy.WorkspaceDefault
	r.Connectors[0].Browser = BrowserChrome
	d, err := ResolveRoute(r)
	if err != nil {
		t.Fatal(err)
	}
	b := &externalFakeBackend{}
	m := newTestExternalLeaseManager(b)
	meta, _, err := m.Acquire(context.Background(), grantExternalRouteForTest(d), "")
	if err != nil {
		t.Fatal("synthetic required authenticated Chrome rejected", err)
	}
	if _, err := m.Release(context.Background(), d.Scope, meta.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
}

func TestExternalCloseResultCompatibility(t *testing.T) {
	closed := map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": externalSelectedPageClosedText}}}
	if err := externalCloseResultError(closed); err != nil {
		t.Fatal("1.7.0 closed-page sentinel rejected", err)
	}
	last := map[string]any{"content": []any{map[string]any{"type": "text", "text": externalLastPageCloseText}}}
	if err := externalCloseResultError(last); err == nil {
		t.Fatal("last-page refusal accepted as close")
	}
	other := map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": "other failure"}}}
	if err := externalCloseResultError(other); err == nil {
		t.Fatal("unrelated MCP failure accepted as close")
	}
}

func externalCallCount(b *externalFakeBackend, tool string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	count := 0
	for _, call := range b.calls {
		if call.tool == tool {
			count++
		}
	}
	return count
}

func TestExternalIdleTTLSweepStopsOnlyOwnedConnectorAndPage(t *testing.T) {
	b := &externalFakeBackend{}
	policy := defaultExternalLeasePolicy()
	policy.IdleTTL = time.Minute
	m := newExternalLeaseManager(b, policy)
	m.verifier = testAcceptExternalPeer
	meta, _ := acquireExternal(t, m)
	if !meta.ExpiresAt.Equal(meta.LastActiveAt.Add(policy.IdleTTL)) {
		t.Fatalf("expires_at=%s last_active=%s", meta.ExpiresAt, meta.LastActiveAt)
	}
	l, err := m.lease(meta.BrowserLeaseID)
	if err != nil {
		t.Fatal(err)
	}
	l.mu.Lock()
	l.metadata.LastActiveAt = time.Now().UTC().Add(-2 * policy.IdleTTL)
	l.metadata.ExpiresAt = l.metadata.LastActiveAt.Add(policy.IdleTTL)
	l.mu.Unlock()
	if err := m.SweepExpired(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	l.mu.Lock()
	cleaned := l.metadata
	l.mu.Unlock()
	if cleaned.CleanupState != CleanupComplete {
		t.Fatalf("cleanup=%+v", cleaned)
	}
	if externalCallCount(b, "close_page") != 1 || externalCallCount(b, "stop") != 1 {
		t.Fatalf("calls=%v", b.calls)
	}
	if cleaned.Ownership.Process != OwnerExternalPersistent || cleaned.Ownership.Profile != OwnerExternalPersistent {
		t.Fatal("external user browser/profile ownership changed")
	}
}

func TestExternalFailedCleanupRecoveryRetriesConnectorOnly(t *testing.T) {
	b := &externalFakeBackend{}
	m := newTestExternalLeaseManager(b)
	meta, _ := acquireExternal(t, m)
	b.mu.Lock()
	b.stopErr = errors.New("stop failed")
	b.mu.Unlock()
	if _, err := m.Release(context.Background(), leaseScope(), meta.BrowserLeaseID); err == nil {
		t.Fatal("cleanup failure hidden")
	}
	closeCalls := externalCallCount(b, "close_page")
	if closeCalls != 1 {
		t.Fatalf("close page calls=%d", closeCalls)
	}
	b.mu.Lock()
	b.stopErr = nil
	b.mu.Unlock()
	if err := m.RecoverFailed(time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if externalCallCount(b, "close_page") != closeCalls {
		t.Fatal("recovery touched external page again")
	}
	if externalCallCount(b, "stop") != 2 {
		t.Fatalf("connector stop calls=%d", externalCallCount(b, "stop"))
	}
	l, _ := m.lease(meta.BrowserLeaseID)
	l.mu.Lock()
	cleaned := l.metadata
	l.mu.Unlock()
	if cleaned.CleanupState != CleanupComplete || !strings.Contains(cleaned.CleanupReason, "external browser/profile preserved") {
		t.Fatalf("cleanup=%+v", cleaned)
	}
}

func TestExternalAcquireOrphanConnectorRecovery(t *testing.T) {
	b := &externalFakeBackend{baselineErr: errors.New("baseline failed"), stopErr: errors.New("stop failed")}
	m := newTestExternalLeaseManager(b)
	if _, _, err := m.Acquire(context.Background(), externalRoute(externalStart()), ""); err == nil {
		t.Fatal("acquire unexpectedly succeeded")
	}
	m.mu.Lock()
	orphans := len(m.orphans)
	m.mu.Unlock()
	if orphans != 1 {
		t.Fatalf("orphans=%d", orphans)
	}
	b.mu.Lock()
	b.stopErr = nil
	b.mu.Unlock()
	if err := m.RecoverFailed(time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	orphans = len(m.orphans)
	m.mu.Unlock()
	if orphans != 0 {
		t.Fatalf("recovered orphans=%d", orphans)
	}
	if externalCallCount(b, "close_page") != 0 {
		t.Fatal("orphan recovery touched a user/owned page without proven identity")
	}
}
