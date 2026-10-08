package browser

import (
	"github.com/uvwt/agentdock/internal/browserpolicy"
	"time"
)

type ScopeProvenance string

const (
	ScopeRuntime  ScopeProvenance = "runtime"
	ScopeUpstream ScopeProvenance = "trusted-upstream"
	ScopeACP      ScopeProvenance = "trusted-acp"
)

// RequestScope is constructed by the trusted caller boundary after root
// canonicalization/classification. It must never be decoded from browser tool input.
type RequestScope struct {
	WorkspaceID            string
	CanonicalWorkspaceRoot string
	OwnerTaskID            string
	OwnerSessionID         string
	OwnerACPSessionID      string
	Provenance             ScopeProvenance
}

type EngineKind string

const (
	EngineNativeCDP         EngineKind = "native-cdp"
	EngineChromeDevToolsMCP EngineKind = "chrome-devtools-mcp"
)
const PreferredEngineVersion = "1.7.0"

type ProfileClass string

const (
	ProfileIsolated              ProfileClass = "isolated-temporary"
	ProfileAuthenticatedExternal ProfileClass = "authenticated-external"
	ProfileExternal              ProfileClass = "external-persistent"
)

type ForegroundPolicy string

const ForegroundForbidden ForegroundPolicy = "forbidden"

type LifecyclePolicy string

const (
	LifecycleOwned    LifecyclePolicy = "close-owned-resources"
	LifecycleExternal LifecyclePolicy = "release-lease-preserve-browser-profile"
	LifecycleAdapter  LifecyclePolicy = "adapter-session-drain"
)

type CleanupState string

const (
	CleanupPending   CleanupState = "pending"
	CleanupReleasing CleanupState = "releasing"
	CleanupComplete  CleanupState = "complete"
	CleanupFailed    CleanupState = "failed"
)

type ResourceOwner string

const (
	OwnerExternalPersistent ResourceOwner = "external-persistent"
	OwnerAgentDockIsolated  ResourceOwner = "agentdock-isolated"
	OwnerAdapter            ResourceOwner = "adapter-owned"
)

// Ownership is independent for each resource; observed PIDs confer no authority.
type ResourceOwnership struct {
	Process   ResourceOwner
	Profile   ResourceOwner
	Connector ResourceOwner
}
type ConnectorCapability string

const (
	CapabilityBackgroundPage ConnectorCapability = "background-page"
	CapabilityNoFocus        ConnectorCapability = "no-focus-no-window-mutation"
	CapabilityLeaseTarget    ConnectorCapability = "per-lease-target"
	CapabilitySafeRelease    ConnectorCapability = "release-owned-target-only"
)

type ConnectorMetadata struct {
	ID            string
	Browser       Kind
	ProfileID     string
	ProfileClass  ProfileClass
	Engine        EngineKind
	EngineVersion string
	Transport     string
	Endpoint      string
	ProfilePath   string
	Registered    bool
	Healthy       bool
	Verified      bool
	Authenticated bool
	Capabilities  []ConnectorCapability
	Ownership     ResourceOwnership
}

// Override provenance means an explicit user instruction verified by the caller,
// not merely a model-selected connector or tool argument.
type RouteOverride struct {
	Route                   browserpolicy.RouteKind
	Browser                 Kind
	ConnectorID             string
	ProfileID               string
	ExplicitUserInstruction bool
}
type RouteRequest struct {
	Scope      RequestScope
	Policy     browserpolicy.BrowserRoutePolicy
	Override   *RouteOverride
	Connectors []ConnectorMetadata
}
type ResolvedStart struct {
	Browser          Kind
	Engine           EngineKind
	EngineVersion    string
	ConnectorID      string
	ProfileID        string
	ProfileClass     ProfileClass
	Endpoint         string
	ProfilePath      string
	Headless         bool
	BackgroundPage   bool
	ForegroundPolicy ForegroundPolicy
	LifecyclePolicy  LifecyclePolicy
	Ownership        ResourceOwnership
}
type RouteDecision struct {
	Scope RequestScope
	Route browserpolicy.RouteKind
	Start ResolvedStart
	// Core-only admission authority. Serialization cannot transfer this grant.
	grant *externalRouteGrant
}
type PIDObservation struct {
	PID           int
	Executable    string
	StartIdentity string
	ObservedAt    time.Time
}
type LeaseMetadata struct {
	BrowserSessionID string
	BrowserLeaseID   string
	OwnerType        ResourceOwner
	Scope            RequestScope
	// OwnerProfileID identifies the owner/ACP adapter profile, never the browser
	// profile. The browser profile identity is ResolvedStart.ProfileID.
	OwnerProfileID   string
	WorkerID         string
	BrowserContextID string
	// IsolationContextName is the MCP named context, never a native context ID.
	IsolationContextName string
	PageID               string
	// BrowserPID is observation only; external ownership never permits killing it.
	BrowserPID       PIDObservation
	ConnectorPID     PIDObservation
	CDPEndpoint      string
	ProfilePath      string
	ProfileClass     ProfileClass
	Ownership        ResourceOwnership
	ForegroundPolicy ForegroundPolicy
	LifecyclePolicy  LifecyclePolicy
	CreatedAt        time.Time
	LastActiveAt     time.Time
	ExpiresAt        time.Time
	CleanupState     CleanupState
	CleanupReason    string
	CleanupError     string
}
