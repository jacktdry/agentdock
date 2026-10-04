package browser

import (
	"sort"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

type BrokerOwnerDiagnostic struct {
	ACPSessionID           string   `json:"owner_acp_session_id"`
	ProfileID              string   `json:"owner_profile_id"`
	CanonicalWorkspaceRoot string   `json:"canonical_workspace_root"`
	LeaseIDs               []string `json:"lease_ids"`
}

type BrokerOwnershipDiagnostic struct {
	Process   ResourceOwner `json:"process"`
	Profile   ResourceOwner `json:"profile"`
	Connector ResourceOwner `json:"connector"`
}

type BrokerWorkerDiagnostic struct {
	WorkerID      string      `json:"worker_id"`
	CreatedAt     time.Time   `json:"created_at"`
	State         WorkerState `json:"state"`
	PID           int         `json:"pid,omitempty"`
	ServerName    string      `json:"server_name,omitempty"`
	ServerVersion string      `json:"server_version,omitempty"`
	PageIDRouting bool        `json:"page_id_routing"`
	Error         string      `json:"error,omitempty"`
}

type BrokerLeaseDiagnostic struct {
	LeaseID              string                    `json:"lease_id"`
	ACPSessionID         string                    `json:"owner_acp_session_id,omitempty"`
	ProfileID            string                    `json:"owner_profile_id,omitempty"`
	Route                browserpolicy.RouteKind   `json:"route"`
	WorkspaceID          string                    `json:"workspace_id"`
	WorkerID             string                    `json:"worker_id"`
	PageID               string                    `json:"page_id"`
	IsolationContextName string                    `json:"isolation_context_name,omitempty"`
	ConnectorID          string                    `json:"connector_id,omitempty"`
	ConnectorPID         int                       `json:"connector_pid,omitempty"`
	OwnerType            ResourceOwner             `json:"owner_type"`
	Ownership            BrokerOwnershipDiagnostic `json:"ownership"`
	ForegroundPolicy     ForegroundPolicy          `json:"foreground_policy"`
	CleanupState         CleanupState              `json:"cleanup_state"`
	CleanupReason        string                    `json:"cleanup_reason,omitempty"`
	CleanupError         string                    `json:"cleanup_error,omitempty"`
	CreatedAt            time.Time                 `json:"created_at"`
	LastActiveAt         time.Time                 `json:"last_active_at"`
	ExpiresAt            time.Time                 `json:"expires_at,omitempty"`
}

type BrokerQueueDiagnostic struct {
	Active          int `json:"active"`
	Queued          int `json:"queued"`
	MaxConcurrency  int `json:"max_concurrency"`
	QueueCapacity   int `json:"queue_capacity"`
	ManagedOrphans  int `json:"managed_orphans"`
	ExternalOrphans int `json:"external_orphans"`
}

type BrokerDiagnostics struct {
	Owners             []BrokerOwnerDiagnostic  `json:"owners"`
	Leases             []BrokerLeaseDiagnostic  `json:"leases"`
	Workers            []BrokerWorkerDiagnostic `json:"workers"`
	Queue              BrokerQueueDiagnostic    `json:"queue"`
	LifecycleLastError string                   `json:"lifecycle_last_error,omitempty"`
}

func (r *WorkerRegistry) Snapshot() []BrokerWorkerDiagnostic {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	workers := make([]*managedWorker, 0, len(r.workers))
	for _, w := range r.workers {
		workers = append(workers, w)
	}
	r.mu.Unlock()
	out := make([]BrokerWorkerDiagnostic, 0, len(workers))
	for _, w := range workers {
		w.mu.Lock()
		info := w.info
		w.mu.Unlock()
		out = append(out, BrokerWorkerDiagnostic{
			WorkerID: info.WorkerID, CreatedAt: info.CreatedAt, State: info.State,
			PID: info.Session.PID, ServerName: info.Session.ServerName, ServerVersion: info.Session.ServerVersion,
			PageIDRouting: info.Compatibility.PageIDRouting, Error: info.Error,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (b *ACPBridge) Diagnostics() BrokerDiagnostics {
	if b == nil {
		return BrokerDiagnostics{}
	}
	b.mu.Lock()
	owners := make([]BrokerOwnerDiagnostic, 0, len(b.bySession))
	leaseOwners := make(map[string]BrokerOwnerDiagnostic)
	leaseRoutes := make(map[string]browserpolicy.RouteKind)
	for _, owner := range b.bySession {
		d := BrokerOwnerDiagnostic{ACPSessionID: owner.sessionID, ProfileID: owner.profileID, CanonicalWorkspaceRoot: owner.scope.CanonicalWorkspaceRoot}
		for id, entry := range owner.leases {
			d.LeaseIDs = append(d.LeaseIDs, id)
			leaseOwners[id] = d
			leaseRoutes[id] = entry.route
		}
		sort.Strings(d.LeaseIDs)
		owners = append(owners, d)
	}
	b.mu.Unlock()
	sort.Slice(owners, func(i, j int) bool { return owners[i].ACPSessionID < owners[j].ACPSessionID })

	var leases []BrokerLeaseDiagnostic
	b.managed.manager.mu.Lock()
	managed := make(map[string]*managedLease, len(b.managed.manager.leases))
	for id, l := range b.managed.manager.leases {
		managed[id] = l
	}
	b.managed.manager.mu.Unlock()
	for id, l := range managed {
		l.mu.Lock()
		m := l.metadata
		bind := l.binding
		l.mu.Unlock()
		o := leaseOwners[id]
		leases = append(leases, BrokerLeaseDiagnostic{LeaseID: id, ACPSessionID: o.ACPSessionID, ProfileID: o.ProfileID, Route: func() browserpolicy.RouteKind {
			r := leaseRoutes[id]
			if r == "" {
				return browserpolicy.RouteManaged
			}
			return r
		}(), WorkspaceID: m.Scope.WorkspaceID, WorkerID: m.WorkerID, PageID: m.PageID, IsolationContextName: m.IsolationContextName, ConnectorPID: m.ConnectorPID.PID, OwnerType: m.OwnerType, Ownership: BrokerOwnershipDiagnostic{Process: m.Ownership.Process, Profile: m.Ownership.Profile, Connector: m.Ownership.Connector}, ForegroundPolicy: m.ForegroundPolicy, CleanupState: m.CleanupState, CleanupReason: m.CleanupReason, CleanupError: m.CleanupError, CreatedAt: m.CreatedAt, LastActiveAt: m.LastActiveAt, ExpiresAt: m.ExpiresAt, ConnectorID: bind.ConnectorID})
	}
	b.external.mu.Lock()
	external := make(map[string]*externalLease, len(b.external.leases))
	for id, l := range b.external.leases {
		external[id] = l
	}
	externalOrphans := len(b.external.orphans)
	b.external.mu.Unlock()
	for id, l := range external {
		l.mu.Lock()
		m := l.metadata
		bind := l.binding
		l.mu.Unlock()
		o := leaseOwners[id]
		leases = append(leases, BrokerLeaseDiagnostic{LeaseID: id, ACPSessionID: o.ACPSessionID, ProfileID: o.ProfileID, Route: func() browserpolicy.RouteKind {
			r := leaseRoutes[id]
			if r == "" {
				return browserpolicy.RouteExternal
			}
			return r
		}(), WorkspaceID: m.Scope.WorkspaceID, WorkerID: m.WorkerID, PageID: m.PageID, ConnectorID: bind.ConnectorID, ConnectorPID: m.ConnectorPID.PID, OwnerType: m.OwnerType, Ownership: BrokerOwnershipDiagnostic{Process: m.Ownership.Process, Profile: m.Ownership.Profile, Connector: m.Ownership.Connector}, ForegroundPolicy: m.ForegroundPolicy, CleanupState: m.CleanupState, CleanupReason: m.CleanupReason, CleanupError: m.CleanupError, CreatedAt: m.CreatedAt, LastActiveAt: m.LastActiveAt, ExpiresAt: m.ExpiresAt})
	}
	sort.Slice(leases, func(i, j int) bool { return leases[i].CreatedAt.Before(leases[j].CreatedAt) })

	b.managed.queue.mu.Lock()
	active, queued := b.managed.queue.active, len(b.managed.queue.waiters)
	b.managed.queue.mu.Unlock()
	b.managed.mu.Lock()
	managedOrphans := len(b.managed.orphans)
	b.managed.mu.Unlock()
	lastErr := ""
	if b.lifecycle != nil && b.lifecycle.LastError() != nil {
		lastErr = b.lifecycle.LastError().Error()
	}
	return BrokerDiagnostics{Owners: owners, Leases: leases, Workers: b.registry.Snapshot(), Queue: BrokerQueueDiagnostic{Active: active, Queued: queued, MaxConcurrency: b.managed.policy.MaxConcurrency, QueueCapacity: b.managed.policy.QueueCapacity, ManagedOrphans: managedOrphans, ExternalOrphans: externalOrphans}, LifecycleLastError: lastErr}
}

func (b *ACPBridge) CleanupStale(now time.Time) error { return b.sweepLifecycle(now.UTC()) }
