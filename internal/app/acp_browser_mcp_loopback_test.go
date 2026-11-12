package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/browserdesktop"
	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/config"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

// These are synthetic named SDK clients, not real Codex/Antigravity adapters.
// Only the MCP protocol crosses TCP; no browser lease is acquired.
func TestACPBrowserRealLoopbackMCPHandshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := toolbrowser.NewRoutePlanner(nil, catalog, nil)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := toolbrowser.NewACPBridge(planner, toolbrowser.NewWorkerRegistry(toolbrowser.WorkerDependencies{
		Probe: func(context.Context, mcpclient.ServerConfig) (string, error) {
			t.Error("unexpected worker probe")
			return "", errors.New("worker probe forbidden")
		},
		Open: func(context.Context, mcpclient.ServerConfig) (toolbrowser.WorkerSession, error) {
			t.Error("unexpected worker open")
			return nil, errors.New("worker open forbidden")
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := bridge.Close(); err != nil {
			t.Error("bridge cleanup failed")
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("test loopback listener unavailable")
	}
	server := &http.Server{Handler: bridge.MCPHTTPHandler(), ReadHeaderTimeout: 5 * time.Second}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		if err := server.Shutdown(cleanupCtx); err != nil {
			t.Error("HTTP shutdown failed")
			_ = server.Close()
		}
		_ = listener.Close()
		if err := <-served; !errors.Is(err, http.ErrServerClosed) {
			t.Error("HTTP serve failed")
		}
	})
	runtime := &Runtime{cfg: config.Config{BrowserEnabled: true, ACPEnabled: true}, acpBrowser: bridge}
	assertCounts := func(owners, authenticated int) {
		t.Helper()
		s := runtime.RuntimeBrowserDesktop()["snapshot"].(browserdesktop.Snapshot)
		if s.Availability != "available" || s.Owners != owners || s.AuthenticatedTransportOwners != authenticated || s.Workers != 0 || s.Leases != 0 {
			t.Fatalf("counts: owners=%d authenticated=%d workers=%d leases=%d availability=%s", s.Owners, s.AuthenticatedTransportOwners, s.Workers, s.Leases, s.Availability)
		}
	}
	var providers [2]acpHostCapabilityProvider
	var endpoints [2]acpruntime.SessionMCPServer
	var sessions [2]*mcpsdk.ClientSession
	names := []string{"codex", "antigravity"}
	for i, name := range names {
		providers[i] = acpHostCapabilityProvider{bridge: bridge, profileID: name, port: listener.Addr().(*net.TCPAddr).Port}
		servers, err := providers[i].Servers(ctx, name, workspace, nil)
		if err != nil || len(servers) != 1 {
			t.Fatal("provider registration failed")
		}
		endpoints[i] = servers[0]
		if len(servers[0].Headers) != 1 || servers[0].Headers[0].Name != "Authorization" || !strings.HasPrefix(servers[0].Headers[0].Value, "Bearer ") {
			t.Fatal("provider authorization header missing")
		}
		repeated, err := providers[i].Servers(ctx, name, workspace, nil)
		if err != nil || len(repeated) != 1 || repeated[0].Headers[0] != servers[0].Headers[0] {
			t.Fatal("registration is not idempotent")
		}
	}
	if endpoints[0].Headers[0].Value == endpoints[1].Headers[0].Value {
		t.Fatal("independent owners share a token")
	}
	assertCounts(2, 0)
	base := &http.Transport{Proxy: nil}
	t.Cleanup(base.CloseIdleConnections)
	newHTTPClient := func(endpoint acpruntime.SessionMCPServer) *http.Client {
		return &http.Client{Timeout: 5 * time.Second, Transport: browserLoopbackAuthTransport{base, endpoint.URL, endpoint.Headers[0].Value}, CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("redirect forbidden")
		}}
	}
	want := []string{"browser_acquire", "browser_click", "browser_evaluate", "browser_fill", "browser_navigate", "browser_press_key", "browser_release", "browser_screenshot", "browser_snapshot"}
	listTools := func(session *mcpsdk.ClientSession) {
		t.Helper()
		result, err := session.ListTools(ctx, &mcpsdk.ListToolsParams{})
		if err != nil {
			t.Fatal("SDK tools/list failed")
		}
		var got []string
		for _, tool := range result.Tools {
			got = append(got, tool.Name)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) || result.NextCursor != "" {
			t.Fatalf("unexpected broker tool names: %v", got)
		}
	}
	for i, name := range names {
		client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: name, Version: "test"}, nil)
		sessions[i], err = client.Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: endpoints[i].URL, HTTPClient: newHTTPClient(endpoints[i]), MaxRetries: -1, DisableStandaloneSSE: true}, nil)
		if err != nil {
			t.Fatal("SDK initialize/connect failed")
		}
		session := sessions[i]
		t.Cleanup(func() {
			if err := session.Close(); err != nil {
				t.Error("SDK session cleanup failed")
			}
		})
		listTools(session)
		assertCounts(2, i+1)
	}
	result, err := sessions[0].CallTool(ctx, &mcpsdk.CallToolParams{Name: "browser_snapshot", Arguments: map[string]any{"lease_id": "fabricated-loopback-lease"}})
	if err != nil || result == nil || !result.IsError {
		t.Fatal("fabricated lease was not rejected as a tool error")
	}
	encoded, err := json.Marshal(result.StructuredContent)
	var denial struct {
		Code string `json:"code"`
	}
	if err != nil || json.Unmarshal(encoded, &denial) != nil || denial.Code != toolbrowser.ErrLeaseOwnerMismatch {
		t.Fatal("fabricated lease did not fail the owner gate")
	}
	assertCounts(2, 2)
	assertUnauthorized := func(endpoint acpruntime.SessionMCPServer) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
		if err != nil {
			t.Fatal("request construction failed")
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		response, err := newHTTPClient(endpoint).Do(req)
		if err != nil {
			t.Fatal("unauthorized request failed")
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unauthorized status=%d", response.StatusCode)
		}
	}
	unknown := endpoints[0]
	unknown.Headers = []acpruntime.SessionMCPHeader{{Name: "Authorization", Value: "Bearer fabricated-unknown-token"}}
	assertUnauthorized(unknown)
	assertCounts(2, 2)
	for i, name := range names {
		if err := providers[i].ReleaseSession(ctx, name); err != nil {
			t.Fatal("provider release failed")
		}
		assertCounts(1-i, 1-i)
		assertUnauthorized(endpoints[i])
		assertCounts(1-i, 1-i)
		if i == 0 {
			listTools(sessions[1])
			assertCounts(1, 1)
		}
	}
}

type browserLoopbackAuthTransport struct {
	base                    http.RoundTripper
	endpoint, authorization string
}

func (r browserLoopbackAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() != r.endpoint {
		return nil, errors.New("non-test endpoint forbidden")
	}
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", r.authorization)
	return r.base.RoundTrip(clone)
}
