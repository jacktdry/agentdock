package browser

import (
	"github.com/uvwt/agentdock/internal/browserpolicy"
	"path/filepath"
	"strings"
)

// ResolveRoute is pure: no discovery, health probes, launches or filesystem IO.
// Production callers must use RoutePlanner, which derives policy and identities.
// This low-level helper accepts preassembled trusted data for contract testing.
func ResolveRoute(r RouteRequest) (RouteDecision, error) {
	fail := func(code, reason, id string) (RouteDecision, error) {
		return RouteDecision{}, browserError(code, "browser route rejected", "routing", &ErrorDetails{WorkspaceID: r.Scope.WorkspaceID, ConnectorID: id, Reason: reason}, nil)
	}
	s := r.Scope
	route, id, profile, err := selectRoute(s, r.Policy, r.Override)
	if err != nil {
		return RouteDecision{}, err
	}
	d := RouteDecision{Scope: s, Route: route}
	d.Start = ResolvedStart{Browser: BrowserChrome, Engine: EngineChromeDevToolsMCP, EngineVersion: PreferredEngineVersion, ProfileClass: ProfileIsolated, Headless: true, BackgroundPage: true, ForegroundPolicy: ForegroundForbidden, LifecyclePolicy: LifecycleOwned, Ownership: ResourceOwnership{Process: OwnerAgentDockIsolated, Profile: OwnerAgentDockIsolated, Connector: OwnerAgentDockIsolated}}
	if route == browserpolicy.RouteManaged {
		return d, nil
	}
	var c *ConnectorMetadata
	for i := range r.Connectors {
		if r.Connectors[i].ID == id {
			if c != nil {
				return fail(ErrPolicyConflict, "duplicate connector registration", id)
			}
			c = &r.Connectors[i]
		}
	}
	if c == nil || !c.Registered || !c.Healthy || !c.Verified || c.Endpoint == "" {
		return fail(ErrRequiredRouteUnavailable, "connector missing, unhealthy or unverified", id)
	}
	if (c.Browser != BrowserChrome && c.Browser != BrowserChromium && c.Browser != BrowserEdge) || c.Engine != EngineChromeDevToolsMCP || c.EngineVersion != PreferredEngineVersion || c.Transport != "websocket" || c.ProfileID == "" {
		return fail(ErrRequiredRouteUnavailable, "connector identity incomplete", id)
	}
	if profile != "" && c.ProfileID != profile {
		return fail(ErrPolicyConflict, "connector profile mismatch", id)
	}
	if o := r.Override; o != nil && o.Browser != "" && o.Browser != c.Browser {
		return fail(ErrPolicyConflict, "connector browser mismatch", id)
	}
	if route == browserpolicy.RouteRequiredExternal && c.ProfileClass != ProfileAuthenticatedExternal {
		return fail(ErrRequiredRouteUnavailable, "authenticated required profile unavailable", id)
	}
	if r.Policy.Class == browserpolicy.WorkspaceCompany && c.Browser != BrowserEdge {
		return fail(ErrRequiredRouteUnavailable, "company required browser unavailable", id)
	}
	if c.ProfileClass == ProfileAuthenticatedExternal && !c.Authenticated {
		return fail(ErrRequiredRouteUnavailable, "authenticated external profile unavailable", id)
	}
	if (c.ProfileClass != ProfileExternal && c.ProfileClass != ProfileAuthenticatedExternal) || c.Ownership.Process != OwnerExternalPersistent || c.Ownership.Profile != OwnerExternalPersistent || (c.Ownership.Connector != OwnerAgentDockIsolated && c.Ownership.Connector != OwnerAdapter) {
		return fail(ErrRequiredRouteUnavailable, "external ownership not verified", id)
	}
	for _, want := range requiredRouteCapabilities() {
		found := false
		for _, got := range c.Capabilities {
			if want == got {
				found = true
			}
		}
		if !found {
			return RouteDecision{}, browserError(ErrRequiredRouteUnavailable, "browser route rejected", "routing", &ErrorDetails{WorkspaceID: s.WorkspaceID, ConnectorID: id, Reason: "required capability unavailable", RequiredCapabilities: []ConnectorCapability{want}}, nil)
		}
	}
	d.Start = ResolvedStart{Browser: c.Browser, Engine: c.Engine, EngineVersion: c.EngineVersion, ConnectorID: id, ProfileID: c.ProfileID, ProfileClass: c.ProfileClass, Endpoint: c.Endpoint, ProfilePath: c.ProfilePath, BackgroundPage: true, ForegroundPolicy: ForegroundForbidden, LifecyclePolicy: LifecycleExternal, Ownership: c.Ownership}
	return d, nil
}

// selectRoute is shared by the planner and the pure low-level resolver.
func selectRoute(s RequestScope, policy browserpolicy.BrowserRoutePolicy, override *RouteOverride) (browserpolicy.RouteKind, string, string, error) {
	reject := func(code, reason, id string) (browserpolicy.RouteKind, string, string, error) {
		return "", "", "", browserError(code, "browser route rejected", "routing", &ErrorDetails{WorkspaceID: s.WorkspaceID, ConnectorID: id, Reason: reason}, nil)
	}
	if strings.TrimSpace(s.WorkspaceID) == "" || !filepath.IsAbs(s.CanonicalWorkspaceRoot) || filepath.Clean(s.CanonicalWorkspaceRoot) != s.CanonicalWorkspaceRoot ||
		(s.Provenance != ScopeRuntime && s.Provenance != ScopeUpstream && s.Provenance != ScopeACP) ||
		(s.OwnerTaskID == "" && s.OwnerSessionID == "" && s.OwnerACPSessionID == "") {
		return reject(ErrScopeRequired, "trusted workspace and owner scope required", "")
	}
	if err := browserpolicy.ValidateRoutePolicy(policy); err != nil {
		return reject(ErrPolicyConflict, err.Error(), "")
	}
	route, id, profile := policy.Route, policy.RequiredConnectorID, policy.RequiredProfileID
	if o := override; o != nil {
		if !o.ExplicitUserInstruction {
			return reject(ErrPolicyConflict, "override lacks trusted explicit user provenance", o.ConnectorID)
		}
		if route == browserpolicy.RouteRequiredExternal {
			if o.Route != browserpolicy.RouteExternal || o.ConnectorID != id || (o.ProfileID != "" && o.ProfileID != profile) {
				return reject(ErrPolicyConflict, "override conflicts with required authenticated route", id)
			}
		} else {
			if o.Route != browserpolicy.RouteExternal || !policy.AllowExplicitExternal || o.ConnectorID == "" {
				return reject(ErrPolicyConflict, "explicit external route not permitted", o.ConnectorID)
			}
			route, id, profile = browserpolicy.RouteExternal, o.ConnectorID, o.ProfileID
		}
	}
	return route, id, profile, nil
}
