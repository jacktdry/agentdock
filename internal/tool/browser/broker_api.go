package browser

import "context"

// BrowserBroker is the future lease boundary. Scope must be supplied by a trusted
// caller for acquisition and every operation; implementations must verify lease
// ownership and resolve its target on each call. No implementation is wired yet.
type BrowserBroker interface {
	Acquire(ctx context.Context, route RouteRequest, request StartRequest) (AcquiredBrowser, error)
	Act(ctx context.Context, scope RequestScope, leaseID string, request ActRequest) (Snapshot, error)
	Snapshot(ctx context.Context, scope RequestScope, leaseID string, request SnapshotRequest) (Snapshot, error)
	Release(ctx context.Context, scope RequestScope, leaseID string) (CloseResult, error)
}

type AcquiredBrowser struct {
	Lease   LeaseMetadata
	Binding EngineBinding
	Start   StartResult
}

type EngineCapabilities struct {
	Kind         EngineKind
	Version      string
	Browsers     []Kind
	Profiles     []ProfileClass
	Headless     bool
	Capabilities []ConnectorCapability
}

// EngineBinding identifies a single lease's concrete targets, never a globally
// selected page. Existing request session/page IDs must agree with this binding.
type EngineBinding struct {
	BrowserLeaseID   string
	BrowserSessionID string
	WorkerID         string
	// BrowserContextID is a native context ID, never an isolatedContext name.
	BrowserContextID string
	// IsolationContextName is the MCP named context, never a native context ID.
	IsolationContextName string
	// PageID is opaque and engine-local. The MCP adapter must later parse/bind
	// numeric page IDs rather than interpreting this as a native CDP target ID.
	PageID      string
	ConnectorID string
	Engine      EngineKind
}

// BrowserEngine adapts operations against an explicit binding. Close releases
// only owned resources according to the resolved ownership/lifecycle policy.
type BrowserEngine interface {
	Capabilities() EngineCapabilities
	Open(ctx context.Context, scope RequestScope, resolved ResolvedStart, request StartRequest) (EngineBinding, StartResult, error)
	Act(ctx context.Context, scope RequestScope, binding EngineBinding, request ActRequest) (Snapshot, error)
	Snapshot(ctx context.Context, scope RequestScope, binding EngineBinding, request SnapshotRequest) (Snapshot, error)
	Close(ctx context.Context, scope RequestScope, binding EngineBinding) (CloseResult, error)
}

// EngineLifecycle shuts down workers, separately from releasing a page binding.
type EngineLifecycle interface{ Shutdown(context.Context) error }
