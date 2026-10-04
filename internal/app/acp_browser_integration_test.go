//go:build browser_integration

package app

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/config"
	toolacp "github.com/uvwt/agentdock/internal/tool/acp"
)

func TestCodexACPUsesInjectedBrowserBrokerWithoutSpawningChromeDevtools(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("process-tree smoke is currently macOS-only")
	}
	codexACP, err := exec.LookPath("codex-acp")
	if err != nil {
		t.Skip("codex-acp not installed")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("session/new error: %#v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	workspace := t.TempDir()
	cfg := config.Config{
		AgentDockHome: t.TempDir(), AgentDockDefaultDir: workspace,
		Host: "127.0.0.1", Port: port, BrowserEnabled: true, ACPEnabled: true,
		ACPProfiles:       []config.ACPProfile{{ID: "codex", Kind: "codex", Command: codexACP, Enabled: true}},
		ACPDefaultProfile: "codex", ACPMaxPrompts: 2, ACPInteractionMS: 30_000,
	}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rt.Close() }()
	h := rt.ACPBrowserMCPHandler()
	if h == nil {
		t.Fatal("ACP browser MCP handler unavailable")
	}
	var requests atomic.Int64
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		h.ServeHTTP(w, r)
	})}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = server.Shutdown(ctx)
		cancel()
		<-done
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "new", ProfileID: "codex", CWD: workspace})
	if err != nil {
		t.Fatalf("session/new error: %#v", err)
	}
	if result["session"] == nil {
		t.Fatalf("session result=%#v", result)
	}
	deadline := time.Now().Add(5 * time.Second)
	for requests.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if requests.Load() == 0 {
		t.Fatal("codex-acp never connected to injected AgentDock Browser Broker MCP")
	}

	commands, err := descendantCommands(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range commands {
		for _, forbidden := range []string{"chrome-devtools-mcp", "cua-repl", "/cua_node/bin/node_repl", "SkyComputerUseClient"} {
			if strings.Contains(command, forbidden) {
				t.Fatalf("adapter-owned browser/computer-control descendant detected (%s): %s", forbidden, command)
			}
		}
	}
}

func descendantCommands(root int) ([]string, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,command=").Output()
	if err != nil {
		return nil, err
	}
	parents := map[int]int{}
	commands := map[int]string{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		parents[pid] = ppid
		commands[pid] = strings.Join(fields[2:], " ")
	}
	isDescendant := func(pid int) bool {
		seen := map[int]bool{}
		for pid > 0 && !seen[pid] {
			seen[pid] = true
			parent, ok := parents[pid]
			if !ok {
				return false
			}
			if parent == root {
				return true
			}
			pid = parent
		}
		return false
	}
	var result []string
	for pid, command := range commands {
		if isDescendant(pid) {
			result = append(result, command)
		}
	}
	return result, nil
}

func TestAntigravityACPRequiresInjectedBrowserBrokerWithoutSpawningBrowserBackends(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("process-tree smoke is currently macOS-only")
	}
	antigravityACP, err := exec.LookPath("antigravity-acp")
	if err != nil {
		antigravityACP = "/Users/wei/.local/bin/antigravity-acp"
		if _, statErr := os.Stat(antigravityACP); statErr != nil {
			t.Skip("antigravity-acp not installed")
		}
	}
	agy, err := exec.LookPath("agy")
	if err != nil {
		agy = "/Users/wei/.local/bin/agy"
		if _, statErr := os.Stat(agy); statErr != nil {
			t.Skip("agy not installed")
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	workspace := t.TempDir()
	t.Setenv("AGENTDOCK_TEST_AGY_BIN", agy)
	t.Setenv("AGENTDOCK_TEST_AGY_SKIP_DOWNLOAD", "1")
	cfg := config.Config{
		AgentDockHome: t.TempDir(), AgentDockDefaultDir: workspace,
		Host: "127.0.0.1", Port: port, BrowserEnabled: true, ACPEnabled: true,
		ACPProfiles: []config.ACPProfile{{
			ID: "antigravity", Kind: "custom", Command: antigravityACP, Enabled: true,
			EnvFromEnv: map[string]string{
				"AGY_BIN":           "AGENTDOCK_TEST_AGY_BIN",
				"AGY_SKIP_DOWNLOAD": "AGENTDOCK_TEST_AGY_SKIP_DOWNLOAD",
			},
		}},
		ACPDefaultProfile: "antigravity", ACPMaxPrompts: 2, ACPInteractionMS: 30_000,
	}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rt.Close() }()
	h := rt.ACPBrowserMCPHandler()
	if h == nil {
		t.Fatal("ACP browser MCP handler unavailable")
	}
	server := &http.Server{Handler: h}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = server.Shutdown(ctx)
		cancel()
		<-done
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "new", ProfileID: "antigravity", CWD: workspace})
	if err != nil {
		t.Fatalf("antigravity session/new error: %#v", err)
	}
	if result["session"] == nil {
		t.Fatalf("antigravity session result=%#v", result)
	}
	time.Sleep(750 * time.Millisecond)
	commands, err := descendantCommands(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range commands {
		for _, forbidden := range []string{"chrome-devtools-mcp", "cua-repl", "/cua_node/bin/node_repl", "SkyComputerUseClient"} {
			if strings.Contains(command, forbidden) {
				t.Fatalf("antigravity adapter-owned browser/computer-control descendant detected (%s): %s", forbidden, command)
			}
		}
	}
}

func TestAntigravityACPBrowserDisabledStillStartsWithoutGlobalBrowserBackends(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("process-tree smoke is currently macOS-only")
	}
	antigravityACP := "/Users/wei/.local/bin/antigravity-acp"
	if _, err := os.Stat(antigravityACP); err != nil {
		t.Skip("antigravity-acp not installed")
	}
	agy := "/Users/wei/.local/bin/agy"
	if _, err := os.Stat(agy); err != nil {
		t.Skip("agy not installed")
	}
	workspace := t.TempDir()
	t.Setenv("AGENTDOCK_TEST_AGY_BIN_DISABLED", agy)
	t.Setenv("AGENTDOCK_TEST_AGY_SKIP_DISABLED", "1")
	cfg := config.Config{
		AgentDockHome: t.TempDir(), AgentDockDefaultDir: workspace,
		Host: "127.0.0.1", Port: 0, BrowserEnabled: false, ACPEnabled: true,
		ACPProfiles: []config.ACPProfile{{
			ID: "antigravity", Kind: "custom", Command: antigravityACP, Enabled: true,
			EnvFromEnv: map[string]string{
				"AGY_BIN":           "AGENTDOCK_TEST_AGY_BIN_DISABLED",
				"AGY_SKIP_DOWNLOAD": "AGENTDOCK_TEST_AGY_SKIP_DISABLED",
			},
		}},
		ACPDefaultProfile: "antigravity", ACPMaxPrompts: 2, ACPInteractionMS: 30_000,
	}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rt.Close() }()
	if rt.ACPBrowserMCPHandler() != nil {
		t.Fatal("browser-disabled runtime unexpectedly exposed ACP Browser Broker MCP")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "new", ProfileID: "antigravity", CWD: workspace})
	if err != nil {
		t.Fatalf("browser-disabled antigravity session/new error: %#v", err)
	}
	if result["session"] == nil {
		t.Fatalf("browser-disabled antigravity session result=%#v", result)
	}
	time.Sleep(750 * time.Millisecond)
	commands, err := descendantCommands(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range commands {
		for _, forbidden := range []string{"chrome-devtools-mcp", "cua-repl", "/cua_node/bin/node_repl", "SkyComputerUseClient"} {
			if strings.Contains(command, forbidden) {
				t.Fatalf("browser-disabled antigravity spawned forbidden backend (%s): %s", forbidden, command)
			}
		}
	}
}
