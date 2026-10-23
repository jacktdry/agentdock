package browser

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

// Core-only lifecycle, not a standalone MCP import or renderer status source.
type externalRuntime interface {
	ConnectorStatusProvider
	ExternalLeaseBackend
	externalPeerVerifier
	externalTransportFence
	Connect(context.Context, string) error
	Disconnect(context.Context, string) error
	setRevalidation(externalPeerRevalidation)
	workerSnapshot() []BrokerWorkerDiagnostic
	Close() error
}

type nativeEdgeConnection struct {
	root         *directCDPDispatcher
	peer         externalPeerIdentity
	source       edgeProcessPreflight
	profile      browserpolicy.ProfileDefinition
	intent       browserpolicy.ProfileDefinition
	profileEpoch edgeProfileMetadata
	connector    browserpolicy.ConnectorDefinition
	cancel       context.CancelFunc
}

type nativeEdgeDependencies struct {
	dial   func(context.Context, browserpolicy.ConnectorDefinition, browserpolicy.ProfileDefinition, browserpolicy.OriginScope) (*nativeEdgeConnection, error)
	verify func(context.Context, *nativeEdgeConnection) error
}

// Only finite Core-authored stages may reach the operator. Raw CDP, signing,
// socket or Profile errors must never be included in a connection failure.
type nativeEdgeConnectFailure struct{ stage string }

func (e *nativeEdgeConnectFailure) Error() string {
	return "native company Edge connection unavailable"
}

func nativeEdgeConnectError(err error) error {
	var failure *nativeEdgeConnectFailure
	if errors.As(err, &failure) {
		switch failure.stage {
		case "configuration", "process_preflight", "socket_and_websocket", "dispatcher_attach", "browser_metadata", "profile_context", "owned_target_create", "owned_profile_metadata", "owned_profile_cleanup", "profile_epoch", "peer_revalidation", "connection_admission":
			return externalLeaseError(ErrRequiredRouteUnavailable, "", "company Edge connection unavailable at "+failure.stage, nil)
		}
	}
	return externalPeerVerificationError()
}

type nativeEdgeWorker struct {
	mu         sync.Mutex
	info       WorkerInfo
	connection *nativeEdgeConnection
	target     *directCDPDispatcher
	series     *externalFenceSeries
	stopped    bool
}

type nativeEdgeRuntime struct {
	mu             sync.Mutex
	connectGate    chan struct{}
	catalog        browserpolicy.Catalog
	scopes         map[string]browserpolicy.OriginScope
	pins           map[string]string
	connection     *nativeEdgeConnection
	workers        map[string]*nativeEdgeWorker
	stopped        map[string]bool
	stopOrder      []string
	deps           nativeEdgeDependencies
	reverify       externalPeerRevalidation
	closed         bool
	pendingConnect context.CancelFunc
}

// NewNextEdgeRoutePlanner creates no connection. Only an explicit Broker
// connect operation may initiate Edge's own authorization handshake. Runtime
// status and lease acquisition never reconnect or silently switch profiles.
func NewNextEdgeRoutePlanner(policies []browserpolicy.WorkspaceRootPolicy, catalog browserpolicy.Catalog, home string) (*RoutePlanner, error) {
	userHome, err := os.UserHomeDir()
	if err != nil || runtime.GOOS != "darwin" || os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" || home != filepath.Join(userHome, ".agentdock-next") {
		return nil, externalTransportFenceError()
	}
	deps, err := nativeEdgePlatformDependencies()
	if err != nil {
		return nil, err
	}
	native := &nativeEdgeRuntime{catalog: catalog, deps: deps, workers: make(map[string]*nativeEdgeWorker), stopped: make(map[string]bool), connectGate: make(chan struct{}, 1), scopes: make(map[string]browserpolicy.OriginScope)}
	planner, err := NewRoutePlanner(policies, catalog, native)
	if err != nil {
		return nil, err
	}
	count := 0
	for _, connector := range catalog.Definition().Connectors {
		if connector.Driver != browserpolicy.DriverNextEdgeCDP {
			continue
		}
		count++
		seen := map[string]bool{}
		origins := []string{}
		for _, mapping := range planner.policies {
			if mapping.Policy.RequiredConnectorID != connector.ID {
				continue
			}
			for _, origin := range mapping.Policy.AllowedOrigins {
				if !seen[origin] {
					origins = append(origins, origin)
					seen[origin] = true
				}
			}
		}
		scope, err := browserpolicy.NewOriginScope(origins)
		if err != nil || scope == "" {
			return nil, externalTransportFenceError()
		}
		native.scopes[connector.ID] = scope
	}
	if count != 1 {
		return nil, externalTransportFenceError()
	}
	planner.externalRuntime = native
	return planner, nil
}

func (b *nativeEdgeRuntime) setRevalidation(verify externalPeerRevalidation) { b.reverify = verify } // Construction only.

func (b *nativeEdgeRuntime) workerSnapshot() []BrokerWorkerDiagnostic {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]BrokerWorkerDiagnostic, 0, len(b.workers))
	for _, w := range b.workers {
		info := w.info
		state := info.State
		if b.closed {
			state = WorkerStopped
		} else if b.connection != w.connection || !w.connection.root.live() {
			state = WorkerFailed
		}
		out = append(out, BrokerWorkerDiagnostic{WorkerID: info.WorkerID, CreatedAt: info.CreatedAt, State: state,
			PID: info.Session.PID, ServerName: info.Session.ServerName, ServerVersion: info.Session.ServerVersion, PageIDRouting: true})
	}
	return out
}

func (b *nativeEdgeRuntime) Connect(ctx context.Context, id string) error {
	if ctx == nil || ctx.Err() != nil {
		return externalPeerVerificationError()
	}
	select {
	case b.connectGate <- struct{}{}:
	case <-ctx.Done():
		return externalPeerVerificationError()
	}
	defer func() { <-b.connectGate }()
	connector, ok := b.catalog.Connector(id)
	profile, pok := b.catalog.Profile(connector.ProfileID)
	scope := b.scopes[id]
	if !ok || !pok || scope == "" || connector.Driver != browserpolicy.DriverNextEdgeCDP || b.deps.dial == nil || b.deps.verify == nil {
		return externalPeerVerificationError()
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return externalPeerVerificationError()
	}
	if existing := b.connection; existing != nil && existing.root.live() {
		b.mu.Unlock()
		if existing.connector != connector || existing.intent != profile {
			return externalPeerVerificationError()
		}
		if err := b.deps.verify(ctx, existing); err != nil {
			existing.root.Close()
			return externalPeerVerificationError()
		}
		return nil
	}
	// Explicit reconnection starts a new opaque generation. Old workers keep
	// only their old source pointer; their target IDs never reach this socket.
	intent := profile
	if profile.BindCurrentProfile && b.pins[id] != "" {
		profile.ProfileDirectory = b.pins[id]
		profile.BindCurrentProfile = false
	}
	connectCtx, stopConnect := context.WithCancel(ctx)
	b.pendingConnect = stopConnect
	b.mu.Unlock()
	defer func() { stopConnect(); b.mu.Lock(); b.pendingConnect = nil; b.mu.Unlock() }()
	connection, err := b.deps.dial(connectCtx, connector, profile, scope)
	if err != nil || connection == nil {
		return nativeEdgeConnectError(err)
	}
	if connection.root == nil || connection.cancel == nil || connection.connector != connector ||
		connection.peer.source == nil || connection.peer.incarnation == nil || connection.peer.source == connection.peer.incarnation ||
		connection.profile.ID != profile.ID || connection.profile.UserDataDir != profile.UserDataDir ||
		!browserpolicy.ValidExternalProfileDirectory(connection.profile.ProfileDirectory) ||
		(profile.ProfileDirectory != "" && connection.profile.ProfileDirectory != profile.ProfileDirectory) || b.deps.verify(connectCtx, connection) != nil {
		if connection.cancel != nil {
			connection.cancel()
		}
		if connection.root != nil {
			connection.root.Close()
		}
		return nativeEdgeConnectError(&nativeEdgeConnectFailure{stage: "connection_admission"})
	}
	connection.intent = intent
	b.mu.Lock()
	if b.closed || ctx.Err() != nil {
		b.mu.Unlock()
		connection.cancel()
		connection.root.Close()
		return externalPeerVerificationError()
	}
	old := b.connection
	b.connection = connection
	if b.pins == nil {
		b.pins = make(map[string]string)
	}
	b.pins[id] = connection.profile.ProfileDirectory
	b.mu.Unlock()
	if old != nil {
		old.cancel()
		old.root.Close()
	}
	return nil
}

func (b *nativeEdgeRuntime) ConnectorStatus(ctx context.Context, id string) (ConnectorRuntimeStatus, error) {
	b.mu.Lock()
	connection := b.connection
	closed := b.closed
	b.mu.Unlock()
	if ctx == nil || ctx.Err() != nil || closed || connection == nil || connection.connector.ID != id {
		return ConnectorRuntimeStatus{}, externalPeerVerificationError()
	}
	if b.deps.verify(ctx, connection) != nil {
		connection.root.Close()
		return ConnectorRuntimeStatus{}, externalPeerVerificationError()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.connection != connection || !connection.root.live() || ctx.Err() != nil {
		return ConnectorRuntimeStatus{}, externalPeerVerificationError()
	}
	now := time.Now().UTC()
	status := ConnectorRuntimeStatus{ConnectorID: id, ProfileID: connection.profile.ID, Endpoint: connection.connector.Endpoint,
		ObservedAt: now, ExpiresAt: now.Add(5 * time.Second), Healthy: true, Verified: true, Authenticated: true,
		Engine: EngineNativeCDP, EngineVersion: nativeEdgeEngineVersion, Transport: "websocket", Capabilities: requiredRouteCapabilities(),
		source: connection.peer.source, incarnation: connection.peer.incarnation}
	status.qualification = &connectorQualification{format: 1, observation: copyConnectorRuntimeStatus(status)}
	return status, nil
}

func (b *nativeEdgeRuntime) StartExternal(ctx context.Context, options ExternalWorkerOptions) (WorkerInfo, error) {
	route := options.Route
	if ctx == nil || ctx.Err() != nil || validateExternalStart(route) != nil || route.Start.Engine != EngineNativeCDP || route.grant == nil ||
		!route.grant.used.Load() || !route.grant.started.CompareAndSwap(false, true) {
		return WorkerInfo{}, externalTransportFenceError()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.connection
	if b.closed || c == nil || c.peer != route.grant.peer || !c.root.live() || b.reverify == nil ||
		c.connector.ID != route.Start.ConnectorID || c.connector.Endpoint != route.Start.Endpoint || c.profile.ID != route.Start.ProfileID ||
		c.profile.UserDataDir != route.Start.ProfilePath {
		return WorkerInfo{}, externalTransportFenceError()
	}
	active := 0
	for _, w := range b.workers {
		if !w.stopped {
			active++
		}
	}
	if active >= 16 {
		return WorkerInfo{}, externalTransportFenceError()
	}
	target, err := c.root.forkTarget(route.Start.OriginScope)
	if err != nil {
		return WorkerInfo{}, externalTransportFenceError()
	}
	info := WorkerInfo{WorkerID: rand.Text(), CreatedAt: time.Now().UTC(), State: WorkerReady,
		Session: mcpclient.SessionInfo{ProtocolVersion: "1.3", ServerName: "agentdock-core-edge-cdp", ServerVersion: nativeEdgeEngineVersion, PID: os.Getpid()}}
	w := &nativeEdgeWorker{info: info, connection: c, target: target}
	series, err := newExternalFenceSeries(info, c.peer, func(ctx context.Context, current WorkerInfo, peer externalPeerIdentity) (externalFencedOperation, error) {
		if ctx.Err() != nil || current != w.info || peer != w.connection.peer {
			return nil, externalTransportFenceError()
		}
		return &nativeEdgeOperation{worker: w, peer: peer}, nil
	}, b.reverify)
	if err != nil {
		return WorkerInfo{}, err
	}
	w.series = series
	b.workers[info.WorkerID] = w
	return info, nil
}

func (b *nativeEdgeRuntime) worker(info WorkerInfo) (*nativeEdgeWorker, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	w := b.workers[info.WorkerID]
	if b.closed || w == nil || w.info != info || b.connection != w.connection {
		return nil, externalPeerVerificationError()
	}
	return w, nil
}

func (b *nativeEdgeRuntime) VerifyExternalPeer(ctx context.Context, peer externalPeerIdentity, info WorkerInfo) (externalPeerIdentity, error) {
	w, err := b.worker(info)
	if err != nil || ctx == nil || peer != w.connection.peer {
		return externalPeerIdentity{}, externalPeerVerificationError()
	}
	if b.deps.verify(ctx, w.connection) != nil {
		w.connection.root.Close()
		return externalPeerIdentity{}, externalPeerVerificationError()
	}
	b.mu.Lock()
	same := !b.closed && b.connection == w.connection
	b.mu.Unlock()
	if !same || !w.connection.root.live() || ctx.Err() != nil {
		return externalPeerIdentity{}, externalPeerVerificationError()
	}
	return w.connection.peer, nil
}

// No unfenced compatibility path is permitted for native Edge.
func (b *nativeEdgeRuntime) CallExternal(context.Context, WorkerInfo, string, map[string]any) (map[string]any, error) {
	return nil, externalTransportFenceError()
}

func (b *nativeEdgeRuntime) CallExternalFenced(ctx context.Context, info WorkerInfo, peer externalPeerIdentity, tool string, args map[string]any) (map[string]any, error) {
	w, err := b.worker(info)
	if err != nil {
		return nil, err
	}
	return w.series.CallExternalFenced(ctx, info, peer, tool, args)
}

func (b *nativeEdgeRuntime) Stop(id string) error {
	b.mu.Lock()
	w := b.workers[id]
	stopped := b.stopped[id]
	b.mu.Unlock()
	if stopped {
		return nil
	}
	if w == nil {
		return externalPeerVerificationError()
	}
	w.series.Close()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return nil
	}
	if w.target.targetID != "" && !w.target.closeConfirmed {
		ctx, cancel := context.WithTimeout(context.Background(), directCDPCallTimeout)
		defer cancel()
		if b.deps.verify(ctx, w.connection) != nil {
			// Lost identity revokes the entire held transport. No old target ID
			// is sent after a process/Profile change, even during recovery.
			w.connection.root.Close()
		}
		if _, err := w.target.CloseOwnedTarget(ctx); err != nil && w.connection.root.live() {
			return externalPeerVerificationError()
		}
	}
	// A lost socket disposes its hidden targets in Edge. Never use another
	// connection, profile, PID or guessed target to retry uncertain cleanup.
	b.mu.Lock()
	w.stopped = true
	delete(b.workers, id)
	if b.stopped == nil {
		b.stopped = make(map[string]bool)
	}
	b.stopped[id] = true
	b.stopOrder = append(b.stopOrder, id)
	if len(b.stopOrder) > 1024 {
		delete(b.stopped, b.stopOrder[0])
		b.stopOrder = b.stopOrder[1:]
	}
	b.mu.Unlock()
	return nil
}

func (b *nativeEdgeRuntime) Close() error {
	b.mu.Lock()
	b.closed = true
	c := b.connection
	pending := b.pendingConnect
	b.mu.Unlock()
	if pending != nil {
		pending()
		select {
		case b.connectGate <- struct{}{}:
			<-b.connectGate
		case <-time.After(3 * time.Second):
			return externalPeerVerificationError()
		}
	}
	if c != nil {
		c.cancel()
		c.root.Close()
		select {
		case <-c.root.relay.Done():
		case <-time.After(3 * time.Second):
			return externalPeerVerificationError()
		}
	}
	return nil
}

func (b *nativeEdgeRuntime) Disconnect(ctx context.Context, id string) error {
	if ctx == nil || ctx.Err() != nil {
		return externalPeerVerificationError()
	}
	select {
	case b.connectGate <- struct{}{}:
	case <-ctx.Done():
		return externalPeerVerificationError()
	}
	defer func() { <-b.connectGate }()
	b.mu.Lock()
	c := b.connection
	if b.closed || c == nil || c.connector.ID != id {
		b.mu.Unlock()
		return externalPeerVerificationError()
	}
	for _, w := range b.workers {
		if w.connection == c && !w.stopped {
			b.mu.Unlock()
			return externalLeaseError(ErrLeaseStateInvalid, "", "company connection still has active leases", nil)
		}
	}
	b.connection = nil
	b.mu.Unlock()
	c.cancel()
	c.root.Close()
	select {
	case <-c.root.relay.Done():
		return nil
	case <-ctx.Done():
		return externalPeerVerificationError()
	}
}
