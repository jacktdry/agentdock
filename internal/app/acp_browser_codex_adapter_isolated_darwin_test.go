//go:build darwin

package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/browserdesktop"
	"github.com/uvwt/agentdock/internal/config"
	toolacp "github.com/uvwt/agentdock/internal/tool/acp"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

// Initialize-only fallback: the real ACP adapter talks to a test-owned fake
// native app-server. This does NOT prove SessionNew, injected MCP config, or a
// Browser MCP handshake. No prompt, model, authentication, or browser is used.
func TestACPBrowserCodexAdapterIsolatedOfflineInitialize(t *testing.T) {
	testACPBrowserCodexAdapterIsolatedOffline(t, false)
}

// Account/model/thread replies are synthetic test fixtures, never login or
// model execution evidence. Only the installed ACP adapter is real.
func TestACPBrowserCodexAdapterIsolatedOfflineSessionNew(t *testing.T) {
	testACPBrowserCodexAdapterIsolatedOffline(t, true)
}

func testACPBrowserCodexAdapterIsolatedOffline(t *testing.T, sessionNew bool) {
	t.Helper()
	if os.Getenv("AGENTDOCK_TEST_REAL_CODEX_ACP_OFFLINE") != "1" {
		t.Skip("set AGENTDOCK_TEST_REAL_CODEX_ACP_OFFLINE=1 for isolated real adapter with fake native")
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
	secretPath := filepath.Join(root, "fake-bearer")
	// Runs even on Fatal. Overwrite the private capture before TempDir removal.
	defer func() {
		if secret, err := os.ReadFile(secretPath); err == nil {
			clear(secret)
			if err := os.WriteFile(secretPath, secret, 0600); err != nil {
				t.Error("private bearer zeroing failed")
			}
			if err := os.Remove(secretPath); err != nil {
				t.Error("private bearer removal failed")
			}
		} else if !os.IsNotExist(err) {
			t.Error("private bearer cleanup unavailable")
		}
	}()
	// The wrapper contains only test paths. It execs Node so it creates no
	// additional shell child; stdin EOF and a watchdog bound native lifetime.
	script := filepath.Join(root, "fake-native.cjs")
	native := filepath.Join(root, "fake-codex")
	source := `const fs = require('node:fs');
const readline = require('node:readline');
const path = require('node:path');
const home = process.env.HOME;
const sessionNew = process.env.AGENTDOCK_P8_FAKE_SESSION_NEW === '1';
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
  if (/^[a-zA-Z0-9_/-]{1,64}$/.test(m.method))
    fs.writeFileSync(path.join(home, 'fake-last-method'), m.method, {mode: 0o600});
  let result;
  if (m.method === 'initialize') {
    fs.writeFileSync(path.join(home, 'fake-initialized'), 'initialized', {mode: 0o600});
    result = {userAgent: 'agentdock-offline-test', codexHome: process.env.CODEX_HOME};
  } else if (m.method === 'account/read') {
    // Synthetic test-only account bypasses ACP's auth gate. No real account,
    // login, credentials, Keychain, or native configuration is read or created.
    result = {account: sessionNew ? {type: 'chatgpt', email: 'offline@example.invalid', planType: 'free'} : null, requiresOpenaiAuth: true};
  } else if (m.method === 'config/read') {
    // ACP performs a passive configuration read after initialize. Nothing from
    // the user's native Codex configuration is ever loaded into this fixture.
    result = {config: {}, layers: []};
  } else if (m.method === 'skills/list') {
    result = {data: []};
  } else if (sessionNew && m.method === 'skills/extraRoots/set' &&
             Array.isArray(m.params?.extraRoots) && m.params.extraRoots.length === 0) {
    result = {};
  } else if (sessionNew && m.method === 'model/list') {
    result = {data: [{id: 'offline-test-model', model: 'offline-test-model', displayName: 'Offline fixture',
      description: 'Synthetic only', isDefault: true, hidden: false, inputModalities: ['text'],
      supportedReasoningEfforts: [{reasoningEffort: 'medium', description: 'Synthetic'}],
      defaultReasoningEffort: 'medium'}], nextCursor: null};
  } else if (sessionNew && m.method === 'thread/start') {
    const servers = m.params?.config?.mcp_servers;
    const browser = servers?.['agentdock-browser'];
    const authorization = browser?.http_headers?.Authorization;
    const names = servers ? Object.keys(servers).sort() : [];
    const computer = servers?.['agentdock-computer'];
    const checks = [m.params?.cwd === path.join(home, 'work'), Boolean(servers),
      names.length === 2 && names[0] === 'agentdock-browser' && names[1] === 'agentdock-computer',
      Boolean(browser), Boolean(browser && browser.url === process.env.AGENTDOCK_P8_EXPECTED_MCP_URL),
      typeof authorization === 'string' && /^Bearer \S+$/.test(authorization),
      Boolean(browser?.http_headers && Object.keys(browser.http_headers).length === 1),
      Boolean(computer && computer.url === process.env.AGENTDOCK_P8_EXPECTED_MCP_URL.replace('/acp-browser/', '/acp-computer/')),
      Boolean(computer?.http_headers && Object.keys(computer.http_headers).length === 1 && computer.http_headers.Authorization === authorization)];
    if (checks.some(ok => !ok)) {
      fs.writeFileSync(path.join(home, 'fake-config-checks'), checks.map(ok => ok ? '1' : '0').join(''), {mode: 0o600});
      process.exit(6);
    }
    // Never persist params, URL, or session IDs. Sole secret exception: a
    // private 0600 test capture, zeroed and removed by the Go parent.
    fs.writeFileSync(path.join(home, 'fake-bearer'), authorization, {mode: 0o600, flag: 'wx'});
    fs.writeFileSync(path.join(home, 'fake-config-valid'), 'true', {mode: 0o600});
    result = {thread: {id: 'offline-test-thread', turns: [], cwd: m.params.cwd},
      model: 'offline-test-model', modelProvider: 'openai', reasoningEffort: 'medium'};
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
		"APP_SERVER_LOGS": "", "AGENTDOCK_P8_FAKE_SESSION_NEW": "0",
	} {
		if key == "AGENTDOCK_P8_FAKE_SESSION_NEW" && sessionNew {
			value = "1"
		}
		hostKey := "AGENTDOCK_P8_OFFLINE_" + key
		t.Setenv(hostKey, value)
		env[key] = hostKey
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("private loopback listener unavailable")
	}
	defer listener.Close()
	t.Setenv("AGENTDOCK_P8_OFFLINE_EXPECTED_URL", "http://127.0.0.1:"+strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)+"/internal/acp-browser/mcp")
	env["AGENTDOCK_P8_EXPECTED_MCP_URL"] = "AGENTDOCK_P8_OFFLINE_EXPECTED_URL"
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
		assertIsolatedCodexFilesAbsent(t, root, state, isolated)
		if _, err := os.Lstat(filepath.Join(root, "fake-rejected")); !os.IsNotExist(err) {
			t.Error("fake native received unsupported method during owned lifetime")
		}
		if _, err := os.Stat(filepath.Join(root, "fake-pid")); err == nil {
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
	if sessionNew {
		testIsolatedCodexSessionNew(t, ctx, rt, root, workspace, secretPath)
		return
	}
	t.Log("isolated real adapter initialize only; SessionNew and Browser MCP injection/handshake NOT proven")
}

func assertIsolatedCodexFilesAbsent(t *testing.T, root, state, isolated string) {
	t.Helper()
	for _, dir := range []string{filepath.Join(root, ".codex"), isolated, filepath.Join(state, "acp", "codex", "codex-home")} {
		for _, name := range []string{"auth.json", "config.toml"} {
			if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
				t.Error("private auth/config unexpectedly present after cleanup")
			}
		}
	}
}

func testIsolatedCodexSessionNew(t *testing.T, ctx context.Context, rt *Runtime, root, workspace, secretPath string) {
	t.Helper()
	result, err := rt.acp.Session(ctx, toolacp.SessionRequest{Action: "new", ProfileID: "codex", CWD: workspace})
	if err != nil {
		if method, readErr := os.ReadFile(filepath.Join(root, "fake-rejected")); readErr == nil {
			t.Fatalf("real adapter SessionNew failed; fake native refused method %q (error details withheld)", string(method))
		}
		var toolErr *toolacp.ToolError
		if errors.As(err, &toolErr) {
			_, stageErr := os.Stat(filepath.Join(root, "fake-config-valid"))
			_, secretErr := os.Stat(filepath.Join(root, "fake-bearer"))
			lastMethod, _ := os.ReadFile(filepath.Join(root, "fake-last-method"))
			checks, _ := os.ReadFile(filepath.Join(root, "fake-config-checks"))
			t.Fatalf("real adapter SessionNew failed: tool code=%s category=%s lastNativeMethod=%q checkBits=%q configStage=%t bearerStage=%t (sensitive details withheld)", toolErr.Code, toolErr.Category, string(lastMethod), string(checks), stageErr == nil, secretErr == nil)
		}
		var coded *acpruntime.Error
		if errors.As(err, &coded) {
			t.Fatalf("real adapter SessionNew failed: agentdock code=%s (sensitive details withheld)", coded.Code)
		}
		t.Fatalf("real adapter SessionNew failed: error kind=%T (sensitive details withheld)", err)
	}
	session, ok := result["session"].(acpruntime.SessionRecord)
	if !ok || session.ID == "" {
		t.Fatal("SessionNew record missing")
	}
	assertCounts := func(owners, authenticated int) {
		t.Helper()
		s := rt.RuntimeBrowserDesktop()["snapshot"].(browserdesktop.Snapshot)
		if s.Owners != owners || s.AuthenticatedTransportOwners != authenticated || s.Workers != 0 || s.Leases != 0 {
			t.Fatal("unexpected browser owner/transport/worker/lease counts")
		}
	}
	assertCounts(1, 0)
	if marker, err := os.ReadFile(filepath.Join(root, "fake-config-valid")); err != nil || string(marker) != "true" {
		t.Fatal("native thread/start MCP config validation missing")
	}
	stat, err := os.Lstat(secretPath)
	if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm() != 0600 {
		t.Fatal("private bearer capture permissions invalid")
	}
	secret, err := os.ReadFile(secretPath)
	if err != nil || len(secret) <= len("Bearer ") || !strings.HasPrefix(string(secret), "Bearer ") {
		t.Fatal("private bearer capture missing")
	}
	defer clear(secret)
	endpoint := rt.acpBrowser.MCPServerURL(rt.cfg.Port)
	base := &http.Transport{Proxy: nil}
	defer base.CloseIdleConnections()
	httpClient := &http.Client{Timeout: 3 * time.Second, Transport: browserLoopbackAuthTransport{base, endpoint, string(secret)},
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect forbidden") }}
	// Synthetic SDK uses the real-adapter-injected secret. Fake native never
	// connects to MCP, and no native MCP startup/handshake success is fabricated.
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "offline-test-sdk", Version: "test"}, nil)
	mcpSession, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient, MaxRetries: -1, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal("synthetic SDK initialize failed (details withheld)")
	}
	defer func() {
		if err := mcpSession.Close(); err != nil {
			t.Error("synthetic SDK cleanup failed")
		}
	}()
	tools, err := mcpSession.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil || tools == nil || len(tools.Tools) != 9 || tools.NextCursor != "" {
		t.Fatal("synthetic SDK tools/list failed")
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"browser_acquire", "browser_click", "browser_evaluate", "browser_fill", "browser_navigate", "browser_press_key", "browser_release", "browser_screenshot", "browser_snapshot"}) {
		t.Fatal("unexpected browser tool set")
	}
	assertCounts(1, 1)
	denied, err := mcpSession.CallTool(ctx, &mcpsdk.CallToolParams{Name: "browser_snapshot", Arguments: map[string]any{"lease_id": "fabricated-offline-lease"}})
	if err != nil || denied == nil || !denied.IsError {
		t.Fatal("fabricated lease not rejected")
	}
	encoded, err := json.Marshal(denied.StructuredContent)
	var denial struct {
		Code string `json:"code"`
	}
	if err != nil || json.Unmarshal(encoded, &denial) != nil || denial.Code != toolbrowser.ErrLeaseOwnerMismatch {
		t.Fatal("fabricated lease missed owner gate")
	}
	assertCounts(1, 1)
	if err := rt.acpBrowser.ReleaseSession(ctx, session.ID); err != nil {
		t.Fatal("browser session release failed")
	}
	assertCounts(0, 0)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	if err != nil {
		t.Fatal("revoked request construction failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	response, err := httpClient.Do(req)
	if err != nil {
		t.Fatal("revoked token request failed (details withheld)")
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("released token not rejected with 401")
	}
	assertCounts(0, 0)
	if _, err := os.Lstat(filepath.Join(root, "fake-rejected")); !os.IsNotExist(err) {
		t.Fatal("fake native received unsupported method")
	}
	t.Log("real adapter SessionNew/config injection; synthetic account/model/thread and SDK only; no login, model execution, native MCP handshake, or browser acceptance")
}
