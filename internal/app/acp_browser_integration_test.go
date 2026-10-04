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

	acpruntime "github.com/uvwt/agentdock/internal/acp"
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

func TestAntigravityACPTwentyEphemeralLifecycleStress(t *testing.T) {
	if os.Getenv("AGENTDOCK_RUN_STRESS") != "1" {
		t.Skip("set AGENTDOCK_RUN_STRESS=1 for the 20-cycle adapter lifecycle stress")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("process-tree stress is currently macOS-only")
	}
	antigravityACP := "/Users/wei/.local/bin/antigravity-acp"
	if resolved, err := exec.LookPath("antigravity-acp"); err == nil {
		antigravityACP = resolved
	}
	if _, err := os.Stat(antigravityACP); err != nil {
		t.Skip("antigravity-acp not installed")
	}
	versionBytes, err := exec.Command(antigravityACP, "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(versionBytes)); got != "1.2.0-agentdock.5" {
		t.Fatalf("antigravity-acp version=%q, want 1.2.0-agentdock.5", got)
	}
	agy := "/Users/wei/.local/bin/agy"
	if resolved, err := exec.LookPath("agy"); err == nil {
		agy = resolved
	}
	if _, err := os.Stat(agy); err != nil {
		t.Skip("agy not installed")
	}
	t.Setenv("AGENTDOCK_STRESS_AGY_BIN", agy)
	t.Setenv("AGENTDOCK_STRESS_AGY_SKIP_DOWNLOAD", "1")

	for iteration := 0; iteration < 20; iteration++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("cycle %d listen: %v", iteration, err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		workspace := t.TempDir()
		cfg := config.Config{
			AgentDockHome: t.TempDir(), AgentDockDefaultDir: workspace,
			Host: "127.0.0.1", Port: port, BrowserEnabled: true, ACPEnabled: true,
			ACPProfiles: []config.ACPProfile{{
				ID: "antigravity", Kind: "custom", Command: antigravityACP, Enabled: true,
				EnvFromEnv: map[string]string{
					"AGY_BIN":           "AGENTDOCK_STRESS_AGY_BIN",
					"AGY_SKIP_DOWNLOAD": "AGENTDOCK_STRESS_AGY_SKIP_DOWNLOAD",
				},
			}},
			ACPDefaultProfile: "antigravity", ACPMaxPrompts: 1, ACPInteractionMS: 30_000,
		}
		if err := cfg.Normalize(); err != nil {
			_ = listener.Close()
			t.Fatalf("cycle %d normalize: %v", iteration, err)
		}
		rt, err := NewRuntime(cfg)
		if err != nil {
			_ = listener.Close()
			t.Fatalf("cycle %d runtime: %v", iteration, err)
		}
		mux := http.NewServeMux()
		if h := rt.ACPBrowserMCPHandler(); h != nil {
			mux.Handle("/internal/acp-browser/mcp", h)
		}
		if h := rt.ACPComputerMCPHandler(); h != nil {
			mux.Handle("/internal/acp-computer/mcp", h)
		}
		server := &http.Server{Handler: mux}
		serverDone := make(chan error, 1)
		go func() { serverDone <- server.Serve(listener) }()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		result, sessionErr := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "new", ProfileID: "antigravity", CWD: workspace})
		cancel()
		if sessionErr != nil {
			_ = rt.Close()
			_ = server.Close()
			<-serverDone
			t.Fatalf("cycle %d session/new: %v", iteration, sessionErr)
		}
		session, ok := result["session"].(acpruntime.SessionRecord)
		if !ok || session.ID == "" {
			_ = rt.Close()
			_ = server.Close()
			<-serverDone
			t.Fatalf("cycle %d session result=%#v", iteration, result)
		}
		owners := rt.acpBrowser.Diagnostics().Owners
		if len(owners) != 1 || owners[0].ACPSessionID != session.ID {
			_ = rt.Close()
			_ = server.Close()
			<-serverDone
			t.Fatalf("cycle %d browser owners=%+v session=%s", iteration, owners, session.ID)
		}

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		_, closeErr := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "close", ProfileID: "antigravity", SessionID: session.ID})
		cancel()
		if closeErr != nil {
			_ = rt.Close()
			_ = server.Close()
			<-serverDone
			t.Fatalf("cycle %d session/close: %v", iteration, closeErr)
		}
		if owners := rt.acpBrowser.Diagnostics().Owners; len(owners) != 0 {
			_ = rt.Close()
			_ = server.Close()
			<-serverDone
			t.Fatalf("cycle %d capability owner leaked after close: %+v", iteration, owners)
		}
		if err := rt.Close(); err != nil {
			t.Fatalf("cycle %d runtime close: %v", iteration, err)
		}
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = server.Shutdown(shutdownCtx)
		shutdownCancel()
		<-serverDone

		deadline := time.Now().Add(3 * time.Second)
		for {
			commands, err := descendantCommands(os.Getpid())
			if err != nil {
				t.Fatal(err)
			}
			leaked := ""
			for _, command := range commands {
				for _, forbidden := range []string{"antigravity-acp", "chrome-devtools-mcp", "cua-repl", "/cua_node/bin/node_repl", "SkyComputerUseClient"} {
					if strings.Contains(command, forbidden) {
						leaked = command
						break
					}
				}
				if leaked != "" {
					break
				}
			}
			if leaked == "" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("cycle %d leaked descendant after runtime close: %s", iteration, leaked)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
