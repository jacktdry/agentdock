package browser

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"time"

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

// ConnectorRuntimeStatus contains provider-observed identity and assertions,
// never ownership. Raw assertions cannot qualify a route. Only a package-private
// qualification from an independently verified source may authorize forwarding.
// There is no production attestor or qualification minting path in C2b-A.
type ConnectorRuntimeStatus struct {
	ConnectorID   string
	ProfileID     string
	Endpoint      string    // Exact canonical catalog endpoint; never renderer data.
	ObservedAt    time.Time // UTC observation time.
	ExpiresAt     time.Time // UTC expiration, at most five seconds after observation.
	Healthy       bool
	Verified      bool
	Authenticated bool
	Engine        EngineKind
	EngineVersion string
	Transport     string
	Capabilities  []ConnectorCapability

	// Core-only opaque identities; never serialized or projected to Desktop.
	source        *connectorEvidenceIdentity
	incarnation   *connectorEvidenceIdentity
	qualification *connectorQualification
}

// Nonzero-sized identities have distinct pointer identity. A future reviewed
// attestor must derive these from its own source and verified runtime incarnation,
// never configuration, raw status, reachability or a caller-supplied identifier.
type connectorEvidenceIdentity struct{ marker byte }

// Immutable after minting, except used. The pointer itself is a single-use nonce:
// status copies share consumption, including across planners. No production code
// constructs this authority; only offline _test.go fixtures currently mint it.
type connectorQualification struct {
	format      uint8
	observation ConnectorRuntimeStatus // Detached snapshot, qualification nil.
	used        atomic.Bool
}

func connectorObservationFresh(status ConnectorRuntimeStatus, now time.Time) bool {
	_, observedOffset := status.ObservedAt.Zone()
	_, expiresOffset := status.ExpiresAt.Zone()
	return !status.ObservedAt.IsZero() && !status.ExpiresAt.IsZero() && observedOffset == 0 && expiresOffset == 0 &&
		!status.ObservedAt.After(now) && now.Sub(status.ObservedAt) <= 5*time.Second &&
		status.ExpiresAt.After(now) && status.ExpiresAt.After(status.ObservedAt) &&
		status.ExpiresAt.Sub(status.ObservedAt) <= 5*time.Second
}

func consumeConnectorQualification(status ConnectorRuntimeStatus, now time.Time) bool {
	q := status.qualification
	if q == nil || q.format != 1 {
		return false
	}
	e := q.observation
	if !connectorObservationFresh(e, now) || e.source == nil || e.incarnation == nil || e.source == e.incarnation ||
		status.source != e.source || status.incarnation != e.incarnation ||
		status.ConnectorID != e.ConnectorID || status.ProfileID != e.ProfileID || status.Endpoint != e.Endpoint ||
		!status.ObservedAt.Equal(e.ObservedAt) || !status.ExpiresAt.Equal(e.ExpiresAt) ||
		status.Healthy != e.Healthy || status.Verified != e.Verified || status.Authenticated != e.Authenticated ||
		status.Engine != e.Engine || status.EngineVersion != e.EngineVersion || status.Transport != e.Transport ||
		!slices.Equal(status.Capabilities, e.Capabilities) {
		return false
	}
	return q.used.CompareAndSwap(false, true)
}

type ConnectorStatusProvider interface {
	// Returned slices must not be mutated concurrently with return or the
	// planner's immediate copy. That provider contract cannot be enforced here.
	ConnectorStatus(context.Context, string) (ConnectorRuntimeStatus, error)
}

func copyConnectorRuntimeStatus(status ConnectorRuntimeStatus) ConnectorRuntimeStatus {
	status.Capabilities = append([]ConnectorCapability(nil), status.Capabilities...)
	return status
}

type RoutePlanner struct {
	policies []browserpolicy.WorkspaceRootPolicy
	catalog  browserpolicy.Catalog
	status   ConnectorStatusProvider
	// Held until the provider actually returns, even after the caller times out.
	statusInFlight chan struct{}
}

func NewRoutePlanner(policies []browserpolicy.WorkspaceRootPolicy, catalog browserpolicy.Catalog, status ConnectorStatusProvider) (*RoutePlanner, error) {
	normalized, err := browserpolicy.NormalizeWorkspacePolicies(policies)
	if err != nil {
		return nil, err
	}
	if err := browserpolicy.ValidateWorkspacePolicies(normalized, catalog); err != nil {
		return nil, err
	}
	return &RoutePlanner{policies: normalized, catalog: catalog, status: status, statusInFlight: make(chan struct{}, 1)}, nil
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
	if ctx.Err() != nil {
		return RouteDecision{}, routeUnavailable(plan.Scope, plan.ConnectorID, "runtime verification canceled")
	}
	if p.status == nil {
		return RouteDecision{}, routeUnavailable(plan.Scope, plan.ConnectorID, "runtime verification unavailable")
	}
	// Bound the caller's wait independently of provider cooperation. A stuck
	// provider retains this planner's only slot; never spawn another behind it.
	statusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	select {
	case p.statusInFlight <- struct{}{}:
	case <-statusCtx.Done():
		return RouteDecision{}, routeUnavailable(plan.Scope, plan.ConnectorID, "runtime verification canceled or timed out")
	}
	if ctx.Err() != nil || statusCtx.Err() != nil {
		<-p.statusInFlight
		return RouteDecision{}, routeUnavailable(plan.Scope, plan.ConnectorID, "runtime verification canceled or timed out")
	}
	type statusResult struct {
		status ConnectorRuntimeStatus
		err    error
	}
	results := make(chan statusResult, 1)
	go func() {
		defer func() { <-p.statusInFlight }()
		completed := false
		var result statusResult
		defer func() {
			if !completed {
				// Completion flag also handles panic(nil) with legacy panicnil.
				// Never retain or expose the panic value or stack.
				_ = recover()
				result = statusResult{err: errors.New("runtime connector verification unavailable")}
			}
			// Buffered so a timed-out consumer cannot prevent slot release.
			results <- result
		}()
		status, err := p.status.ConnectorStatus(statusCtx, plan.ConnectorID)
		status = copyConnectorRuntimeStatus(status)
		result = statusResult{status: status, err: err}
		completed = true
	}()
	var result statusResult
	select {
	case result = <-results:
	case <-statusCtx.Done():
		return RouteDecision{}, routeUnavailable(plan.Scope, plan.ConnectorID, "runtime verification canceled or timed out")
	}
	if ctx.Err() != nil || statusCtx.Err() != nil {
		return RouteDecision{}, routeUnavailable(plan.Scope, plan.ConnectorID, "runtime verification canceled or timed out")
	}
	if result.err != nil {
		return RouteDecision{}, routeUnavailable(plan.Scope, plan.ConnectorID, "runtime connector verification unavailable")
	}
	status := result.status
	if status.ConnectorID != plan.ConnectorID || status.ProfileID != plan.ProfileID || status.Endpoint != plan.Endpoint {
		return RouteDecision{}, routeUnavailable(plan.Scope, plan.ConnectorID, "runtime connector identity mismatch")
	}
	now := time.Now().UTC()
	if !connectorObservationFresh(status, now) {
		return RouteDecision{}, routeUnavailable(plan.Scope, plan.ConnectorID, "runtime connector observation stale or invalid")
	}
	// Identity/time consistency is necessary but cannot attest profile/auth or
	// no-focus/lease/release capabilities. Check the sealed observation before
	// forwarding any provider assertions to the trusted pure resolver.
	if !consumeConnectorQualification(status, now) {
		return RouteDecision{}, routeUnavailable(plan.Scope, plan.ConnectorID, "runtime connector qualification unavailable")
	}
	request.Connectors = []ConnectorMetadata{{ID: plan.ConnectorID, Browser: plan.Browser, ProfileID: plan.ProfileID, ProfileClass: plan.ProfileClass, Endpoint: plan.Endpoint, Registered: true, Healthy: status.Healthy, Verified: status.Verified, Authenticated: status.Authenticated, Engine: status.Engine, EngineVersion: status.EngineVersion, Transport: status.Transport, Capabilities: status.Capabilities, Ownership: plan.Ownership}}
	return ResolveRoute(request)
}
