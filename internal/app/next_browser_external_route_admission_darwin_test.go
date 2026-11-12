//go:build darwin

package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/config"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

type untrustedExternalStatus func(context.Context, string) (toolbrowser.ConnectorRuntimeStatus, error)

func (f untrustedExternalStatus) ConnectorStatus(ctx context.Context, id string) (toolbrowser.ConnectorRuntimeStatus, error) {
	return f(ctx, id)
}

// A NEXT-private route config is policy intent only. A working WS MCP in Stable,
// an Edge-owned loopback socket, and even unsealed positive health claims MUST
// NOT grant an external NEXT Browser Broker lease. No processes or network are
// started in this test: only the real config reader and route planner run.
func TestNextPrivateCompanyRoutesRequireBoundExternalAttestor(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".agentdock-next")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	company := t.TempDir()
	profile := t.TempDir()
	endpoint := "ws://127.0.0.1:9222/devtools/browser"
	payload, err := json.Marshal(struct {
		Version  int                                 `json:"version"`
		Catalog  browserpolicy.CatalogDefinition     `json:"catalog"`
		Policies []browserpolicy.WorkspaceRootPolicy `json:"workspace_policies"`
	}{
		Version: 1,
		Catalog: browserpolicy.CatalogDefinition{
			Profiles:   []browserpolicy.ProfileDefinition{{ID: "company-edge", Browser: browserpolicy.BrowserEdge, Class: browserpolicy.ProfileAuthenticatedExternal, UserDataDir: profile}},
			Connectors: []browserpolicy.ConnectorDefinition{{ID: "edge-mcp", ProfileID: "company-edge", Driver: browserpolicy.DriverChromeDevToolsMCPWS, Endpoint: endpoint}},
		},
		Policies: []browserpolicy.WorkspaceRootPolicy{{Root: company, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "edge-mcp", RequiredProfileID: "company-edge", AllowedOrigins: []string{"https://oprueba.com", "https://homiplay.io"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "browser-routes.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_HOME", "")
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv("AGENTDOCK_STDIO", "false")
	t.Setenv("AGENTDOCK_BROWSER_CATALOG_JSON", "")
	t.Setenv("AGENTDOCK_BROWSER_WORKSPACE_POLICIES_JSON", "")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.BrowserCatalog.Connectors) != 1 || len(cfg.BrowserWorkspacePolicies) != 1 {
		t.Fatalf("missing NEXT private route: catalog=%v policies=%v", cfg.BrowserCatalog, cfg.BrowserWorkspacePolicies)
	}
	catalog, err := browserpolicy.NewCatalog(cfg.BrowserCatalog)
	if err != nil {
		t.Fatal(err)
	}
	scope := toolbrowser.RequestScope{WorkspaceID: "company-site-uat", CanonicalWorkspaceRoot: company, OwnerTaskID: "real-owner-required", Provenance: toolbrowser.ScopeACP}
	for _, provider := range []struct {
		name   string
		source toolbrowser.ConnectorStatusProvider
	}{
		{"absent attestor", nil},
		{"unsealed status claims", untrustedExternalStatus(func(_ context.Context, id string) (toolbrowser.ConnectorRuntimeStatus, error) {
			now := time.Now().UTC()
			return toolbrowser.ConnectorRuntimeStatus{
				ConnectorID: id, ProfileID: "company-edge", Endpoint: endpoint,
				ObservedAt: now, ExpiresAt: now.Add(3 * time.Second),
				Healthy: true, Verified: true, Authenticated: true,
				Engine: toolbrowser.EngineChromeDevToolsMCP, EngineVersion: toolbrowser.PreferredEngineVersion,
				Transport: "websocket", Capabilities: []toolbrowser.ConnectorCapability{
					toolbrowser.CapabilityBackgroundPage, toolbrowser.CapabilityNoFocus, toolbrowser.CapabilityLeaseTarget, toolbrowser.CapabilitySafeRelease,
				},
			}, nil
		})},
	} {
		t.Run(provider.name, func(t *testing.T) {
			planner, err := toolbrowser.NewRoutePlanner(cfg.BrowserWorkspacePolicies, catalog, provider.source)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := planner.Plan(scope, nil)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Route != browserpolicy.RouteRequiredExternal || plan.Browser != toolbrowser.Kind(browserpolicy.BrowserEdge) || plan.ConnectorID != "edge-mcp" {
				t.Fatalf("unsafe route plan %+v", plan)
			}
			decision, err := planner.Resolve(context.Background(), scope, nil)
			var routeErr *toolbrowser.Error
			if !errors.As(err, &routeErr) || routeErr.Code != toolbrowser.ErrRequiredRouteUnavailable {
				t.Fatalf("expected fail-closed external admission, decision=%+v err=%v", decision, err)
			}
			if decision != (toolbrowser.RouteDecision{}) {
				t.Fatalf("route minted despite missing trusted evidence: %+v", decision)
			}
		})
	}
}

func TestNextNativeEdgePlannerIsPassiveAndRejectsOtherRuntimeModes(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	company, _ := browserpolicy.CanonicalWorkspaceRoot(t.TempDir())
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{
		Profiles: []browserpolicy.ProfileDefinition{{ID: "company", Browser: browserpolicy.BrowserEdge, Class: browserpolicy.ProfileAuthenticatedExternal,
			UserDataDir: filepath.Join(home, "Library", "Application Support", "Microsoft Edge"), SelectedProfileName: "公司", BindCurrentProfile: true}},
		Connectors: []browserpolicy.ConnectorDefinition{{ID: "company-edge", ProfileID: "company", Driver: browserpolicy.DriverNextEdgeCDP, Endpoint: "ws://127.0.0.1:9222/devtools/browser"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AgentDockHome: filepath.Join(home, ".agentdock-next"), ACPEnabled: true, BrowserEnabled: true, Port: 8767,
		BrowserWorkspacePolicies: []browserpolicy.WorkspaceRootPolicy{{Root: company, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany,
			Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "company-edge", RequiredProfileID: "company", AllowedOrigins: []string{"https://oprueba.com", "https://www.oprueba.com"}}}}}
	planner, err := newRuntimeBrowserRoutePlanner(cfg, catalog)
	if err != nil {
		t.Fatal(err)
	}
	_, err = planner.Resolve(context.Background(), toolbrowser.RequestScope{WorkspaceID: "company", CanonicalWorkspaceRoot: company, OwnerTaskID: "task", Provenance: toolbrowser.ScopeACP}, nil)
	var routeErr *toolbrowser.Error
	if !errors.As(err, &routeErr) || routeErr.Code != toolbrowser.ErrRequiredRouteUnavailable {
		t.Fatal("passive planner connected or published a route without explicit consent", err)
	}
	for _, mutate := range []func(*config.Config){
		func(c *config.Config) { c.Stdio = true }, func(c *config.Config) { c.Port = 8765 }, func(c *config.Config) { c.ACPEnabled = false },
		func(c *config.Config) { c.BrowserEnabled = false }, func(c *config.Config) { c.AgentDockHome = filepath.Join(home, ".agentdock") },
	} {
		bad := cfg
		mutate(&bad)
		if _, err := newRuntimeBrowserRoutePlanner(bad, catalog); err == nil {
			t.Fatal("native Edge admitted outside isolated NEXT mode")
		}
	}
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "stable")
	if _, err := newRuntimeBrowserRoutePlanner(cfg, catalog); err == nil {
		t.Fatal("stable variant admitted native NEXT Edge")
	}
}
