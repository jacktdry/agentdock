//go:build darwin || linux

package browser

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

// Observe and constrain this test's real session; never synthesize MCP results
// or bypass the registry's private keeper selection boundary.
type realACPBridgeSession struct {
	WorkerSession
	mu         sync.Mutex
	keeper     float64
	hasKeeper  bool
	selected   float64
	selections int
	closes     int
	stopCalls  int
	onStop     func()
	stopOnce   sync.Once
}

func (s *realACPBridgeSession) Call(ctx context.Context, tool string, args map[string]any) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch tool {
	case "new_page":
		if args["url"] != "about:blank" || args["background"] != true {
			return nil, fmt.Errorf("test forbids navigation or foreground creation")
		}
	case "select_page":
		if !s.hasKeeper || args["pageId"] != s.keeper || args["bringToFront"] != false || len(args) != 2 {
			return nil, fmt.Errorf("test forbids non-keeper selection or focus")
		}
	case "close_page":
		if !s.hasKeeper || args["pageId"] == s.keeper || s.selected != s.keeper {
			return nil, fmt.Errorf("test forbids keeper close or unguarded cleanup")
		}
	case "evaluate_script", "list_pages":
	default:
		return nil, fmt.Errorf("tool outside isolated real proof")
	}
	result, err := s.WorkerSession.Call(ctx, tool, args)
	if err != nil {
		return result, err
	}
	if err := leaseCallResultError(result); err != nil {
		return result, err
	}
	switch tool {
	case "new_page":
		name, _ := args["isolatedContext"].(string)
		if strings.HasPrefix(name, "agentdock-pool-keeper-") {
			if s.hasKeeper {
				return nil, fmt.Errorf("duplicate keeper")
			}
			page, _, err := isolatedPageID(result, name)
			if err != nil {
				return nil, err
			}
			s.keeper, s.hasKeeper = page, true
		}
	case "select_page":
		s.selected = s.keeper
		s.selections++
	case "close_page":
		s.closes++
	}
	return result, nil
}

func (s *realACPBridgeSession) Close() error {
	s.mu.Lock()
	s.stopCalls++
	s.mu.Unlock()
	err := s.WorkerSession.Close()
	if err == nil {
		s.stopOnce.Do(s.onStop)
	}
	return err
}

func TestACPBridgeSharedRealHeadless(t *testing.T) {
	if os.Getenv("AGENTDOCK_TEST_REAL_SHARED_ACP_BRIDGE") != "1" {
		t.Skip("supervisor opt-in only: AGENTDOCK_TEST_REAL_SHARED_ACP_BRIDGE=1")
	}
	if ManagedEnginePackageSpec != "chrome-devtools-mcp@1.7.0" {
		t.Fatal("this proof requires the exact MCP 1.7.0 contract")
	}
	// No parallel test, HOME override, existing browser/profile, autoConnect,
	// external URL, GUI/screen inspection or global process cleanup.
	t.Setenv("NPM_CONFIG_OFFLINE", "true")
	t.Setenv("NPM_CONFIG_UPDATE_NOTIFIER", "false")
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(cwd, "acp-bridge-chrome-tmp")
	if err := os.Mkdir(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	var mu sync.Mutex
	var sessions []*realACPBridgeSession
	live, peak := 0, 0
	observed := make(map[int]string)
	budget, err := newManagedSharedBrowserBudget(1)
	if err != nil {
		t.Fatal(err)
	}
	var bridge *ACPBridge
	registry := NewWorkerRegistry(WorkerDependencies{
		// The real handshake independently checks this exact version; no probe
		// subprocess/network access is needed by the test-only dependency.
		Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return ManagedEngineVersion, nil },
		Open: func(ctx context.Context, cfg mcpclient.ServerConfig) (WorkerSession, error) {
			want := []string{"--yes", "chrome-devtools-mcp@1.7.0", "--isolated", "--headless", "--experimentalPageIdRouting", "--experimentalStructuredContent", "--no-usage-statistics", "--no-performance-crux"}
			if cfg.Command != "npx" || cfg.Cwd != cwd || cfg.Name != ManagedEngineServerName || cfg.Transport != mcpclient.TransportStdio || !slices.Equal(cfg.Args, want) {
				return nil, fmt.Errorf("unsafe or unexpected managed launch config")
			}
			cfg.Args = append([]string{"--offline"}, cfg.Args...)
			s, err := mcpclient.OpenStdioSession(ctx, cfg)
			if s == nil {
				return nil, err
			}
			wrapped := &realACPBridgeSession{WorkerSession: s, onStop: func() {
				mu.Lock()
				live--
				mu.Unlock()
			}}
			mu.Lock()
			sessions = append(sessions, wrapped)
			live++
			if live > peak {
				peak = live
			}
			mu.Unlock()
			return wrapped, err
		},
	})
	ownedProcesses := func(ctx context.Context) ([]realSharedProcess, error) {
		mu.Lock()
		defer mu.Unlock()
		for _, s := range sessions {
			if _, err := realSharedProcesses(ctx, s.Info().PID, tmp, observed); err != nil {
				return nil, err
			}
		}
		return realSharedProcesses(ctx, 0, tmp, observed)
	}
	checkBudget := func(want int) {
		t.Helper()
		budget.mu.Lock()
		defer budget.mu.Unlock()
		if budget.used != want || budget.maxSlots != 1 {
			t.Fatalf("budget used=%d max=%d, want used=%d max=1", budget.used, budget.maxSlots, want)
		}
	}
	// Register before construction/startup. Failed Close is a blocker: never
	// release claims/slots manually or kill processes discovered through ps.
	t.Cleanup(func() {
		cancel()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cleanupCancel()
		if _, err := ownedProcesses(cleanupCtx); err != nil {
			t.Errorf("pre-shutdown observation: %s", realSharedDiagnosticText(err.Error()))
		}
		if bridge != nil {
			if err := bridge.Close(); err != nil {
				t.Errorf("BLOCKER owned bridge Close: %s", realSharedDiagnosticText(err.Error()))
			}
		}
		if err := registry.Shutdown(cleanupCtx); err != nil {
			t.Errorf("BLOCKER owned registry Shutdown: %s", realSharedDiagnosticText(err.Error()))
		}
		budget.mu.Lock()
		used, owners := budget.used, len(budget.owners)
		budget.mu.Unlock()
		if used != 0 || owners != 0 {
			t.Errorf("BLOCKER retained budget slots=%d owner claims=%d", used, owners)
		}
		for {
			rows, err := ownedProcesses(cleanupCtx)
			if err != nil {
				t.Errorf("cleanup observation: %s", realSharedDiagnosticText(err.Error()))
				return
			}
			if len(rows) == 0 {
				t.Log("owned MCP/Chrome processes gone; no test TMPDIR process marker; budget=0")
				return
			}
			select {
			case <-cleanupCtx.Done():
				t.Errorf("BLOCKER owned processes remain: %s", realSharedDiagnosticText(fmt.Sprint(rows)))
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	})
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := NewRoutePlanner(nil, catalog, nil)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err = newACPBridgeBudgetedRotatingSharedForWorkspace(planner, registry, cwd, budget)
	if err != nil {
		t.Fatal(err)
	}
	bridge.lifecycle.Close() // Manual sweep of the real lifecycle, no racing ticks.
	type owner struct {
		session, token string
		meta           LeaseMetadata
		value          string
	}
	owners := []owner{{session: "chatgpt-equivalent"}, {session: "codex-acp"}, {session: "agy-acp"}}
	ids := make(map[string]bool)
	for i := range owners {
		o := &owners[i]
		o.token, err = bridge.RegisterSession(o.session, o.session, cwd)
		if err != nil {
			t.Fatal(err)
		}
		o.meta, err = bridge.Acquire(ctx, o.token, "about:blank")
		if err != nil {
			t.Fatalf("owner %d acquire: %s", i, realSharedDiagnosticText(err.Error()))
		}
		for j, id := range []string{o.token, o.meta.BrowserLeaseID, o.meta.WorkerID, o.meta.PageID, o.meta.IsolationContextName} {
			key := fmt.Sprintf("%d:%s", j, id)
			if id == "" || ids[key] {
				t.Fatal("duplicate/empty owner identity")
			}
			ids[key] = true
		}
		if o.meta.ConnectorPID.PID <= 0 || o.meta.ConnectorPID.PID != owners[0].meta.ConnectorPID.PID {
			t.Fatal("owners did not share a real MCP PID")
		}
	}
	checkBudget(1)
	checkChrome := func() {
		t.Helper()
		rows, err := ownedProcesses(ctx)
		if err != nil {
			t.Fatal(realSharedDiagnosticText(err.Error()))
		}
		main := 0
		for _, row := range rows {
			if strings.Contains(row.command, "--remote-debugging-pipe") && !strings.Contains(row.command, "--type=") {
				main++
				if !strings.Contains(row.command, "--headless") || !strings.Contains(row.command, "--user-data-dir="+tmp+string(os.PathSeparator)) || strings.Contains(row.command, "--autoConnect") {
					t.Fatal("Chrome main not test-owned isolated headless")
				}
			}
		}
		if main != 1 {
			t.Fatalf("Chrome main count=%d, want 1", main)
		}
	}
	checkChrome()
	evaluate := func(o *owner, function, want string) {
		t.Helper()
		result, err := bridge.Call(ctx, o.token, o.meta.BrowserLeaseID, "evaluate_script", map[string]any{"function": function})
		if err != nil {
			t.Fatal(realSharedDiagnosticText(err.Error()))
		}
		if err := leaseCallResultError(result); err != nil {
			t.Fatal(realSharedDiagnosticText(err.Error()))
		}
		structured, ok := result["structuredContent"].(map[string]any)
		if !ok || structured["reconnected"] == true {
			t.Fatal("invalid/reconnected evaluate response")
		}
		message, _ := structured["message"].(string)
		_, block, found := strings.Cut(message, "```json\n")
		value, _, ended := strings.Cut(block, "\n```")
		var got string
		if !found || !ended || json.Unmarshal([]byte(value), &got) != nil || got != want {
			t.Fatalf("isolated evaluate mismatch: %s", realSharedDiagnosticText(message))
		}
	}
	read := "() => window.__agent ?? ''"
	for range 3 {
		for i := range owners {
			o := &owners[i]
			o.value = rand.Text()
			evaluate(o, fmt.Sprintf("() => { window.__agent = %q; return window.__agent; }", o.value), o.value)
			sibling := &owners[(i+1)%len(owners)]
			evaluate(sibling, read, sibling.value)
		}
	}
	a := &owners[0]
	_, err = bridge.Call(ctx, a.token, a.meta.BrowserLeaseID, "evaluate_script", map[string]any{"function": read, "pageId": 999.0})
	assertBrowserCode(t, err, ErrLeaseTargetMismatch)
	_, err = bridge.Call(ctx, a.token, a.meta.BrowserLeaseID, "select_page", nil)
	assertBrowserCode(t, err, ErrActionInvalid)
	_, err = bridge.Call(ctx, owners[1].token, a.meta.BrowserLeaseID, "evaluate_script", map[string]any{"function": read})
	assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
	g := bridge.rotating.generation
	underlying := g.worker.WorkerID
	_, err = registry.Call(ctx, underlying, "select_page", map[string]any{"pageId": *g.keeperPage, "bringToFront": false})
	assertBrowserCode(t, err, ErrEngineSchemaIncompatible)
	if _, err := bridge.rotating.Call(ctx, a.meta.WorkerID, "close_page", map[string]any{"pageId": *g.keeperPage}); err == nil {
		t.Fatal("borrower closed keeper")
	}
	for i := range owners {
		o := &owners[i]
		if err := bridge.ReleaseSession(ctx, o.session); err != nil {
			t.Fatalf("ReleaseSession: %s", realSharedDiagnosticText(err.Error()))
		}
		l, err := bridge.managed.manager.lease(o.meta.BrowserLeaseID)
		if err != nil {
			t.Fatal(err)
		}
		l.mu.Lock()
		cleanup := l.metadata
		l.mu.Unlock()
		if cleanup.CleanupState != CleanupComplete || cleanup.CleanupError != "" || cleanup.CleanupReason != "shared managed owned page closed and borrow ended" {
			t.Fatal("session release did not complete shared page cleanup")
		}
		for j := i + 1; j < len(owners); j++ {
			evaluate(&owners[j], read, owners[j].value)
		}
		checkBudget(1) // Releasing a borrow never releases a browser slot.
	}
	mu.Lock()
	first := sessions[0]
	starts := len(sessions)
	mu.Unlock()
	first.mu.Lock()
	closes, selections, stops := first.closes, first.selections, first.stopCalls
	first.mu.Unlock()
	if starts != 1 || closes != 3 || selections != 3 || stops != 0 {
		t.Fatalf("starts=%d closes=%d keeper selections=%d stops=%d", starts, closes, selections, stops)
	}
	result, err := registry.Call(ctx, underlying, "list_pages", map[string]any{})
	if err != nil {
		t.Fatal(realSharedDiagnosticText(err.Error()))
	}
	structured, _ := result["structuredContent"].(map[string]any)
	pages, ok := structured["pages"].([]any)
	if !ok || structured["reconnected"] == true {
		t.Fatal("invalid retained pages response")
	}
	isolated := 0
	for _, entry := range pages {
		page, ok := entry.(map[string]any)
		if !ok || page["url"] != "about:blank" {
			t.Fatal("unexpected retained page")
		}
		if page["isolatedContext"] != nil {
			isolated++
			if page["id"] != *g.keeperPage {
				t.Fatal("borrowed isolated page survived release")
			}
		}
	}
	// MCP may retain its default bootstrap blank page. Never guess ownership
	// and close it: the pool owns exactly one remaining isolated keeper page.
	if isolated != 1 {
		t.Fatalf("retained isolated pages=%d, want keeper only", isolated)
	}
	checkChrome()
	for range 2 {
		if err := bridge.sweepLifecycle(time.Now().Add(time.Hour)); err != nil {
			t.Fatal(realSharedDiagnosticText(err.Error()))
		}
	}
	checkBudget(0)
	first.mu.Lock()
	stops = first.stopCalls
	first.mu.Unlock()
	info, err := registry.Info(underlying)
	if err != nil || info.State != WorkerStopped || stops != 1 {
		t.Fatal("idle reap did not stop owned worker exactly once")
	}
	// Prove old Chrome/process identities are gone before allowing replacement;
	// sequential live-real generations must never overlap with a budget of one.
	for {
		rows, err := ownedProcesses(ctx)
		if err != nil {
			t.Fatal(realSharedDiagnosticText(err.Error()))
		}
		if len(rows) == 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("owned generation processes survived idle reap")
		case <-time.After(100 * time.Millisecond):
		}
	}
	next := owner{session: a.session}
	next.token, err = bridge.RegisterSession(next.session, next.session, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if next.token == a.token {
		t.Fatal("re-registration reused revoked capability")
	}
	next.meta, err = bridge.Acquire(ctx, next.token, "about:blank")
	if err != nil {
		t.Fatal(realSharedDiagnosticText(err.Error()))
	}
	newUnderlying := bridge.rotating.generation.worker.WorkerID
	if newUnderlying == underlying || next.meta.WorkerID == a.meta.WorkerID {
		t.Fatal("replacement reused old generation identity")
	}
	mu.Lock()
	starts = len(sessions)
	mu.Unlock()
	if starts != 2 {
		t.Fatalf("replacement starts=%d, want 2", starts)
	}
	checkBudget(1)
	checkChrome()
	evaluate(&next, read, "")
	for _, o := range owners {
		if _, err := bridge.Call(ctx, o.token, o.meta.BrowserLeaseID, "evaluate_script", map[string]any{"function": read}); err == nil {
			t.Fatal("released session token accepted")
		}
		if _, err := bridge.Call(ctx, next.token, o.meta.BrowserLeaseID, "evaluate_script", map[string]any{"function": read}); err == nil {
			t.Fatal("old lease accepted by replacement owner")
		}
		if _, err := bridge.rotating.Call(ctx, o.meta.WorkerID, "evaluate_script", map[string]any{"pageId": 999.0, "function": read}); err == nil {
			t.Fatal("old borrow routed into replacement generation")
		}
	}
	evaluate(&next, read, "")
	if err := bridge.Close(); err != nil {
		t.Fatalf("BLOCKER bridge Close: %s", realSharedDiagnosticText(err.Error()))
	}
	checkBudget(0)
	mu.Lock()
	defer mu.Unlock()
	if live != 0 || peak != 1 {
		t.Fatalf("owned MCP live=%d peak=%d, want 0/1", live, peak)
	}
	for _, s := range sessions {
		s.mu.Lock()
		stops := s.stopCalls
		s.mu.Unlock()
		if stops != 1 {
			t.Fatalf("owned session stops=%d, want exactly 1", stops)
		}
	}
	t.Logf("ACPBridge owners=3; MCP PID=%d; generations=2; budget max=1; owned MCP peak=1; Chrome main=1 per generation; headless/no-focus call guards enforced", a.meta.ConnectorPID.PID)
}
