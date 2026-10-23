package browser

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uvwt/agentdock/internal/browserpolicy"
)

// Uses a real local WebSocket and the production planner/bridge/lease/fence
// path. Only OS identity/dial dependencies are synthetic; this is not Edge UAT.
func nativeRuntimeFixture(t *testing.T) (*ACPBridge, *nativeEdgeRuntime, *dispatcherFixture, string, *atomic.Int32, *atomic.Bool) {
	t.Helper()
	urls, sessions := map[string]string{}, map[string]string{}
	next := 0
	root, f := newDispatcherFixture(t, func(_ *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
		var result any
		switch cmd.Method {
		case "Target.getBrowserContexts":
			result = map[string]any{"browserContextIds": []string{}, "defaultBrowserContextId": "company"}
		case "Target.createTarget":
			next++
			id := fmt.Sprintf("target-%d", next)
			urls[id] = cmd.Params["url"].(string)
			result = map[string]any{"targetId": id}
		case "Target.attachToTarget":
			id := cmd.Params["targetId"].(string)
			sessions["session-"+id] = id
			result = map[string]any{"sessionId": "session-" + id}
		case "Target.getTargetInfo":
			id := cmd.Params["targetId"].(string)
			result = map[string]any{"targetInfo": map[string]any{"targetId": id, "type": "page", "url": urls[id], "browserContextId": "company"}}
		case "Page.navigate":
			urls[sessions[cmd.SessionID]] = cmd.Params["url"].(string)
			result = map[string]any{}
		case "Target.closeTarget":
			id := cmd.Params["targetId"].(string)
			_, exists := urls[id]
			delete(urls, id)
			result = map[string]any{"success": exists}
		case "Fetch.enable":
			result = map[string]any{}
		default:
			return false
		}
		if err := dispatcherReply(c, cmd, result); err != nil {
			t.Error(err)
		}
		return true
	})
	if err := root.bindBrowserContext(context.Background(), "company"); err != nil {
		t.Fatal(err)
	}
	profile := browserpolicy.ProfileDefinition{ID: "company", Browser: browserpolicy.BrowserEdge, Class: browserpolicy.ProfileAuthenticatedExternal, UserDataDir: "/configured/edge", SelectedProfileName: "公司", BindCurrentProfile: true}
	connector := browserpolicy.ConnectorDefinition{ID: "company-edge", ProfileID: profile.ID, Driver: browserpolicy.DriverNextEdgeCDP, Endpoint: "ws://127.0.0.1:9222/devtools/browser"}
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{Profiles: []browserpolicy.ProfileDefinition{profile}, Connectors: []browserpolicy.ConnectorDefinition{connector}})
	if err != nil {
		t.Fatal(err)
	}
	dials, identityLost := &atomic.Int32{}, &atomic.Bool{}
	native := &nativeEdgeRuntime{catalog: catalog, scopes: map[string]browserpolicy.OriginScope{connector.ID: root.originScope}, workers: map[string]*nativeEdgeWorker{}, connectGate: make(chan struct{}, 1)}
	native.deps = nativeEdgeDependencies{
		dial: func(ctx context.Context, c browserpolicy.ConnectorDefinition, p browserpolicy.ProfileDefinition, s browserpolicy.OriginScope) (*nativeEdgeConnection, error) {
			dials.Add(1)
			p.ProfileDirectory, p.BindCurrentProfile = "Profile 1", false
			return &nativeEdgeConnection{root: root, peer: root.relay.peer, connector: c, profile: p, cancel: func() {}}, nil
		},
		verify: func(ctx context.Context, c *nativeEdgeConnection) error {
			if ctx == nil || ctx.Err() != nil || identityLost.Load() || !c.root.live() {
				return externalPeerVerificationError()
			}
			return nil
		},
	}
	workspace := t.TempDir()
	planner, err := NewRoutePlanner([]browserpolicy.WorkspaceRootPolicy{{Root: workspace, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredProfileID: profile.ID, RequiredConnectorID: connector.ID, AllowedOrigins: []string{"https://example.invalid"}}}}, catalog, native)
	if err != nil {
		t.Fatal(err)
	}
	planner.externalRuntime = native
	bridge, err := NewACPBridge(planner, NewWorkerRegistry(WorkerDependencies{}))
	if err != nil {
		t.Fatal(err)
	}
	bridge.lifecycle.Close()
	t.Cleanup(func() { _ = bridge.Close() })
	return bridge, native, f, workspace, dials, identityLost
}

func TestNativeEdgeBrokerSharesConnectionAndFencesOwners(t *testing.T) {
	b, native, f, root, dials, _ := nativeRuntimeFixture(t)
	ctx := directContext("chatgpt")
	if _, err := b.DirectAcquire(ctx, root, ""); err == nil || dials.Load() != 0 {
		t.Fatal("acquire connected without explicit authorization")
	}
	if b.ConnectExternal(context.Background(), root, false) == nil || dials.Load() != 0 {
		t.Fatal("unauthenticated actor connected")
	}
	if err := b.ConnectExternal(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	if err := b.ConnectExternal(ctx, root, false); err != nil || dials.Load() != 1 {
		t.Fatal("second connect did not retain authorized source", err)
	}
	a, err := b.DirectAcquire(ctx, root, "https://example.invalid/")
	if err != nil {
		t.Fatal(err)
	}
	token, acp := sharedBridgeAcquire(t, b, "codex-company", root)
	diagnostics := b.Diagnostics()
	if len(diagnostics.Workers) != 2 {
		t.Fatal("native workers missing from broker diagnostics")
	}
	for _, lease := range diagnostics.Leases {
		if lease.LeaseID == a.BrowserLeaseID && (!lease.AuthenticatedDirectOwner || lease.Route != browserpolicy.RouteRequiredExternal) {
			t.Fatal("authenticated direct company lease reported as unowned or wrong route")
		}
	}
	if a.WorkerID == acp.WorkerID || a.BrowserLeaseID == acp.BrowserLeaseID || a.PageID != "1" || acp.PageID != "1" {
		t.Fatal("logical page IDs must be isolated by immutable worker ownership")
	}
	if err := b.ConnectExternal(ctx, root, true); err == nil || !native.connection.root.live() {
		t.Fatal("disconnect dropped active sibling leases")
	}
	before := len(f.recorded())
	if _, err := b.DirectCall(directContext("other"), root, a.BrowserLeaseID, "take_snapshot", nil); err == nil || len(f.recorded()) != before {
		t.Fatal("cross-principal call reached the transport")
	}
	if _, err := b.DirectRelease(ctx, root, acp.BrowserLeaseID); err == nil {
		t.Fatal("direct actor released ACP page")
	}
	if _, err := b.Call(context.Background(), token, a.BrowserLeaseID, "take_snapshot", nil); err == nil {
		t.Fatal("ACP actor accessed direct page")
	}
	meta, err := b.DirectRelease(ctx, root, a.BrowserLeaseID)
	if err != nil || meta.CleanupState != CleanupComplete || meta.CleanupReason != "owned external page closed; native lease retired; shared connection preserved" {
		t.Fatal("native cleanup status inaccurate", meta, err)
	}
	if _, err := b.Call(context.Background(), token, acp.BrowserLeaseID, "take_snapshot", nil); err != nil {
		t.Fatal("release broke sibling", err)
	}
	if _, err := b.DirectRelease(ctx, root, a.BrowserLeaseID); err != nil {
		t.Fatal("release is not idempotent", err)
	}
	if err := b.ReleaseSession(context.Background(), "codex-company"); err != nil {
		t.Fatal(err)
	}
	if len(native.workers) != 0 || commandCount(f.recorded(), "Target.closeTarget") != 2 || f.connections.Load() != 1 || dials.Load() != 1 {
		t.Fatal("resource release or single-connection reuse failed")
	}
	for _, method := range []string{"Browser.close", "Target.getTargets", "Target.activateTarget", "Target.setDiscoverTargets", "Browser.setDownloadBehavior"} {
		if commandCount(f.recorded(), method) != 0 {
			t.Fatalf("global browser mutation or user catalog: %s", method)
		}
	}
	if err := b.ConnectExternal(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := b.DirectAcquire(ctx, root, ""); err == nil || dials.Load() != 1 {
		t.Fatal("disconnected source auto-reconnected")
	}
}

func TestNativeEdgeLostIdentityQuarantinesAndDoesNotCloseOldID(t *testing.T) {
	b, native, f, root, dials, lost := nativeRuntimeFixture(t)
	ctx := directContext("chatgpt")
	if err := b.ConnectExternal(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	meta, err := b.DirectAcquire(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	lost.Store(true)
	before := len(f.recorded())
	if _, err := b.DirectCall(ctx, root, meta.BrowserLeaseID, "take_snapshot", nil); err == nil {
		t.Fatal("changed Profile/process identity remained trusted")
	}
	if workers := b.Diagnostics().Workers; len(workers) != 1 || workers[0].State != WorkerFailed {
		t.Fatal("lost native transport reported ready")
	}
	if _, err := b.DirectRelease(ctx, root, meta.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	if len(f.recorded()) != before || native.connection.root.live() || dials.Load() != 1 {
		t.Fatal("uncertain cleanup wrote an old ID or reconnected")
	}
	if _, err := b.DirectAcquire(ctx, root, ""); err == nil || dials.Load() != 1 {
		t.Fatal("lost identity fell back or auto-connected")
	}
}

func TestNativeEdgeRuntimeCloseCancelsPendingConsent(t *testing.T) {
	_, native, _, _, _, _ := nativeRuntimeFixture(t)
	entered, done := make(chan struct{}), make(chan error, 1)
	native.deps.dial = func(ctx context.Context, _ browserpolicy.ConnectorDefinition, _ browserpolicy.ProfileDefinition, _ browserpolicy.OriginScope) (*nativeEdgeConnection, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	go func() { done <- native.Connect(context.Background(), "company-edge") }()
	pinnedWait(t, entered)
	if err := native.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled consent published a connection")
		}
	case <-time.After(time.Second):
		t.Fatal("pending connect did not stop")
	}
}

func TestNativeEdgeExplicitReconnectPinsProfileAndIsolatesOldGeneration(t *testing.T) {
	b, native, f, root, _, _ := nativeRuntimeFixture(t)
	_, replacement, f2, _, _, _ := nativeRuntimeFixture(t)
	ctx := directContext("chatgpt")
	if err := b.ConnectExternal(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	old, err := b.DirectAcquire(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	native.connection.root.Close()
	pinnedDirectory := ""
	native.deps.dial = func(ctx context.Context, c browserpolicy.ConnectorDefinition, p browserpolicy.ProfileDefinition, scope browserpolicy.OriginScope) (*nativeEdgeConnection, error) {
		pinnedDirectory = p.ProfileDirectory
		if p.BindCurrentProfile {
			t.Error("reconnect would silently follow a different current Profile")
		}
		return replacement.deps.dial(ctx, c, p, scope)
	}
	if err := b.ConnectExternal(ctx, root, false); err != nil || pinnedDirectory != "Profile 1" {
		t.Fatal("explicit reconnect lost observed Profile pin", pinnedDirectory, err)
	}
	before := len(f2.recorded())
	if _, err := b.DirectCall(ctx, root, old.BrowserLeaseID, "take_snapshot", nil); err == nil || len(f2.recorded()) != before {
		t.Fatal("old generation reached new transport")
	}
	if _, err := b.DirectRelease(ctx, root, old.BrowserLeaseID); err != nil || len(f2.recorded()) != before {
		t.Fatal("old cleanup closed a target in the new generation", err)
	}
	current, err := b.DirectAcquire(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.DirectRelease(ctx, root, current.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	if commandCount(f.recorded(), "Target.closeTarget") != 0 || commandCount(f2.recorded(), "Target.closeTarget") != 1 {
		t.Fatal("generation cleanup crossed its held connection")
	}
}

func TestNativeEdgeConnectionFailureStagesExcludeRawDetails(t *testing.T) {
	for _, stage := range []string{"configuration", "process_preflight", "socket_and_websocket", "dispatcher_attach", "browser_metadata", "profile_context", "owned_target_create", "owned_profile_metadata", "owned_profile_cleanup", "profile_epoch", "peer_revalidation", "connection_admission"} {
		err := nativeEdgeConnectError(fmt.Errorf("secret-canary: %w", &nativeEdgeConnectFailure{stage: stage}))
		var reported *Error
		if !errors.As(err, &reported) || reported.Details.Reason != "company Edge connection unavailable at "+stage || reported.Cause != nil || strings.Contains(err.Error(), "secret-canary") {
			t.Fatal("finite stage lost or raw detail exposed", err)
		}
	}
	for _, err := range []error{errors.New("secret-canary"), &nativeEdgeConnectFailure{stage: "secret-canary"}} {
		if strings.Contains(nativeEdgeConnectError(err).Error(), "secret-canary") {
			t.Fatal("untrusted stage exposed")
		}
	}
}
