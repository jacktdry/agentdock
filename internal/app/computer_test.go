package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/browserpolicy"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
	toolcomputer "github.com/uvwt/agentdock/internal/tool/computer"
)

type appFakeComputerProvider struct{}

func (appFakeComputerProvider) ID() toolcomputer.ProviderID { return toolcomputer.ProviderOrca }
func (appFakeComputerProvider) FrontmostApp(context.Context) (*toolcomputer.AppIdentity, error) {
	return &toolcomputer.AppIdentity{Name: "ChatGPT", BundleID: "com.openai.codex", PID: 1}, nil
}
func (appFakeComputerProvider) Observe(_ context.Context, req toolcomputer.ObservationRequest) (map[string]any, error) {
	return map[string]any{"kind": req.Action, "supports": map[string]any{"apps": true}}, nil
}
func (appFakeComputerProvider) Act(_ context.Context, req toolcomputer.ActionRequest) (map[string]any, error) {
	return map[string]any{"kind": req.Action, "verification": map[string]any{"status": "verified"}}, nil
}

func testComputerRuntime(t *testing.T) *Runtime {
	t.Helper()
	r := newRuntimeValidationTestRuntime(t)
	broker, err := toolcomputer.NewBroker(appFakeComputerProvider{})
	if err != nil {
		t.Fatal(err)
	}
	r.computer = toolcomputer.NewServiceWithBroker(broker)
	return r
}

func TestComputerPublicToolsMatchOutputContracts(t *testing.T) {
	r := testComputerRuntime(t)
	acquired, err := r.Call(context.Background(), "computer_session", map[string]any{"action": "acquire", "capability": "observe", "foreground": "forbidden"})
	if err != nil {
		t.Fatal(err)
	}
	assertToolResultMatchestestOutputSchema(t, "computer_session", acquired)
	id, _ := acquired["computer_session_id"].(string)
	if id == "" {
		t.Fatalf("acquired=%#v", acquired)
	}
	observed, err := r.Call(context.Background(), "computer_observe", map[string]any{"session_id": id, "action": "capabilities"})
	if err != nil {
		t.Fatal(err)
	}
	assertToolResultMatchestestOutputSchema(t, "computer_observe", observed)

	actSession, err := r.Call(context.Background(), "computer_session", map[string]any{"action": "acquire", "capability": "act", "foreground": "allowed"})
	if err != nil {
		t.Fatal(err)
	}
	actID, _ := actSession["computer_session_id"].(string)
	acted, err := r.Call(context.Background(), "computer_act", map[string]any{"session_id": actID, "action": "click", "app": "Test", "element_index": 1})
	if err != nil {
		t.Fatal(err)
	}
	assertToolResultMatchestestOutputSchema(t, "computer_act", acted)
	if acted["action_verification"] != "verified" {
		t.Fatalf("acted=%#v", acted)
	}
}

func TestComputerPublicToolForegroundGateFailsClosed(t *testing.T) {
	r := testComputerRuntime(t)
	acquired, err := r.Call(context.Background(), "computer_session", map[string]any{"action": "acquire", "capability": "act", "foreground": "forbidden"})
	if err != nil {
		t.Fatal(err)
	}
	id := acquired["computer_session_id"].(string)
	acted, err := r.Call(context.Background(), "computer_act", map[string]any{"session_id": id, "action": "click", "app": "Test", "element_index": 1})
	if err != nil {
		t.Fatal(err)
	}
	if acted["computer_ok"] != false || acted["code"] != toolcomputer.ErrForegroundRequired {
		t.Fatalf("acted=%#v", acted)
	}
}

func TestACPHostCapabilityProviderSharesTokenAcrossBrowserAndComputerSurfaces(t *testing.T) {
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := toolbrowser.NewRoutePlanner(nil, catalog, nil)
	if err != nil {
		t.Fatal(err)
	}
	browserBridge, err := toolbrowser.NewACPBridge(planner, toolbrowser.NewWorkerRegistry(toolbrowser.WorkerDependencies{}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = browserBridge.Close() }()
	computerBroker, err := toolcomputer.NewBroker(appFakeComputerProvider{})
	if err != nil {
		t.Fatal(err)
	}
	computerBridge, err := toolcomputer.NewACPBridge(computerBroker)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = computerBridge.Close()
		_ = computerBroker.Close()
	}()
	provider := acpHostCapabilityProvider{bridge: browserBridge, computer: computerBridge, profileID: "antigravity", serverName: "agentdock-browser", port: 27123}
	servers, err := provider.Servers(context.Background(), "acps-shared", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("servers=%#v", servers)
	}
	byName := map[string]acpruntime.SessionMCPServer{}
	for _, server := range servers {
		byName[server.Name] = server
	}
	browserServer, browserOK := byName["agentdock-browser"]
	computerServer, computerOK := byName["agentdock-computer"]
	if !browserOK || !computerOK {
		t.Fatalf("servers=%#v", servers)
	}
	if browserServer.URL != "http://127.0.0.1:27123/internal/acp-browser/mcp" || computerServer.URL != "http://127.0.0.1:27123/internal/acp-computer/mcp" {
		t.Fatalf("browser=%#v computer=%#v", browserServer, computerServer)
	}
	if len(browserServer.Headers) != 1 || len(computerServer.Headers) != 1 || browserServer.Headers[0].Value == "" || browserServer.Headers[0].Value != computerServer.Headers[0].Value {
		t.Fatalf("capability headers browser=%#v computer=%#v", browserServer.Headers, computerServer.Headers)
	}
	if err := provider.ReleaseSession(context.Background(), "acps-shared"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		handler http.Handler
	}{
		{name: "browser", handler: browserBridge.MCPHTTPHandler()},
		{name: "computer", handler: computerBridge.MCPHTTPHandler()},
	} {
		req := httptest.NewRequest(http.MethodPost, "/internal/acp-"+tc.name+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
		req.Header.Set("Authorization", browserServer.Headers[0].Value)
		rec := httptest.NewRecorder()
		tc.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s revoked token status=%d body=%s", tc.name, rec.Code, rec.Body.String())
		}
	}
}

func TestComputerBrokerPublicStatusAndCleanupMatchOutputContract(t *testing.T) {
	r := testComputerRuntime(t)
	bridge, err := toolcomputer.NewACPBridge(r.computer.Broker())
	if err != nil {
		t.Fatal(err)
	}
	r.acpComputer = bridge
	token, err := bridge.RegisterSession("computer-diag", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.Acquire(token, toolcomputer.CapabilityObserve, toolcomputer.ForegroundForbidden); err != nil {
		t.Fatal(err)
	}
	status, err := r.Call(context.Background(), "computer_broker", map[string]any{"action": "status"})
	if err != nil {
		t.Fatal(err)
	}
	assertToolResultMatchestestOutputSchema(t, "computer_broker", status)
	if status["computer_broker_ok"] != true {
		t.Fatalf("status=%#v", status)
	}
	cleaned, err := r.Call(context.Background(), "computer_broker", map[string]any{"action": "cleanup_acp_session", "owner_acp_session_id": "computer-diag"})
	if err != nil {
		t.Fatal(err)
	}
	assertToolResultMatchestestOutputSchema(t, "computer_broker", cleaned)
	if cleaned["computer_broker_ok"] != true || len(r.computer.Broker().Diagnostics().ActiveSessions) != 0 {
		t.Fatalf("cleanup=%#v diagnostics=%+v", cleaned, r.computer.Broker().Diagnostics())
	}
}
