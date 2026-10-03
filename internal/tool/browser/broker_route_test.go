package browser

import (
	"errors"
	"github.com/uvwt/agentdock/internal/browserpolicy"
	"path/filepath"
	"testing"
)

func requiredRouteFixture(t *testing.T) RouteRequest {
	t.Helper()
	return RouteRequest{
		Scope:      RequestScope{WorkspaceID: "workspace-1", CanonicalWorkspaceRoot: filepath.Clean(t.TempDir()), OwnerTaskID: "task-1", Provenance: ScopeACP},
		Policy:     browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "registered", RequiredProfileID: "user-profile"},
		Connectors: []ConnectorMetadata{{ID: "registered", Browser: BrowserEdge, ProfileID: "user-profile", ProfileClass: ProfileAuthenticatedExternal, Engine: EngineChromeDevToolsMCP, EngineVersion: PreferredEngineVersion, Transport: "websocket", Endpoint: "ws://127.0.0.1:9222/devtools/browser", Registered: true, Healthy: true, Verified: true, Authenticated: true, Capabilities: []ConnectorCapability{CapabilityBackgroundPage, CapabilityNoFocus, CapabilityLeaseTarget, CapabilitySafeRelease}, Ownership: ResourceOwnership{Process: OwnerExternalPersistent, Profile: OwnerExternalPersistent, Connector: OwnerAgentDockIsolated}}},
	}
}

func TestResolveRoute(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*RouteRequest)
		code    string
		route   browserpolicy.RouteKind
		browser Kind
	}{
		{name: "required authenticated external", route: browserpolicy.RouteRequiredExternal},
		{name: "generic required authenticated Chrome", mutate: func(r *RouteRequest) {
			r.Policy = browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceDefault, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "registered", RequiredProfileID: "user-profile"}
			r.Connectors[0].Browser = BrowserChrome
		}, route: browserpolicy.RouteRequiredExternal, browser: BrowserChrome},
		{name: "missing connector", mutate: func(r *RouteRequest) { r.Connectors = nil }, code: ErrRequiredRouteUnavailable},
		{name: "unregistered", mutate: func(r *RouteRequest) { r.Connectors[0].Registered = false }, code: ErrRequiredRouteUnavailable},
		{name: "unhealthy", mutate: func(r *RouteRequest) { r.Connectors[0].Healthy = false }, code: ErrRequiredRouteUnavailable},
		{name: "unverified", mutate: func(r *RouteRequest) { r.Connectors[0].Verified = false }, code: ErrRequiredRouteUnavailable},
		{name: "unauthenticated", mutate: func(r *RouteRequest) { r.Connectors[0].Authenticated = false }, code: ErrRequiredRouteUnavailable},
		{name: "wrong engine", mutate: func(r *RouteRequest) { r.Connectors[0].Engine = EngineNativeCDP }, code: ErrRequiredRouteUnavailable},
		{name: "wrong version", mutate: func(r *RouteRequest) { r.Connectors[0].EngineVersion = "1.8.0" }, code: ErrRequiredRouteUnavailable},
		{name: "wrong transport", mutate: func(r *RouteRequest) { r.Connectors[0].Transport = "stdio" }, code: ErrRequiredRouteUnavailable},
		{name: "wrong browser", mutate: func(r *RouteRequest) { r.Connectors[0].Browser = BrowserChrome }, code: ErrRequiredRouteUnavailable},
		{name: "wrong profile", mutate: func(r *RouteRequest) { r.Connectors[0].ProfileID = "other" }, code: ErrPolicyConflict},
		{name: "unsafe process ownership", mutate: func(r *RouteRequest) { r.Connectors[0].Ownership.Process = OwnerAgentDockIsolated }, code: ErrRequiredRouteUnavailable},
		{name: "unsafe profile ownership", mutate: func(r *RouteRequest) { r.Connectors[0].Ownership.Profile = OwnerAgentDockIsolated }, code: ErrRequiredRouteUnavailable},
		{name: "duplicate connector", mutate: func(r *RouteRequest) { r.Connectors = append(r.Connectors, r.Connectors[0]) }, code: ErrPolicyConflict},
		{name: "company managed conflict", mutate: func(r *RouteRequest) { r.Policy.Route = browserpolicy.RouteManaged }, code: ErrPolicyConflict},
		{name: "missing required identity", mutate: func(r *RouteRequest) { r.Policy.RequiredProfileID = "" }, code: ErrPolicyConflict},
		{name: "unknown class", mutate: func(r *RouteRequest) { r.Policy.Class = "unknown" }, code: ErrPolicyConflict},
		{name: "missing scope", mutate: func(r *RouteRequest) { r.Scope = RequestScope{} }, code: ErrScopeRequired},
		{name: "untrusted scope", mutate: func(r *RouteRequest) { r.Scope.Provenance = "tool-input" }, code: ErrScopeRequired},
		{name: "missing owner", mutate: func(r *RouteRequest) { r.Scope.OwnerTaskID = "" }, code: ErrScopeRequired},
		{name: "managed override cannot bypass required", mutate: func(r *RouteRequest) {
			r.Override = &RouteOverride{Route: browserpolicy.RouteManaged, ExplicitUserInstruction: true}
		}, code: ErrPolicyConflict},
		{name: "different connector override cannot bypass required", mutate: func(r *RouteRequest) {
			r.Override = &RouteOverride{Route: browserpolicy.RouteExternal, ConnectorID: "other", ExplicitUserInstruction: true}
		}, code: ErrPolicyConflict},
		{name: "matching override", mutate: func(r *RouteRequest) {
			r.Override = &RouteOverride{Route: browserpolicy.RouteExternal, ConnectorID: "registered", ExplicitUserInstruction: true}
		}, route: browserpolicy.RouteRequiredExternal},
		{name: "default ignores available external", mutate: func(r *RouteRequest) {
			r.Policy = browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceDefault, Route: browserpolicy.RouteManaged}
		}, route: browserpolicy.RouteManaged},
		{name: "explicit permitted", mutate: func(r *RouteRequest) {
			r.Policy = browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceDefault, Route: browserpolicy.RouteManaged, AllowExplicitExternal: true}
			r.Override = &RouteOverride{Route: browserpolicy.RouteExternal, ConnectorID: "registered", ExplicitUserInstruction: true}
		}, route: browserpolicy.RouteExternal},
		{name: "explicit external persistent need not be authenticated", mutate: func(r *RouteRequest) {
			r.Policy = browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceDefault, Route: browserpolicy.RouteManaged, AllowExplicitExternal: true}
			r.Override = &RouteOverride{Route: browserpolicy.RouteExternal, ConnectorID: "registered", ExplicitUserInstruction: true}
			r.Connectors[0].ProfileClass = ProfileExternal
			r.Connectors[0].Authenticated = false
		}, route: browserpolicy.RouteExternal},
		{name: "explicit denied by policy", mutate: func(r *RouteRequest) {
			r.Policy = browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceDefault, Route: browserpolicy.RouteManaged}
			r.Override = &RouteOverride{Route: browserpolicy.RouteExternal, ConnectorID: "registered", ExplicitUserInstruction: true}
		}, code: ErrPolicyConflict},
		{name: "model selection is not user provenance", mutate: func(r *RouteRequest) {
			r.Override = &RouteOverride{Route: browserpolicy.RouteExternal, ConnectorID: "registered"}
		}, code: ErrPolicyConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := requiredRouteFixture(t)
			if tc.mutate != nil {
				tc.mutate(&r)
			}
			got, err := ResolveRoute(r)
			if tc.code != "" {
				var be *Error
				if !errors.As(err, &be) || be.Code != tc.code || got != (RouteDecision{}) {
					t.Fatalf("decision=%+v error=%v", got, err)
				}
				return
			}
			if err != nil || got.Route != tc.route || got.Start.ForegroundPolicy != ForegroundForbidden || !got.Start.BackgroundPage {
				t.Fatalf("decision=%+v error=%v", got, err)
			}
			if tc.route == browserpolicy.RouteManaged {
				if got.Start.Browser != BrowserChrome || !got.Start.Headless || got.Start.ProfileClass != ProfileIsolated || got.Start.Engine != EngineChromeDevToolsMCP || got.Start.EngineVersion != PreferredEngineVersion {
					t.Fatalf("default route=%+v", got)
				}
			} else {
				wantBrowser := tc.browser
				if wantBrowser == "" {
					wantBrowser = BrowserEdge
				}
				if got.Start.Browser != wantBrowser || got.Start.Ownership.Process != OwnerExternalPersistent || got.Start.Ownership.Profile != OwnerExternalPersistent || got.Start.LifecyclePolicy != LifecycleExternal {
					t.Fatalf("external route=%+v", got)
				}
			}
		})
	}
	for _, capability := range []ConnectorCapability{CapabilityBackgroundPage, CapabilityNoFocus, CapabilityLeaseTarget, CapabilitySafeRelease} {
		t.Run("missing-"+string(capability), func(t *testing.T) {
			r := requiredRouteFixture(t)
			r.Connectors[0].Capabilities = nil
			for _, c := range []ConnectorCapability{CapabilityBackgroundPage, CapabilityNoFocus, CapabilityLeaseTarget, CapabilitySafeRelease} {
				if c != capability {
					r.Connectors[0].Capabilities = append(r.Connectors[0].Capabilities, c)
				}
			}
			got, err := ResolveRoute(r)
			var be *Error
			if !errors.As(err, &be) || be.Code != ErrRequiredRouteUnavailable || got != (RouteDecision{}) || len(be.Details.RequiredCapabilities) != 1 || be.Details.RequiredCapabilities[0] != capability {
				t.Fatalf("decision=%+v error=%v", got, err)
			}
		})
	}
}
