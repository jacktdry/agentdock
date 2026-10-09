//go:build darwin || linux

package browser

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

// Test-only decorator: all calls still reach the real, owned stdio session.
type realSharedBackend struct {
	*WorkerRegistry
	mu                sync.Mutex
	starts, stops     int
	worker            WorkerInfo
	session           WorkerSession // Only this test's owned connection; registry blocks select_page.
	closes            []string
	t                 *testing.T
	closeObservations []string
}

var realSharedDiagnosticRedactions = regexp.MustCompile(`(?i)(?:file://)?/(?:Users|home|private|tmp|var|Applications|opt|usr)/[^\s"'<>]+|(?:https?|wss?)://[^\s"'<>]+|(?:bearer\s+|(?:token|password|secret|cookie|authorization)\s*[:=]\s*)[^\s,;]+`)

func realSharedDiagnosticText(value string) string {
	value = realSharedDiagnosticRedactions.ReplaceAllString(value, "[redacted]")
	value = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, value)
	if len(value) > 2048 {
		value = value[:2048] + "[truncated]"
	}
	return value
}

func (b *realSharedBackend) StartManaged(ctx context.Context, opts ManagedWorkerOptions) (WorkerInfo, error) {
	b.mu.Lock()
	b.starts++
	b.mu.Unlock()
	w, err := b.WorkerRegistry.StartManaged(ctx, opts)
	b.mu.Lock()
	b.worker = w
	b.mu.Unlock()
	return w, err
}

func (b *realSharedBackend) Call(ctx context.Context, id, tool string, args map[string]any) (result map[string]any, err error) {
	if tool == "close_page" {
		defer func() {
			// Allowlisted fields from this test's owned about:blank pages only.
			// Error responses may have content.text instead of structuredContent.
			structured, _ := result["structuredContent"].(map[string]any)
			message, _ := structured["message"].(string)
			failed, hasErrorFlag := result["isError"].(bool)
			var errorText []string
			if failed {
				content, _ := result["content"].([]any)
				for _, entry := range content {
					item, _ := entry.(map[string]any)
					if item["type"] == "text" {
						value, _ := item["text"].(string)
						errorText = append(errorText, value)
					}
				}
			}
			cause := ""
			if err != nil {
				cause = err.Error()
			}
			page, _ := args["pageId"].(float64)
			observation := fmt.Sprintf("close_page pageId=%.0f resultPresent=%t isError=%t boolFlagPresent=%t structuredPresent=%t message=%q errorText=%q error=%q", page, result != nil, failed, hasErrorFlag, structured != nil, realSharedDiagnosticText(message), realSharedDiagnosticText(strings.Join(errorText, " | ")), realSharedDiagnosticText(cause))
			b.mu.Lock()
			b.closeObservations = append(b.closeObservations, observation)
			b.mu.Unlock()
			b.t.Logf("%s", observation)
		}()
	}
	b.mu.Lock()
	w := b.worker
	if tool == "close_page" {
		b.closes = append(b.closes, fmt.Sprint(args["pageId"]))
	}
	b.mu.Unlock()
	info, err := b.Info(id)
	if err != nil || id != w.WorkerID || info.Session.PID <= 0 || info.Session.PID != w.Session.PID {
		return nil, fmt.Errorf("real shared worker identity changed: %v", err)
	}
	if tool == "select_page" {
		b.mu.Lock()
		session := b.session
		b.mu.Unlock()
		if session == nil || session.Info().PID != w.Session.PID || args["bringToFront"] != false {
			return nil, errManagedSharedUnavailable
		}
		// Test-only internal selection; StdioSession validates the real schema.
		return session.Call(ctx, tool, args)
	}
	return b.WorkerRegistry.Call(ctx, id, tool, args)
}

func (b *realSharedBackend) Stop(id string) error {
	b.mu.Lock()
	b.stops++
	b.mu.Unlock()
	return b.WorkerRegistry.Stop(id)
}

type realSharedProcess struct {
	pid, parent, group int
	command            string
}

// Observations never grant kill authority. Only registry/backend Stop terminates
// processes. Marker matching also catches Chrome's separately detached group.
func realSharedProcesses(ctx context.Context, root int, marker string, observed map[int]string) ([]realSharedProcess, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,pgid=,command=").Output()
	if err != nil {
		return nil, err
	}
	var rows []realSharedProcess
	owned := map[int]bool{}
	if root > 0 {
		owned[root] = true
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid, e1 := strconv.Atoi(fields[0])
		parent, e2 := strconv.Atoi(fields[1])
		group, e3 := strconv.Atoi(fields[2])
		if e1 != nil || e2 != nil || e3 != nil {
			return nil, fmt.Errorf("invalid ps identity")
		}
		command := strings.Join(fields[3:], " ")
		rows = append(rows, realSharedProcess{pid, parent, group, command})
		if (root > 0 && group == root) || strings.Contains(command, marker) || observed[pid] == command {
			owned[pid] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, row := range rows {
			if owned[row.parent] && !owned[row.pid] {
				owned[row.pid], changed = true, true
			}
		}
	}
	var result []realSharedProcess
	for _, row := range rows {
		if owned[row.pid] {
			observed[row.pid] = row.command
			result = append(result, row)
		}
	}
	return result, nil
}

func TestManagedSharedRealHeadless(t *testing.T) {
	if os.Getenv("AGENTDOCK_TEST_REAL_MANAGED_CHROME") != "1" {
		t.Skip("supervisor opt-in only: AGENTDOCK_TEST_REAL_MANAGED_CHROME=1")
	}
	if ManagedEnginePackageSpec != "chrome-devtools-mcp@1.7.0" {
		t.Fatal("this proof requires the exact MCP 1.7.0 contract")
	}
	// No HOME/profile override, attach endpoint, autoConnect, network navigation,
	// GUI action, or global process cleanup. Never parallelize this env-setting test.
	t.Setenv("NPM_CONFIG_OFFLINE", "true")
	t.Setenv("NPM_CONFIG_UPDATE_NOTIFIER", "false")
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(cwd, "chrome-tmp")
	if err := os.Mkdir(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	var pidMu sync.Mutex
	pid := 0
	observed := make(map[int]string)
	b := &realSharedBackend{t: t}
	registry := NewWorkerRegistry(WorkerDependencies{Open: func(ctx context.Context, cfg mcpclient.ServerConfig) (WorkerSession, error) {
		// Exact managed config plus explicit offline npm; no production wiring.
		cfg.Args = append([]string{"--offline"}, cfg.Args...)
		session, err := mcpclient.OpenStdioSession(ctx, cfg)
		if session != nil {
			b.mu.Lock()
			b.session = session
			b.mu.Unlock()
			pidMu.Lock()
			pid = session.Info().PID
			pidMu.Unlock()
		}
		return session, err
	}})
	b.WorkerRegistry = registry
	shared, err := newManagedSharedWorkerBackend(b, ManagedWorkerOptions{Cwd: cwd})
	// Registered before any startup; Fatal on any later phase still cleans up.
	t.Cleanup(func() {
		cancel()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cleanupCancel()
		pidMu.Lock()
		root := pid
		pidMu.Unlock()
		if _, err := realSharedProcesses(cleanupCtx, root, tmp, observed); err != nil {
			t.Errorf("pre-shutdown process observation unavailable: %v", err)
		}
		if shared != nil {
			if err := shared.Shutdown(cleanupCtx); err != nil {
				t.Errorf("owned shared shutdown: %v", err)
			}
		}
		if err := registry.Shutdown(cleanupCtx); err != nil {
			t.Errorf("owned registry shutdown: %v", err)
		}
		pidMu.Lock()
		root = pid
		pidMu.Unlock()
		for {
			rows, err := realSharedProcesses(cleanupCtx, root, tmp, observed)
			if err != nil {
				t.Errorf("cleanup observation unavailable: %v", err)
				return
			}
			if len(rows) == 0 {
				t.Log("owned MCP/Chrome processes gone")
				return
			}
			select {
			case <-cleanupCtx.Done():
				t.Errorf("owned process cleanup pending: %+v", rows)
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	m := NewManagedLeaseManager(shared)
	leases := make([]LeaseMetadata, 3)
	pages, contexts, borrows := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := range leases {
		scope := sharedScope(i)
		scope.CanonicalWorkspaceRoot = cwd
		meta, _, err := m.Acquire(ctx, scope, leaseStart(t), "about:blank")
		if err != nil {
			t.Fatalf("owner %d acquire (MCP pages/context contract blocker): %v", i, err)
		}
		leases[i] = meta
		if meta.ConnectorPID.PID <= 0 || meta.ConnectorPID.PID != leases[0].ConnectorPID.PID || pages[meta.PageID] || contexts[meta.IsolationContextName] || borrows[meta.WorkerID] {
			t.Fatal("shared PID or isolated lease identity violated")
		}
		pages[meta.PageID], contexts[meta.IsolationContextName], borrows[meta.WorkerID] = true, true, true
	}
	rows, err := realSharedProcesses(ctx, leases[0].ConnectorPID.PID, tmp, observed)
	if err != nil {
		t.Fatal(err)
	}
	chrome := 0
	for _, row := range rows {
		if strings.Contains(row.command, "--remote-debugging-pipe") && !strings.Contains(row.command, "--type=") {
			chrome++
			if !strings.Contains(row.command, "--headless") || !strings.Contains(row.command, "--user-data-dir="+tmp) {
				t.Fatal("Chrome is not test-owned isolated headless")
			}
		}
	}
	if chrome != 1 {
		t.Fatalf("expected one isolated Chrome main process, got %d", chrome)
	}
	t.Logf("owned MCP PID=%d; Chrome main count=%d; owners=3", leases[0].ConnectorPID.PID, chrome)
	evaluate := func(i int, function, want string) {
		t.Helper()
		meta := leases[i]
		result, err := m.Call(ctx, meta.Scope, meta.BrowserLeaseID, "evaluate_script", map[string]any{"function": function})
		if err != nil {
			t.Fatalf("owner %d evaluate: %v", i, err)
		}
		if err := leaseCallResultError(result); err != nil {
			t.Fatal(err)
		}
		structured, ok := result["structuredContent"].(map[string]any)
		if !ok || structured["reconnected"] == true {
			t.Fatal("invalid/reconnected structuredContent")
		}
		message, _ := structured["message"].(string)
		_, block, found := strings.Cut(message, "```json\n")
		value, _, ended := strings.Cut(block, "\n```")
		var got string
		if !found || !ended || json.Unmarshal([]byte(value), &got) != nil || got != want {
			t.Fatalf("owner %d structured evaluate result mismatch (protocol/target blocker): want %q, message %q", i, want, message)
		}
	}
	values := []string{"", "", ""}
	read := "() => window.__agent ?? ''"
	values[0] = rand.Text()
	evaluate(0, fmt.Sprintf("() => { window.__agent = %q; return window.__agent; }", values[0]), values[0])
	evaluate(1, read, "")
	evaluate(2, read, "")
	// 18 interleaved writes/reads: all owners act, each read checks its own page.
	for round := range 3 {
		for i := range leases {
			values[i] = fmt.Sprintf("%s-%d-%d", rand.Text(), i, round)
			evaluate(i, fmt.Sprintf("() => { window.__agent = %q; return window.__agent; }", values[i]), values[i])
			evaluate((i+1)%3, read, values[(i+1)%3])
		}
	}
	for i, meta := range leases {
		cleanup, err := m.Release(ctx, meta.Scope, meta.BrowserLeaseID)
		if err != nil || cleanup.CleanupState != CleanupComplete || cleanup.CleanupError != "" {
			b.mu.Lock()
			observations := strings.Join(b.closeObservations, "\n")
			b.mu.Unlock()
			t.Fatalf("owner %d Release close_page cleanup: state=%s reason=%q cleanupError=%q error=%q; recorded close_page observations:\n%s", i, cleanup.CleanupState, realSharedDiagnosticText(cleanup.CleanupReason), realSharedDiagnosticText(cleanup.CleanupError), realSharedDiagnosticText(fmt.Sprint(err)), observations)
		}
		b.mu.Lock()
		starts, stops, closed := b.starts, b.stops, append([]string(nil), b.closes...)
		b.mu.Unlock()
		if starts != 1 || stops != 0 || len(closed) != i+1 || closed[i] != meta.PageID {
			t.Fatalf("wrong startup/owner cleanup: starts=%d stops=%d closes=%v", starts, stops, closed)
		}
		for j := i + 1; j < len(leases); j++ {
			evaluate(j, read, values[j])
		}
	}
	if err := shared.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	starts, stops := b.starts, b.stops
	b.mu.Unlock()
	if starts != 1 || stops != 1 {
		t.Fatalf("owned worker starts=%d stops=%d", starts, stops)
	}
}
