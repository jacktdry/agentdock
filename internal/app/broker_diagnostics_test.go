package app

import (
	"context"
	"testing"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

func testBrowserBrokerRuntime(t *testing.T) *Runtime {
	t.Helper()
	r := newRuntimeValidationTestRuntime(t)
	cfg := r.cfg
	cfg.BrowserEnabled = true
	cfg.ACPEnabled = true
	cfg.Stdio = false
	names, validators, err := compileAvailableToolContracts(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r.cfg, r.toolNames, r.toolValidators = cfg, names, validators
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := toolbrowser.NewRoutePlanner(nil, catalog, nil)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := toolbrowser.NewACPBridge(planner, toolbrowser.NewWorkerRegistry(toolbrowser.WorkerDependencies{}))
	if err != nil {
		t.Fatal(err)
	}
	r.acpBrowser = bridge
	return r
}

func TestBrowserBrokerPublicStatusAndCleanupMatchOutputContract(t *testing.T) {
	r := testBrowserBrokerRuntime(t)
	token, err := r.acpBrowser.RegisterSession("diag-session", "codex", r.cfg.AgentDockDefaultDir)
	if err != nil || token == "" {
		t.Fatalf("register token=%q err=%v", token, err)
	}
	status, err := r.Call(context.Background(), "browser_broker", map[string]any{"action": "status"})
	if err != nil {
		t.Fatal(err)
	}
	assertToolResultMatchestestOutputSchema(t, "browser_broker", status)
	if status["browser_broker_ok"] != true {
		t.Fatalf("status=%#v", status)
	}
	cleaned, err := r.Call(context.Background(), "browser_broker", map[string]any{"action": "cleanup_acp_session", "owner_acp_session_id": "diag-session"})
	if err != nil {
		t.Fatal(err)
	}
	assertToolResultMatchestestOutputSchema(t, "browser_broker", cleaned)
	if cleaned["browser_broker_ok"] != true || len(r.acpBrowser.Diagnostics().Owners) != 0 {
		t.Fatalf("cleanup=%#v diagnostics=%+v", cleaned, r.acpBrowser.Diagnostics())
	}
}
