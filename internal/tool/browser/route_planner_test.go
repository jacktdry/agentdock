package browser

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

type testConnectorStatus func(context.Context, string) (ConnectorRuntimeStatus, error)

func (f testConnectorStatus) ConnectorStatus(ctx context.Context, id string) (ConnectorRuntimeStatus, error) {
	return f(ctx, id)
}

func plannerFixture(t *testing.T, provider ConnectorStatusProvider) (*RoutePlanner, RequestScope) {
	t.Helper()
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{
		Profiles:   []browserpolicy.ProfileDefinition{{ID: "user-edge", Browser: browserpolicy.BrowserEdge, Class: browserpolicy.ProfileAuthenticatedExternal}},
		Connectors: []browserpolicy.ConnectorDefinition{{ID: "edge-ws", ProfileID: "user-edge", Driver: browserpolicy.DriverChromeDevToolsMCPWS, Endpoint: "ws://127.0.0.1:9222/devtools/browser"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := RequestScope{WorkspaceID: "workspace", CanonicalWorkspaceRoot: t.TempDir(), OwnerTaskID: "task", Provenance: ScopeACP}
	policies := []browserpolicy.WorkspaceRootPolicy{{Root: scope.CanonicalWorkspaceRoot, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "edge-ws", RequiredProfileID: "user-edge"}}}
	planner, err := NewRoutePlanner(policies, catalog, provider)
	if err != nil {
		t.Fatal(err)
	}
	// Caller mutations cannot weaken the planner's trusted policy snapshot.
	policies[0].Policy = browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceDefault, Route: browserpolicy.RouteManaged}
	return planner, scope
}

func assertRouteCode(t *testing.T, err error, code string) {
	t.Helper()
	var be *Error
	if !errors.As(err, &be) || be.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
}

func TestPlannerCompanyIntentAndCanonicalScope(t *testing.T) {
	planner, scope := plannerFixture(t, nil)
	canonical, err := browserpolicy.CanonicalWorkspaceRoot(scope.CanonicalWorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(scope.CanonicalWorkspaceRoot, alias); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{scope.CanonicalWorkspaceRoot, alias, alias + string(filepath.Separator) + "."} {
		scope.CanonicalWorkspaceRoot = root
		plan, err := planner.Plan(scope, nil)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Scope.CanonicalWorkspaceRoot != canonical || plan.Route != browserpolicy.RouteRequiredExternal || plan.ProfileID != "user-edge" || plan.ConnectorID != "edge-ws" || plan.Browser != BrowserEdge || plan.ProfileClass != ProfileAuthenticatedExternal || plan.Endpoint != "ws://127.0.0.1:9222/devtools/browser" || plan.ProfileTemplate != "" || plan.Engine != EngineChromeDevToolsMCP || plan.EngineVersion != PreferredEngineVersion || plan.Headless || len(plan.RequiredCapabilities) != 4 {
			t.Fatalf("plan=%+v", plan)
		}
		if plan.Ownership != (ResourceOwnership{Process: OwnerExternalPersistent, Profile: OwnerExternalPersistent, Connector: OwnerAgentDockIsolated}) {
			t.Fatalf("ownership=%+v", plan.Ownership)
		}
		decision, err := planner.Resolve(context.Background(), scope, nil)
		assertRouteCode(t, err, ErrRequiredRouteUnavailable)
		if decision != (RouteDecision{}) {
			t.Fatalf("fail-open decision=%+v", decision)
		}
		plan.RequiredCapabilities[0] = "caller-mutation"
		again, err := planner.Plan(scope, nil)
		if err != nil || again.RequiredCapabilities[0] != CapabilityBackgroundPage {
			t.Fatalf("plan mutation: %+v %v", again, err)
		}
	}
	scope.CanonicalWorkspaceRoot = filepath.Join(alias, "missing")
	_, err = planner.Plan(scope, nil)
	assertRouteCode(t, err, ErrScopeRequired)
}

func validRuntimeStatus() ConnectorRuntimeStatus {
	return ConnectorRuntimeStatus{Healthy: true, Verified: true, Authenticated: true, Engine: EngineChromeDevToolsMCP, EngineVersion: PreferredEngineVersion, Transport: "websocket", Capabilities: requiredRouteCapabilities()}
}

func TestPlannerRuntimeFailClosed(t *testing.T) {
	cases := []struct {
		name        string
		mutate      func(*ConnectorRuntimeStatus)
		providerErr error
	}{
		{name: "valid"},
		{name: "missing status", mutate: func(s *ConnectorRuntimeStatus) { *s = ConnectorRuntimeStatus{} }},
		{name: "unhealthy", mutate: func(s *ConnectorRuntimeStatus) { s.Healthy = false }},
		{name: "unverified", mutate: func(s *ConnectorRuntimeStatus) { s.Verified = false }},
		{name: "unauthenticated", mutate: func(s *ConnectorRuntimeStatus) { s.Authenticated = false }},
		{name: "native engine", mutate: func(s *ConnectorRuntimeStatus) { s.Engine = EngineNativeCDP }},
		{name: "unknown engine", mutate: func(s *ConnectorRuntimeStatus) { s.Engine = "other" }},
		{name: "wrong version", mutate: func(s *ConnectorRuntimeStatus) { s.EngineVersion = "1.8.0" }},
		{name: "missing version", mutate: func(s *ConnectorRuntimeStatus) { s.EngineVersion = "" }},
		{name: "wrong transport", mutate: func(s *ConnectorRuntimeStatus) { s.Transport = "stdio" }},
		{name: "missing transport", mutate: func(s *ConnectorRuntimeStatus) { s.Transport = "" }},
		{name: "provider failure", providerErr: errors.New("unavailable")},
	}
	for _, capability := range requiredRouteCapabilities() {
		want := capability
		cases = append(cases, struct {
			name        string
			mutate      func(*ConnectorRuntimeStatus)
			providerErr error
		}{name: "missing " + string(want), mutate: func(s *ConnectorRuntimeStatus) {
			s.Capabilities = nil
			for _, c := range requiredRouteCapabilities() {
				if c != want {
					s.Capabilities = append(s.Capabilities, c)
				}
			}
		}})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := validRuntimeStatus()
			if tc.mutate != nil {
				tc.mutate(&status)
			}
			calls := 0
			provider := testConnectorStatus(func(ctx context.Context, id string) (ConnectorRuntimeStatus, error) {
				calls++
				if id != "edge-ws" || ctx.Value("test-context") != "passed" {
					t.Fatalf("provider args: %s %v", id, ctx)
				}
				return status, tc.providerErr
			})
			planner, scope := plannerFixture(t, provider)
			if _, err := planner.Plan(scope, nil); err != nil || calls != 0 {
				t.Fatalf("Plan queried runtime: %v calls=%d", err, calls)
			}
			decision, err := planner.Resolve(context.WithValue(context.Background(), "test-context", "passed"), scope, nil)
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			if tc.name != "valid" {
				assertRouteCode(t, err, ErrRequiredRouteUnavailable)
				if decision != (RouteDecision{}) {
					t.Fatalf("fail-open decision=%+v", decision)
				}
				return
			}
			if err != nil || decision.Route != browserpolicy.RouteRequiredExternal || decision.Start.Browser != BrowserEdge || decision.Start.ProfileID != "user-edge" || decision.Start.ConnectorID != "edge-ws" || decision.Start.EngineVersion != PreferredEngineVersion || decision.Start.LifecyclePolicy != LifecycleExternal || decision.Start.ProfilePath != "" || decision.Start.ForegroundPolicy != ForegroundForbidden || !decision.Start.BackgroundPage || decision.Start.Ownership != (ResourceOwnership{Process: OwnerExternalPersistent, Profile: OwnerExternalPersistent, Connector: OwnerAgentDockIsolated}) {
				t.Fatalf("decision=%+v error=%v", decision, err)
			}
		})
	}
}

func TestPlannerOverridesAndDefault(t *testing.T) {
	provider := testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) { return validRuntimeStatus(), nil })
	planner, scope := plannerFixture(t, provider)
	for _, override := range []*RouteOverride{
		{Route: browserpolicy.RouteManaged, ExplicitUserInstruction: true},
		{Route: browserpolicy.RouteExternal, ConnectorID: "other", ExplicitUserInstruction: true},
		{Route: browserpolicy.RouteExternal, ConnectorID: "edge-ws", ProfileID: "other", ExplicitUserInstruction: true},
		{Route: browserpolicy.RouteExternal, ConnectorID: "edge-ws", Browser: BrowserChrome, ExplicitUserInstruction: true},
		{Route: browserpolicy.RouteExternal, ConnectorID: "edge-ws"},
	} {
		_, err := planner.Plan(scope, override)
		assertRouteCode(t, err, ErrPolicyConflict)
		_, err = planner.Resolve(context.Background(), scope, override)
		assertRouteCode(t, err, ErrPolicyConflict)
	}
	matching := &RouteOverride{Route: browserpolicy.RouteExternal, ConnectorID: "edge-ws", Browser: BrowserEdge, ExplicitUserInstruction: true}
	decision, err := planner.Resolve(context.Background(), scope, matching)
	if err != nil || decision.Route != browserpolicy.RouteRequiredExternal {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
	scope.CanonicalWorkspaceRoot = t.TempDir()
	planner.status = testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) {
		t.Fatal("default queried Edge status")
		return ConnectorRuntimeStatus{}, nil
	})
	plan, err := planner.Plan(scope, nil)
	if err != nil || plan.Route != browserpolicy.RouteManaged || plan.ProfileTemplate != ManagedIsolatedChrome || plan.ProfileID != "" || plan.ConnectorID != "" || plan.Endpoint != "" || plan.Browser != BrowserChrome || plan.ProfileClass != ProfileIsolated || plan.EngineVersion != PreferredEngineVersion || !plan.Headless {
		t.Fatalf("plan=%+v error=%v", plan, err)
	}
	decision, err = planner.Resolve(context.Background(), scope, nil)
	if err != nil || decision.Route != browserpolicy.RouteManaged || !decision.Start.Headless {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
	_, err = planner.Plan(scope, matching)
	assertRouteCode(t, err, ErrPolicyConflict)
	allowed, err := NewRoutePlanner([]browserpolicy.WorkspaceRootPolicy{{Root: scope.CanonicalWorkspaceRoot, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceDefault, Route: browserpolicy.RouteManaged, AllowExplicitExternal: true}}}, planner.catalog, provider)
	if err != nil {
		t.Fatal(err)
	}
	decision, err = allowed.Resolve(context.Background(), scope, matching)
	if err != nil || decision.Route != browserpolicy.RouteExternal || decision.Start.Browser != BrowserEdge {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
}

func TestPlannerRejectsInvalidPolicyCatalog(t *testing.T) {
	planner, scope := plannerFixture(t, nil)
	bad := []browserpolicy.WorkspaceRootPolicy{{Root: scope.CanonicalWorkspaceRoot, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "missing", RequiredProfileID: "user-edge"}}}
	if _, err := NewRoutePlanner(bad, planner.catalog, nil); err == nil {
		t.Fatal("dangling required route accepted")
	}
}

func TestPlannerExplicitExternalPersistentDoesNotRequireAuthentication(t *testing.T) {
	root := t.TempDir()
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{
		Profiles: []browserpolicy.ProfileDefinition{
			{ID: "user-chrome", Browser: browserpolicy.BrowserChrome, Class: browserpolicy.ProfileExternalPersistent},
		},
		Connectors: []browserpolicy.ConnectorDefinition{
			{ID: "chrome-ws", ProfileID: "user-chrome", Driver: browserpolicy.DriverChromeDevToolsMCPWS, Endpoint: "ws://127.0.0.1:9333/devtools/browser"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	status := validRuntimeStatus()
	status.Authenticated = false
	provider := testConnectorStatus(func(_ context.Context, id string) (ConnectorRuntimeStatus, error) {
		if id != "chrome-ws" {
			t.Fatalf("connector=%q", id)
		}
		return status, nil
	})
	planner, err := NewRoutePlanner([]browserpolicy.WorkspaceRootPolicy{{
		Root: root,
		Policy: browserpolicy.BrowserRoutePolicy{
			Class:                 browserpolicy.WorkspaceDefault,
			Route:                 browserpolicy.RouteManaged,
			AllowExplicitExternal: true,
		},
	}}, catalog, provider)
	if err != nil {
		t.Fatal(err)
	}
	scope := RequestScope{WorkspaceID: "personal", CanonicalWorkspaceRoot: root, OwnerTaskID: "task", Provenance: ScopeUpstream}
	override := &RouteOverride{Route: browserpolicy.RouteExternal, ConnectorID: "chrome-ws", Browser: BrowserChrome, ExplicitUserInstruction: true}
	decision, err := planner.Resolve(context.Background(), scope, override)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Route != browserpolicy.RouteExternal || decision.Start.Browser != BrowserChrome || decision.Start.ProfileClass != ProfileExternal || decision.Start.ProfileID != "user-chrome" || decision.Start.ConnectorID != "chrome-ws" {
		t.Fatalf("decision=%+v", decision)
	}
}
