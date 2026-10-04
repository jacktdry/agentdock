package browser

import (
	"context"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

// ManagedIsolatedChrome is logical intent, never a directory or worker identity.
const ManagedIsolatedChrome = "managed-isolated-chrome"

type ProfilePlan struct {
	Scope                RequestScope
	Route                browserpolicy.RouteKind
	ProfileTemplate      string
	ProfileID            string
	ConnectorID          string
	Browser              Kind
	ProfileClass         ProfileClass
	Engine               EngineKind
	EngineVersion        string
	Endpoint             string
	Headless             bool
	BackgroundPage       bool
	ForegroundPolicy     ForegroundPolicy
	Ownership            ResourceOwnership
	RequiredCapabilities []ConnectorCapability
}

// ConnectorRuntimeStatus contains observations only, never configured identity
// or ownership. No production status provider is implemented in this step.
type ConnectorRuntimeStatus struct {
	Healthy       bool
	Verified      bool
	Authenticated bool
	Engine        EngineKind
	EngineVersion string
	Transport     string
	Capabilities  []ConnectorCapability
}

type ConnectorStatusProvider interface {
	ConnectorStatus(context.Context, string) (ConnectorRuntimeStatus, error)
}

type RoutePlanner struct {
	policies []browserpolicy.WorkspaceRootPolicy
	catalog  browserpolicy.Catalog
	status   ConnectorStatusProvider
}

func NewRoutePlanner(policies []browserpolicy.WorkspaceRootPolicy, catalog browserpolicy.Catalog, status ConnectorStatusProvider) (*RoutePlanner, error) {
	normalized, err := browserpolicy.NormalizeWorkspacePolicies(policies)
	if err != nil {
		return nil, err
	}
	if err := browserpolicy.ValidateWorkspacePolicies(normalized, catalog); err != nil {
		return nil, err
	}
	return &RoutePlanner{policies: normalized, catalog: catalog, status: status}, nil
}

func requiredRouteCapabilities() []ConnectorCapability {
	return []ConnectorCapability{CapabilityBackgroundPage, CapabilityNoFocus, CapabilityLeaseTarget, CapabilitySafeRelease}
}

func (p *RoutePlanner) Plan(scope RequestScope, override *RouteOverride) (ProfilePlan, error) {
	plan, _, err := p.plan(scope, override)
	return plan, err
}

func (p *RoutePlanner) plan(scope RequestScope, override *RouteOverride) (ProfilePlan, browserpolicy.BrowserRoutePolicy, error) {
	root, policy, err := browserpolicy.ClassifyWorkspace(scope.CanonicalWorkspaceRoot, p.policies)
	if err != nil {
		return ProfilePlan{}, policy, browserError(ErrScopeRequired, "browser route rejected", "routing", &ErrorDetails{WorkspaceID: scope.WorkspaceID, Reason: "workspace canonicalization failed"}, err)
	}
	scope.CanonicalWorkspaceRoot = root
	route, id, profileID, err := selectRoute(scope, policy, override)
	if err != nil {
		return ProfilePlan{}, policy, err
	}
	plan := ProfilePlan{Scope: scope, Route: route, Browser: BrowserChrome, ProfileTemplate: ManagedIsolatedChrome, ProfileClass: ProfileIsolated, Engine: EngineChromeDevToolsMCP, EngineVersion: PreferredEngineVersion, Headless: true, BackgroundPage: true, ForegroundPolicy: ForegroundForbidden, RequiredCapabilities: requiredRouteCapabilities(), Ownership: ResourceOwnership{Process: OwnerAgentDockIsolated, Profile: OwnerAgentDockIsolated, Connector: OwnerAgentDockIsolated}}
	if route == browserpolicy.RouteManaged {
		return plan, policy, nil
	}
	connector, ok := p.catalog.Connector(id)
	if !ok {
		return ProfilePlan{}, policy, routeUnavailable(scope, id, "catalog connector unavailable")
	}
	profile, ok := p.catalog.Profile(connector.ProfileID)
	if !ok {
		return ProfilePlan{}, policy, routeUnavailable(scope, id, "catalog profile unavailable")
	}
	if (profileID != "" && profile.ID != profileID) || (override != nil && override.Browser != "" && override.Browser != Kind(profile.Browser)) {
		return ProfilePlan{}, policy, browserError(ErrPolicyConflict, "browser route rejected", "routing", &ErrorDetails{WorkspaceID: scope.WorkspaceID, ConnectorID: id, Reason: "override conflicts with catalog identity"}, nil)
	}
	plan.ProfileTemplate = ""
	plan.ProfileID, plan.ConnectorID, plan.Endpoint = profile.ID, connector.ID, connector.Endpoint
	plan.Browser, plan.ProfileClass = Kind(profile.Browser), ProfileClass(profile.Class)
	plan.Headless = false
	plan.Ownership = ResourceOwnership{Process: OwnerExternalPersistent, Profile: OwnerExternalPersistent, Connector: OwnerAgentDockIsolated}
	return plan, policy, nil
}

func routeUnavailable(scope RequestScope, id, reason string) error {
	return browserError(ErrRequiredRouteUnavailable, "browser route rejected", "routing", &ErrorDetails{WorkspaceID: scope.WorkspaceID, ConnectorID: id, Reason: reason}, nil)
}

func (p *RoutePlanner) Resolve(ctx context.Context, scope RequestScope, override *RouteOverride) (RouteDecision, error) {
	plan, policy, err := p.plan(scope, override)
	if err != nil {
		return RouteDecision{}, err
	}
	request := RouteRequest{Scope: plan.Scope, Policy: policy, Override: override}
	if plan.Route == browserpolicy.RouteManaged {
		return ResolveRoute(request)
	}
	if p.status == nil {
		return RouteDecision{}, routeUnavailable(plan.Scope, plan.ConnectorID, "runtime verification unavailable")
	}
	status, err := p.status.ConnectorStatus(ctx, plan.ConnectorID)
	if err != nil {
		return RouteDecision{}, routeUnavailable(plan.Scope, plan.ConnectorID, "runtime connector verification unavailable")
	}
	request.Connectors = []ConnectorMetadata{{ID: plan.ConnectorID, Browser: plan.Browser, ProfileID: plan.ProfileID, ProfileClass: plan.ProfileClass, Endpoint: plan.Endpoint, Registered: true, Healthy: status.Healthy, Verified: status.Verified, Authenticated: status.Authenticated, Engine: status.Engine, EngineVersion: status.EngineVersion, Transport: status.Transport, Capabilities: status.Capabilities, Ownership: plan.Ownership}}
	return ResolveRoute(request)
}
