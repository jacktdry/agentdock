//go:build darwin

package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserdesktop"
	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/config"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

const nativeOfflineGate = "AGENTDOCK_TEST_REAL_CODEX_NATIVE_MCP_OFFLINE"

// Upstream must independently review containment before enabling this test.
// Native-alone complements §38's adapter -> fake-native injection proof; this
// does not prove adapter/native integration, model turns, or browser acceptance.
func TestACPBrowserCodexNativeOfflineMCP(t *testing.T) {
	if os.Getenv(nativeOfflineGate) != "1" {
		t.Skip("real native disabled; independent safety review required before opt-in")
	}
	if info, err := os.Stat("/usr/bin/sandbox-exec"); err != nil || !info.Mode().IsRegular() {
		t.Skip("containment phase: sandbox-exec unavailable; no native launch")
	}
	// Pin the installed native binary, never a PATH override or ACP adapter.
	native, err := filepath.EvalSymlinks("/opt/homebrew/bin/codex")
	if err != nil || native != "/opt/homebrew/Caskroom/codex/0.160.0/bin/codex" {
		t.Fatal("binary phase: expected installed Codex 0.160.0; no native launch")
	}
	root, err := os.MkdirTemp("/private/tmp", "agentdock-native-offline-")
	if err != nil {
		t.Fatal("private-root phase: creation failed")
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error("private-root cleanup failed")
		}
	})
	canonical, err := filepath.EvalSymlinks(root)
	info, statErr := os.Lstat(root)
	if err != nil || statErr != nil || canonical != root || !filepath.IsAbs(root) || !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatal("private-root phase: canonical private directory unavailable")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) {
		t.Fatal("private-root phase: owner mismatch")
	}
	home, codexHome, work := filepath.Join(root, "home"), filepath.Join(root, "codex-home"), filepath.Join(root, "work")
	for _, dir := range []string{home, codexHome, work} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal("private-directory phase: creation failed")
		}
	}
	// No os.Environ: credentials, proxy variables, DYLD settings and inherited
	// profiles/configuration are absent. The child never uses the real HOME.
	env := []string{"HOME=" + home, "CODEX_HOME=" + codexHome, "TMPDIR=" + root,
		"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal("listener phase: private loopback unavailable")
	}
	t.Cleanup(func() { _ = listener.Close() })
	port := listener.Addr().(*net.TCPAddr).Port
	profile := nativeOfflineSeatbelt(root, native, port)
	// Validate the exact profile and enforce it on a copied test executable.
	// No native help/version/app-server process precedes this safety preflight.
	nativeOfflinePreflight(t, ctx, root, profile, env, port)

	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	if err != nil {
		t.Fatal("broker phase: catalog failed")
	}
	planner, err := toolbrowser.NewRoutePlanner(nil, catalog, nil)
	if err != nil {
		t.Fatal("broker phase: planner failed")
	}
	bridge, err := toolbrowser.NewACPBridge(planner, toolbrowser.NewWorkerRegistry(toolbrowser.WorkerDependencies{
		Probe: func(context.Context, mcpclient.ServerConfig) (string, error) {
			t.Error("forbidden browser worker probe")
			return "", errors.New("forbidden")
		},
		Open: func(context.Context, mcpclient.ServerConfig) (toolbrowser.WorkerSession, error) {
			t.Error("forbidden browser worker open")
			return nil, errors.New("forbidden")
		},
	}))
	if err != nil {
		t.Fatal("broker phase: bridge failed")
	}
	t.Cleanup(func() {
		if err := bridge.Close(); err != nil {
			t.Error("bridge cleanup failed")
		}
	})
	provider := acpHostCapabilityProvider{bridge: bridge, profileID: "native-offline", port: port}
	servers, err := provider.Servers(ctx, "native-offline-owner", work, nil)
	if err != nil || len(servers) != 1 || servers[0].Name != "agentdock-browser" || len(servers[0].Headers) != 1 {
		t.Fatal("broker phase: registration failed")
	}
	// Browser only: deliberately no computer capability/ACP parity claim.
	rt := &Runtime{cfg: config.Config{BrowserEnabled: true, ACPEnabled: true}, acpBrowser: bridge}
	counts := func(auth int) bool {
		s := rt.RuntimeBrowserDesktop()["snapshot"].(browserdesktop.Snapshot)
		return s.Owners == 1 && s.AuthenticatedTransportOwners == auth && s.Workers == 0 && s.Leases == 0
	}
	if !counts(0) {
		t.Fatal("broker phase: initial owner counts invalid")
	}
	var initialized, notified, listed, forbidden atomic.Bool
	handler := bridge.MCPHTTPHandler()
	server := &http.Server{ReadHeaderTimeout: 2 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			handler.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		_ = r.Body.Close()
		var msg struct {
			Method string `json:"method"`
		}
		if err != nil || json.Unmarshal(body, &msg) != nil {
			forbidden.Store(true)
			http.Error(w, "invalid test protocol", http.StatusBadRequest)
			return
		}
		switch msg.Method {
		case "initialize", "notifications/initialized", "tools/list", "ping":
		default:
			// All tools/call (including browser_acquire) stop before the Broker.
			forbidden.Store(true)
			http.Error(w, "forbidden test method", http.StatusForbidden)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		observed := &nativeOfflineHTTPStatus{ResponseWriter: w}
		handler.ServeHTTP(observed, r)
		if observed.status >= 200 && observed.status < 300 {
			switch msg.Method {
			case "initialize":
				initialized.Store(true)
			case "notifications/initialized":
				notified.Store(true)
			case "tools/list":
				listed.Store(true)
			}
		}
	})}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		if err := <-served; !errors.Is(err, http.ErrServerClosed) {
			t.Error("private HTTP cleanup failed")
		}
	})

	cmd := nativeOfflineCommand(ctx, profile, env, work, native, "app-server", "--listen", "stdio://")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("native-pipes phase: stdin failed")
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("native-pipes phase: stdout failed")
	}
	cmd.Stderr = io.Discard // Never retain raw diagnostics, paths or bearer.
	if err := cmd.Start(); err != nil {
		t.Fatal("native-launch phase: sandboxed launch failed (details withheld)")
	}
	defer func() {
		_ = input.Close()
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // Only this test's new group.
		_ = cmd.Wait()
		if !errors.Is(syscall.Kill(-cmd.Process.Pid, 0), syscall.ESRCH) {
			t.Error("native cleanup phase: owned process group still present")
		}
		if err := provider.ReleaseSession(context.Background(), "native-offline-owner"); err != nil {
			t.Error("owner cleanup failed")
		}
		s := rt.RuntimeBrowserDesktop()["snapshot"].(browserdesktop.Snapshot)
		if s.Owners != 0 || s.AuthenticatedTransportOwners != 0 || s.Workers != 0 || s.Leases != 0 {
			t.Error("owner cleanup counts invalid")
		}
		if forbidden.Load() {
			t.Error("native MCP phase: forbidden protocol method attempted")
		}
	}()
	type reply struct {
		id                      int
		request, result, thread bool
		errorCode               *int
		managedPrefsUnavailable bool
	}
	replies := make(chan reply, 8)
	go func() {
		defer close(replies)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			var wire struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(scanner.Bytes(), &wire) != nil {
				return
			}
			// Only sanitized status crosses the channel. Never retain native
			// response bodies, thread IDs, paths, messages, or arguments.
			msg := reply{request: wire.Method != "" && len(wire.ID) != 0,
				result: len(wire.Result) != 0 && string(wire.Result) != "null"}
			_ = json.Unmarshal(wire.ID, &msg.id)
			if wire.Error != nil {
				code := wire.Error.Code
				msg.errorCode = &code
				// Identifies only this exact platform blocker; no native error
				// message, token, path, or config is copied outside the goroutine.
				msg.managedPrefsUnavailable = strings.Contains(strings.ToLower(wire.Error.Message), "failed to synchronize managed preferences")
			}
			var started struct {
				Thread *struct {
					ID    string            `json:"id"`
					Turns []json.RawMessage `json:"turns"`
				} `json:"thread"`
			}
			if msg.id == 2 && json.Unmarshal(wire.Result, &started) == nil {
				msg.thread = started.Thread != nil && started.Thread.ID != "" && len(started.Thread.Turns) == 0
			}
			select {
			case replies <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	write := func(id any, method string, params any) {
		t.Helper()
		msg := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
		if id != nil {
			msg["id"] = id
		}
		if err := json.NewEncoder(input).Encode(msg); err != nil {
			t.Fatalf("native %s phase: write failed (details withheld)", method)
		}
	}
	wait := func(id int, phase string) {
		t.Helper()
		for {
			select {
			case <-ctx.Done():
				t.Fatalf("native %s phase: deadline; no proof", phase)
			case msg, ok := <-replies:
				if !ok {
					t.Fatalf("native %s phase: protocol ended; no proof", phase)
				}
				if msg.request {
					t.Fatalf("native %s phase: unsolicited server request; no proof", phase)
				}
				if msg.id != id {
					continue
				}
				if msg.errorCode != nil {
					if phase == "thread/start" && *msg.errorCode == -32600 && msg.managedPrefsUnavailable {
						t.Skip("native MCP unqualified: macOS managed preferences unavailable under strict sandbox; HOME/Keychain/network policy unchanged")
					}
					t.Fatalf("native %s phase: JSON-RPC error code=%d (message withheld); no proof", phase, *msg.errorCode)
				}
				if !msg.result || (id == 2 && !msg.thread) {
					t.Fatalf("native %s phase: result missing; no proof", phase)
				}
				return
			}
		}
	}
	write(1, "initialize", map[string]any{"clientInfo": map[string]any{"name": "agentdock-offline-test", "version": "test"}})
	wait(1, "initialize")
	t.Log("real Codex app-server initialize passed within Seatbelt confinement; native MCP handshake still unproven")
	write(nil, "initialized", map[string]any{})
	write(2, "thread/start", map[string]any{"cwd": work, "modelProvider": nil,
		"config": map[string]any{"mcp_servers": map[string]any{"agentdock-browser": map[string]any{
			"url": servers[0].URL, "http_headers": map[string]string{"Authorization": servers[0].Headers[0].Value},
		}}}})
	wait(2, "thread/start")
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for !(initialized.Load() && notified.Load() && listed.Load() && counts(1)) {
		select {
		case <-ctx.Done():
			t.Fatalf("native MCP automatic-start phase: initialize=%t initialized=%t tools-list=%t authenticated-owner=%t; no model fallback, no proof", initialized.Load(), notified.Load(), listed.Load(), counts(1))
		case <-ticker.C:
		}
	}
	t.Log("native-alone MCP initialize/initialized/tools-list and authenticated owner=1; no model turn or browser; ACP integration unproven")
}

// Platform bootstrap permits file reads except the user's entire HOME and
// system Keychains. Writes stay test-root-only; network stays exact-loopback.
// Exec is narrower than process-fork: only the owned probe, true and native.
func nativeOfflineSeatbelt(root, native string, port int) string {
	q := strconv.Quote
	return fmt.Sprintf(`(version 1)
(deny default)
(allow file-read*)
(deny file-read* (subpath "/Users/wei"))
(deny file-read* (subpath "/Library/Keychains") (subpath "/System/Library/Keychains"))
(allow process-fork)
(allow process-exec (literal "/usr/bin/true") (literal %s) (literal %s))
(allow sysctl-read)
(allow file-write* (subpath %s) (literal "/dev/null"))
(allow network-outbound (remote ip "localhost:%d"))
`, q(filepath.Join(root, "probe")), q(native), q(root), port)
}

func nativeOfflineCommand(ctx context.Context, profile string, env []string, work string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", append([]string{"-p", profile}, args...)...)
	cmd.Env, cmd.Dir = env, work
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	return cmd
}

func nativeOfflinePreflight(t *testing.T, ctx context.Context, root, profile string, env []string, port int) {
	t.Helper()
	if err := nativeOfflineCommand(ctx, profile, env, root, "/usr/bin/true").Run(); err != nil {
		t.Fatal("containment profile-validation phase: rejected; no native launch")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal("containment probe phase: executable unavailable")
	}
	source, err := os.Open(self)
	if err != nil {
		t.Fatal("containment probe phase: executable unreadable")
	}
	defer source.Close()
	probe, err := os.OpenFile(filepath.Join(root, "probe"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal("containment probe phase: copy unavailable")
	}
	_, copyErr := io.Copy(probe, source)
	closeErr := probe.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal("containment probe phase: copy failed")
	}
	// Test-owned sibling outside the allowed write subtree. No user file,
	// credential, Keychain, AgentDock or existing service is inspected.
	sentinel, err := os.CreateTemp("/private/tmp", "agentdock-denied-sentinel-")
	if err != nil {
		t.Fatal("containment sentinel phase: creation failed")
	}
	_ = sentinel.Close()
	t.Cleanup(func() { _ = os.Remove(sentinel.Name()) })
	denied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal("containment denied-listener phase: creation failed")
	}
	defer denied.Close()
	probeEnv := append(append([]string{}, env...), nativeOfflineGate+"=1", "AGENTDOCK_NATIVE_CONTAINMENT_PROBE=1",
		"AGENTDOCK_PROBE_ALLOWED_PORT="+strconv.Itoa(port),
		"AGENTDOCK_PROBE_DENIED_PORT="+strconv.Itoa(denied.Addr().(*net.TCPAddr).Port),
		"AGENTDOCK_PROBE_SENTINEL="+sentinel.Name())
	cmd := nativeOfflineCommand(ctx, profile, probeEnv, root, filepath.Join(root, "probe"), "-test.run=^TestACPBrowserCodexNativeContainmentProbe$", "-test.count=1")
	// Output consists solely of fixed test statuses, but discard even that.
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		t.Fatal("containment enforcement phase: probe failed; no native launch (details withheld)")
	}
}

// Re-executed only as a sandboxed child; normal go test skips without sockets.
func TestACPBrowserCodexNativeContainmentProbe(t *testing.T) {
	if os.Getenv(nativeOfflineGate) != "1" || os.Getenv("AGENTDOCK_NATIVE_CONTAINMENT_PROBE") != "1" {
		t.Skip("test-owned containment subprocess only")
	}
	connect := func(ip [4]byte, port int) error {
		fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
		if err != nil {
			return err
		}
		defer syscall.Close(fd)
		return syscall.Connect(fd, &syscall.SockaddrInet4{Addr: ip, Port: port})
	}
	allowed, e1 := strconv.Atoi(os.Getenv("AGENTDOCK_PROBE_ALLOWED_PORT"))
	denied, e2 := strconv.Atoi(os.Getenv("AGENTDOCK_PROBE_DENIED_PORT"))
	if e1 != nil || e2 != nil || allowed <= 0 || denied <= 0 || allowed == denied {
		t.Fatal("probe configuration invalid")
	}
	sentinel := os.Getenv("AGENTDOCK_PROBE_SENTINEL")
	if !strings.HasPrefix(sentinel, "/private/tmp/agentdock-denied-sentinel-") {
		t.Fatal("probe sentinel invalid")
	}
	// Open only the directory itself, never inspect any user file or contents.
	homeDir, err := os.Open("/Users/wei")
	if homeDir != nil {
		_ = homeDir.Close()
	}
	if !errors.Is(err, syscall.EPERM) {
		t.Fatal("user-home directory read not denied with EPERM")
	}
	f, err := os.OpenFile(sentinel, os.O_WRONLY, 0)
	if f != nil {
		_ = f.Close()
	}
	if !errors.Is(err, syscall.EPERM) {
		t.Fatal("outside-root write not denied with EPERM")
	}
	// Establish denial on a reachable, owned OTHER port before any external-IP
	// socket attempt. If containment is absent, stop here without Internet use.
	if err := connect([4]byte{127, 0, 0, 1}, denied); !errors.Is(err, syscall.EPERM) {
		t.Fatal("other loopback port not denied with EPERM")
	}
	if err := connect([4]byte{127, 0, 0, 1}, allowed); err != nil {
		t.Fatal("allowed loopback not reachable")
	}
	// No DNS or HTTP. Seatbelt must reject
	// connect synchronously, before transmitting a packet.
	if err := connect([4]byte{1, 1, 1, 1}, 443); !errors.Is(err, syscall.EPERM) {
		t.Fatal("external socket not denied with EPERM")
	}
}

type nativeOfflineHTTPStatus struct {
	http.ResponseWriter
	status int
}

func (w *nativeOfflineHTTPStatus) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *nativeOfflineHTTPStatus) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (w *nativeOfflineHTTPStatus) Unwrap() http.ResponseWriter { return w.ResponseWriter }
