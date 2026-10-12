//go:build darwin

package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserdesktop"
	"github.com/uvwt/agentdock/internal/config"
	toolacp "github.com/uvwt/agentdock/internal/tool/acp"
)

// Initialize-only fallback: the real ACP adapter talks to a test-owned fake
// native app-server. This does NOT prove SessionNew, injected MCP config, or a
// Browser MCP handshake. No prompt, model, authentication, or browser is used.
func TestACPBrowserCodexAdapterIsolatedOfflineInitialize(t *testing.T) {
	if os.Getenv("AGENTDOCK_TEST_REAL_CODEX_ACP_OFFLINE") != "1" {
		t.Skip("set AGENTDOCK_TEST_REAL_CODEX_ACP_OFFLINE=1 for isolated real adapter initialize only")
	}
	adapter, err := exec.LookPath("codex-acp")
	if err != nil {
		t.Skip("codex-acp not installed")
	}
	adapter, err = filepath.Abs(adapter)
	if err != nil || adapter != "/opt/homebrew/bin/codex-acp" {
		t.Fatal("expected LookPath to resolve /opt/homebrew/bin/codex-acp")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	node, err = filepath.Abs(node)
	if err != nil {
		t.Fatal("cannot resolve Node executable")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, ".agentdock-next")
	isolated := filepath.Join(state, "codex-mcp-isolated")
	workspace := filepath.Join(root, "work")
	for _, dir := range []string{state, isolated, workspace} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", root)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv(nextCodexMCPRegistrationEnv, "source-only")
	// The wrapper contains only test paths. It execs Node so it creates no
	// additional shell child; stdin EOF and a watchdog bound native lifetime.
	script := filepath.Join(root, "fake-native.cjs")
	native := filepath.Join(root, "fake-codex")
	source := `const fs = require('node:fs');
const readline = require('node:readline');
const path = require('node:path');
const home = process.env.HOME;
if (process.argv[2] !== 'app-server' ||
    process.env.CODEX_HOME !== path.join(home, '.agentdock-next', 'codex-mcp-isolated') ||
    process.env.AGENTDOCK_NEXT_CODEX_MCP_ISOLATION !== 'source-only') process.exit(2);
fs.writeFileSync(path.join(home, 'fake-pid'), String(process.pid), {mode: 0o600});
// The owned process group is terminated with SIGKILL by Controller.Terminate;
// no JavaScript exit/signal handler can run. The Go parent verifies its PID is gone.
setTimeout(() => process.exit(3), 25000);
const input = readline.createInterface({input: process.stdin});
input.on('close', () => process.exit(0));
input.on('line', line => {
  let m;
  try { m = JSON.parse(line); } catch { process.exit(4); }
  if (m.id === undefined) {
    if (m.method !== 'initialized') process.exit(5);
    return;
  }
  let result;
  if (m.method === 'initialize') {
    fs.writeFileSync(path.join(home, 'fake-initialized'), 'initialized', {mode: 0o600});
    result = {userAgent: 'agentdock-offline-test', codexHome: process.env.CODEX_HOME};
  } else if (m.method === 'account/read') {
    // Synthetic unauthenticated status; no auth material is read or created.
    result = {account: null, requiresOpenaiAuth: true};
  } else if (m.method === 'config/read') {
    // ACP performs a passive configuration read after initialize. Nothing from
    // the user's native Codex configuration is ever loaded into this fixture.
    result = {config: {}, layers: []};
  } else if (m.method === 'skills/list') {
    result = {data: []};
  } else {
    fs.writeFileSync(path.join(home, 'fake-rejected'), /^[a-zA-Z0-9_/-]{1,64}$/.test(m.method) ? m.method : 'invalid-method-name', {mode: 0o600});
    process.stdout.write(JSON.stringify({jsonrpc:'2.0', id:m.id, error:{code:-32601, message:'offline method forbidden'}}) + '\n');
    return;
  }
  process.stdout.write(JSON.stringify({jsonrpc:'2.0', id:m.id, result}) + '\n');
});
`
	if err := os.WriteFile(script, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	// Shell single quoting also handles spaces and apostrophes in test paths.
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	if err := os.WriteFile(native, []byte("#!/bin/sh\nexec "+quote(node)+" "+quote(script)+" \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for key, value := range map[string]string{
		"HOME": root, "CODEX_HOME": isolated, "CODEX_PATH": native,
		"AGENTDOCK_NEXT_CODEX_MCP_ISOLATION": "source-only",
		"TMPDIR":                             root, "NODE_PATH": "", "PATH": filepath.Dir(node) + ":/usr/bin:/bin",
	} {
		hostKey := "AGENTDOCK_P8_OFFLINE_" + key
		t.Setenv(hostKey, value)
		env[key] = hostKey
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("private loopback listener unavailable")
	}
	defer listener.Close()
	cfg := config.Config{AgentDockHome: state, AgentDockDefaultDir: workspace,
		Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port,
		BrowserEnabled: true, ACPEnabled: true, ACPDefaultProfile: "codex",
		ACPProfiles: []config.ACPProfile{{ID: "codex", Kind: "codex", Enabled: true, Command: adapter, EnvFromEnv: env}}}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal("isolated runtime construction failed")
	}
	defer func() {
		if err := rt.Close(); err != nil {
			t.Error("owned runtime cleanup failed")
		}
		if _, err := os.Stat(filepath.Join(root, "fake-initialized")); err == nil {
			pidBytes, err := os.ReadFile(filepath.Join(root, "fake-pid"))
			if err != nil {
				t.Error("owned native PID evidence missing")
				return
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
			if err != nil || pid <= 0 {
				t.Error("owned native PID evidence malformed")
				return
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				// PID is from the 0700 test HOME, written by its owned fake process.
				// This is observational only; no unrelated process is signalled.
				if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Error("owned fake native still appears live after adapter shutdown")
		}
	}()
	handler := rt.ACPBrowserMCPHandler()
	if handler == nil {
		t.Fatal("runtime browser MCP handler unavailable")
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Error("loopback shutdown failed")
			_ = server.Close()
		}
		if err := <-served; !errors.Is(err, http.ErrServerClosed) {
			t.Error("loopback serve failed")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "info", ProfileID: "codex"})
	if err != nil || info["agent"] == nil {
		t.Fatal("real adapter ACP initialize failed (details withheld)")
	}
	if got, err := os.ReadFile(filepath.Join(root, "fake-initialized")); err != nil || string(got) != "initialized" {
		t.Fatal("fake native initialize evidence missing")
	}
	if method, err := os.ReadFile(filepath.Join(root, "fake-rejected")); err == nil {
		t.Fatalf("fake native received unsupported method: %s", string(method))
	} else if !os.IsNotExist(err) {
		t.Fatal("fake native rejection evidence unavailable")
	}
	for _, forbidden := range []string{
		filepath.Join(root, ".codex", "auth.json"),
		filepath.Join(root, ".codex", "config.toml"),
		filepath.Join(isolated, "auth.json"),
		filepath.Join(isolated, "config.toml"),
		filepath.Join(state, "acp", "codex", "codex-home", "auth.json"),
	} {
		if _, err := os.Lstat(forbidden); !os.IsNotExist(err) {
			t.Fatal("adapter unexpectedly created/copies auth or native MCP config")
		}
	}
	snapshot := rt.RuntimeBrowserDesktop()["snapshot"].(browserdesktop.Snapshot)
	if snapshot.Owners != 0 || snapshot.AuthenticatedTransportOwners != 0 || snapshot.Workers != 0 || snapshot.Leases != 0 {
		t.Fatal("initialize-only smoke unexpectedly created browser state")
	}
	t.Log("isolated real adapter initialize only; SessionNew and Browser MCP injection/handshake NOT proven")
}
