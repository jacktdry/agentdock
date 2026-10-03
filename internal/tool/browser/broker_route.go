package browser

import (
	"github.com/uvwt/agentdock/internal/browserpolicy"
	"path/filepath"
	"strings"
)

// ResolveRoute is pure: no discovery, health probes, launches or filesystem IO.
// Scope, policy and connector verification are supplied by trusted boundaries.
func ResolveRoute(r RouteRequest) (RouteDecision, error) {
	fail := func(code, reason, id string) (RouteDecision, error) {
		return RouteDecision{}, browserError(code, "browser route rejected", "routing", &ErrorDetails{WorkspaceID: r.Scope.WorkspaceID, ConnectorID: id, Reason: reason}, nil)
	}
	s := r.Scope
	if strings.TrimSpace(s.WorkspaceID) == "" || !filepath.IsAbs(s.CanonicalWorkspaceRoot) || filepath.Clean(s.CanonicalWorkspaceRoot) != s.CanonicalWorkspaceRoot ||
		(s.Provenance != ScopeRuntime && s.Provenance != ScopeUpstream && s.Provenance != ScopeACP) ||
		(s.OwnerTaskID == "" && s.OwnerSessionID == "" && s.OwnerACPSessionID == "") {
		return fail(ErrScopeRequired, "trusted workspace and owner scope required", "")
	}
	if err := browserpolicy.ValidateRoutePolicy(r.Policy); err != nil {
		return fail(ErrPolicyConflict, err.Error(), "")
	}
	route, id, profile := r.Policy.Route, r.Policy.RequiredConnectorID, r.Policy.RequiredProfileID
	if o := r.Override; o != nil {
		if !o.ExplicitUserInstruction {
			return fail(ErrPolicyConflict, "override lacks trusted explicit user provenance", o.ConnectorID)
		}
		if route == browserpolicy.RouteRequiredExternal {
			if o.Route != browserpolicy.RouteExternal || o.ConnectorID != id || (o.ProfileID != "" && o.ProfileID != profile) || (o.Browser != "" && o.Browser != BrowserEdge) {
				return fail(ErrPolicyConflict, "override conflicts with required authenticated route", id)
			}
		} else {
			if o.Route != browserpolicy.RouteExternal || !r.Policy.AllowExplicitExternal || o.ConnectorID == "" {
				return fail(ErrPolicyConflict, "explicit external route not permitted", o.ConnectorID)
			}
			route, id, profile = browserpolicy.RouteExternal, o.ConnectorID, o.ProfileID
		}
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
	if (c.Browser != BrowserChrome && c.Browser != BrowserChromium && c.Browser != BrowserEdge) || (c.Engine != EngineNativeCDP && c.Engine != EngineChromeDevToolsMCP) || c.EngineVersion == "" || c.Transport == "" || c.ProfileID == "" {
		return fail(ErrRequiredRouteUnavailable, "connector identity incomplete", id)
	}
	if profile != "" && c.ProfileID != profile {
		return fail(ErrPolicyConflict, "connector profile mismatch", id)
	}
	if o := r.Override; o != nil && o.Browser != "" && o.Browser != c.Browser {
		return fail(ErrPolicyConflict, "connector browser mismatch", id)
	}
	if route == browserpolicy.RouteRequiredExternal && (c.Browser != BrowserEdge || c.ProfileClass != ProfileAuthenticatedExternal || !c.Authenticated) {
		return fail(ErrRequiredRouteUnavailable, "authenticated required browser/profile unavailable", id)
	}
	if (c.ProfileClass != ProfileExternal && c.ProfileClass != ProfileAuthenticatedExternal) || c.Ownership.Process != OwnerExternalPersistent || c.Ownership.Profile != OwnerExternalPersistent || (c.Ownership.Connector != OwnerAgentDockIsolated && c.Ownership.Connector != OwnerAdapter) {
		return fail(ErrRequiredRouteUnavailable, "external ownership not verified", id)
	}
	required := []ConnectorCapability{CapabilityBackgroundPage, CapabilityNoFocus, CapabilityLeaseTarget, CapabilitySafeRelease}
	for _, want := range required {
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
