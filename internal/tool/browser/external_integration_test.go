//go:build browser_integration

package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

func startExternalCDPBrowser(t *testing.T) (string, func()) {
	return startExternalCDPBrowserAt(t, "about:blank")
}

func startExternalCDPBrowserAt(t *testing.T, initialURL string) (string, func()) {
	t.Helper()
	executable := strings.TrimSpace(os.Getenv("AGENTDOCK_BROWSER_EXECUTABLE_PATH"))
	if executable == "" {
		t.Fatal("browser_integration requires AGENTDOCK_BROWSER_EXECUTABLE_PATH and must not skip")
	}
	profileDir, err := os.MkdirTemp("", "agentdock-external-cdp-")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable,
		"--headless=new",
		"--remote-debugging-port=0",
		"--user-data-dir="+profileDir,
		"--no-first-run",
		"--no-default-browser-check",
		initialURL,
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		for range 20 {
			if err := os.RemoveAll(profileDir); err == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		_ = os.RemoveAll(profileDir)
	}

	activePortPath := filepath.Join(profileDir, "DevToolsActivePort")
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(activePortPath)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) > 0 {
				port, parseErr := strconv.Atoi(strings.TrimSpace(lines[0]))
				if parseErr == nil && port > 0 {
					endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
					if probeCDPEndpoint(context.Background(), endpoint) == nil {
						return endpoint, cleanup
					}
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	cleanup()
	t.Fatalf("external Chrome did not publish %s", activePortPath)
	return "", func() {}
}

func TestExternalCDPAttachKeepsBrowserAliveAndIsolatesTargets(t *testing.T) {
	endpoint, cleanup := startExternalCDPBrowser(t)
	defer cleanup()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<title>External CDP</title><main id="ready">external-ready</main>`)
	}))
	defer server.Close()

	service := New(Config{AgentDockHome: t.TempDir()}, nil)
	started, err := service.start(context.Background(), StartRequest{
		CDPURL:  endpoint,
		URL:     server.URL,
		Timeout: 15 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.ConnectionMode != "external_explicit" {
		t.Fatalf("connection mode = %q", started.ConnectionMode)
	}
	if len(started.Pages) != 1 {
		t.Fatalf("external session exposed %d pages, want only AgentDock target: %#v", len(started.Pages), started.Pages)
	}
	if _, err := service.act(context.Background(), ActRequest{
		SessionID: started.SessionID,
		Actions:   []Action{{Kind: "wait_for_text", WaitText: &WaitTextAction{Text: "external-ready", Exact: true, State: StateVisible, Timeout: 5 * time.Second}}},
		Timeout:   10 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.closeSession(CloseRequest{SessionID: started.SessionID}); err != nil {
		t.Fatal(err)
	}
	if err := probeCDPEndpoint(context.Background(), endpoint); err != nil {
		t.Fatalf("external browser stopped after AgentDock session close: %v", err)
	}

	candidates, err := discoverCDPEndpoints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	expectedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, candidate := range candidates {
		candidateURL, parseErr := url.Parse(candidate.URL)
		if parseErr != nil || candidateURL.Host != expectedEndpoint.Host {
			continue
		}
		if candidate.Source == "devtools_active_port" && isBrowserWebSocketURL(candidate.URL) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("auto-discovery did not find DevToolsActivePort browser websocket for %s: %#v", endpoint, candidates)
	}
}

func externalCDPPages(t *testing.T, endpoint string) []map[string]any {
	t.Helper()
	u, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/json/list"
	u.RawQuery = ""
	u.Fragment = ""
	resp, err := directHTTPClient(5 * time.Second).Get(u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("unexpected page-list status: %s", resp.Status)
	}
	var targets []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		t.Fatal(err)
	}
	pages := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		if target["type"] == "page" {
			pages = append(pages, target)
		}
	}
	return pages
}

func externalCDPPageCount(t *testing.T, endpoint string) int {
	return len(externalCDPPages(t, endpoint))
}

func externalCDPHasURL(t *testing.T, endpoint, want string) bool {
	t.Helper()
	for _, page := range externalCDPPages(t, endpoint) {
		if got, _ := page["url"].(string); got == want || strings.HasPrefix(got, want+"/") {
			return true
		}
	}
	return false
}

func TestExternalLeaseManagerKeepsBrowserAliveAndClosesOnlyOwnedPage(t *testing.T) {
	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<title>User Sentinel</title><main>user-page</main>`)
	}))
	defer userServer.Close()
	endpoint, cleanup := startExternalCDPBrowserAt(t, userServer.URL)
	defer cleanup()
	if !externalCDPHasURL(t, endpoint, userServer.URL) {
		t.Fatalf("external user sentinel page missing before broker attach: %#v", externalCDPPages(t, endpoint))
	}
	wsEndpoint, err := resolveCDPWebSocket(context.Background(), endpoint, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<title>Broker External Lease</title><main id="ready">broker-external-ready</main>`)
	}))
	defer server.Close()

	workspace := filepath.Clean(t.TempDir())
	route := RouteDecision{
		Scope: RequestScope{
			WorkspaceID:            "integration-external",
			CanonicalWorkspaceRoot: workspace,
			OwnerTaskID:            "integration-task",
			Provenance:             ScopeRuntime,
		},
		Route: browserpolicy.RouteExternal,
		Start: ResolvedStart{
			Browser:          BrowserChrome,
			Engine:           EngineChromeDevToolsMCP,
			EngineVersion:    PreferredEngineVersion,
			ConnectorID:      "integration-external-ws",
			ProfileID:        "integration-external-profile",
			ProfileClass:     ProfileExternal,
			Endpoint:         wsEndpoint,
			BackgroundPage:   true,
			ForegroundPolicy: ForegroundForbidden,
			LifecyclePolicy:  LifecycleExternal,
			Ownership: ResourceOwnership{
				Process:   OwnerExternalPersistent,
				Profile:   OwnerExternalPersistent,
				Connector: OwnerAgentDockIsolated,
			},
		},
	}
	registry := NewWorkerRegistry(WorkerDependencies{})
	defer func() { _ = registry.Shutdown(context.Background()) }()
	leases := NewExternalLeaseManager(registry)
	meta, _, err := leases.Acquire(context.Background(), route, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !externalCDPHasURL(t, endpoint, userServer.URL) {
		t.Fatal("broker acquire removed or replaced external user page")
	}
	if !externalCDPHasURL(t, endpoint, server.URL) {
		t.Fatalf("broker-owned page missing after acquire: %#v", externalCDPPages(t, endpoint))
	}
	if _, err := leases.Call(context.Background(), route.Scope, meta.BrowserLeaseID, "take_snapshot", nil); err != nil {
		t.Fatal(err)
	}
	cleanupMeta, err := leases.Release(context.Background(), route.Scope, meta.BrowserLeaseID)
	if err != nil || cleanupMeta.CleanupState != CleanupComplete || cleanupMeta.CleanupError != "" {
		t.Fatalf("release = %+v, %v", cleanupMeta, err)
	}
	if err := probeCDPEndpoint(context.Background(), endpoint); err != nil {
		t.Fatalf("external browser stopped after connector release: %v", err)
	}
	if !externalCDPHasURL(t, endpoint, userServer.URL) {
		t.Fatal("release removed external user page")
	}
	if externalCDPHasURL(t, endpoint, server.URL) {
		t.Fatal("release left AgentDock-owned page behind")
	}
}
