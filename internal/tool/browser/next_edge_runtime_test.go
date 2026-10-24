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
	viewports := map[string]bool{}
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
			// Real Edge classifies owned hidden WebContents as "other".
			result = map[string]any{"targetInfo": map[string]any{"targetId": id, "type": "other", "url": urls[id], "browserContextId": "company"}}
		case "Page.navigate":
			if !viewports[cmd.SessionID] {
				t.Error("company navigation starts with a zero-sized hidden viewport")
			}
			urls[sessions[cmd.SessionID]] = cmd.Params["url"].(string)
			result = map[string]any{}
		case "Target.closeTarget":
			id := cmd.Params["targetId"].(string)
			_, exists := urls[id]
			delete(urls, id)
			result = map[string]any{"success": exists}
		case "Fetch.enable":
			result = map[string]any{}
		case "Emulation.setDeviceMetricsOverride":
			if cmd.SessionID == "" || cmd.Params["width"] != float64(1280) || cmd.Params["height"] != float64(800) || cmd.Params["deviceScaleFactor"] != float64(1) || cmd.Params["mobile"] != false {
				t.Error("native viewport is not fixed and target-session scoped")
			}
			viewports[cmd.SessionID] = true
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

func TestNativeEdgePeerBudgetCoversSequentialOSStages(t *testing.T) {
	// A native peer verifier may validly take longer than ONE 2s child
	// deadline because it must also inspect the independent held socket.
	// No Edge process, Profile, user browser or real CDP connection is used.
	if nativeEdgePeerVerificationBudget <= 2*edgeSocketWindow {
		t.Fatal("outer native peer budget cannot fit sequential OS checks")
	}
	b, native, _, root, _, _ := nativeRuntimeFixture(t)
	ctx := directContext("chatgpt")
	if err := b.ConnectExternal(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	const syntheticOSProof = 2250 * time.Millisecond
	native.deps.verify = func(ctx context.Context, conn *nativeEdgeConnection) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < syntheticOSProof || !conn.root.live() {
			return externalPeerVerificationError()
		}
		select {
		case <-time.After(syntheticOSProof):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	scope, err := b.directScope(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := b.planner.Resolve(ctx, scope, nil)
	if err != nil || decision.grant == nil {
		t.Fatalf("native planner rejected a valid sequential proof: %v", err)
	}
	if len(b.planner.statusInFlight) != 0 {
		t.Fatal("native planner returned a proof before releasing the next-admission slot")
	}
	worker := WorkerInfo{WorkerID: "synthetic-native-worker", State: WorkerReady}
	native.mu.Lock()
	native.workers[worker.WorkerID] = &nativeEdgeWorker{info: worker, connection: native.connection}
	peer := native.connection.peer
	native.mu.Unlock()
	if err := b.external.verifyPeer(ctx, peer, worker); err != nil {
		t.Fatalf("native lease verifier rejected a valid sequential proof: %v", err)
	}
	if len(b.external.verificationInFlight) != 0 {
		t.Fatal("native lease peer proof returned before releasing its revalidation slot")
	}
	native.mu.Lock()
	delete(native.workers, worker.WorkerID)
	native.mu.Unlock()
}

func TestNativeEdgePeerStageOnlyAllowsFiniteDetails(t *testing.T) {
	for _, stage := range []string{"transport", "process_epoch", "profile_epoch", "held_socket", "postcondition"} {
		if safeNativeEdgePeerStage(&nativeEdgePeerStageFailure{stage: stage}) != stage {
			t.Fatal("known native rejection lost its finite stage")
		}
		if !strings.Contains(nativePeerVerificationErrorAt(stage).Error(), "at "+stage) {
			t.Fatal("native rejection not actionable")
		}
	}
	for _, unknown := range []error{nil, errors.New("private path token PID:123"), &nativeEdgePeerStageFailure{stage: "private path token PID:123"}} {
		if safeNativeEdgePeerStage(unknown) != "" || strings.Contains(nativePeerVerificationErrorAt(safeNativeEdgePeerStage(unknown)).Error(), "private") {
			t.Fatal("unknown native verifier detail escaped")
		}
	}
}

func TestNativeEdgePeerStageSurvivesLeaseBoundaryWithoutRawErrors(t *testing.T) {
	for _, tc := range []struct {
		name, stage string
		err         error
	}{
		{"known", "held_socket", &nativeEdgePeerStageFailure{stage: "held_socket"}},
		{"unknown", "", errors.New("secret Edge profile PID:123 token=private")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, native, _, root, _, _ := nativeRuntimeFixture(t)
			ctx := directContext("chatgpt")
			if err := b.ConnectExternal(ctx, root, false); err != nil {
				t.Fatal(err)
			}
			worker := WorkerInfo{WorkerID: "synthetic-native-worker", State: WorkerReady}
			native.mu.Lock()
			native.workers[worker.WorkerID] = &nativeEdgeWorker{info: worker, connection: native.connection}
			peer := native.connection.peer
			native.mu.Unlock()
			native.deps.verify = func(context.Context, *nativeEdgeConnection) error { return tc.err }
			err := b.external.verifyPeer(ctx, peer, worker)
			if err == nil || !strings.Contains(err.Error(), "external peer verification unavailable") ||
				strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "token=") {
				t.Fatalf("native peer error leaked or silently accepted: %v", err)
			}
			if tc.stage != "" && !strings.Contains(err.Error(), "at "+tc.stage) {
				t.Fatalf("safe native failure phase missing: %v", err)
			}
			if tc.stage == "" && strings.Contains(err.Error(), " at ") {
				t.Fatalf("unknown provider error created a false phase: %v", err)
			}
			if native.connection.root.live() {
				t.Fatal("peer identity failure left the owned native transport active")
			}
			native.mu.Lock()
			delete(native.workers, worker.WorkerID)
			native.mu.Unlock()
		})
	}
}

// A new task must be able to acquire the still-connected Broker after all
// earlier owners have released their pages. This only tests source and a
// synthetic CDP server; live macOS Edge peer revalidation is a separate gate.
func TestNativeEdgeFreshLeaseAfterAllOwnersRelease(t *testing.T) {
	b, native, f, root, dials, _ := nativeRuntimeFixture(t)
	ctx := directContext("chatgpt")
	if err := b.ConnectExternal(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	first, err := b.DirectAcquire(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.DirectAcquire(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, lease := range []LeaseMetadata{first, second} {
		meta, err := b.DirectRelease(ctx, root, lease.BrowserLeaseID)
		if err != nil || meta.CleanupState != CleanupComplete || meta.CleanupError != "" {
			t.Fatalf("first generation release: %+v, %v", meta, err)
		}
	}
	if len(native.workers) != 0 || !native.connection.root.live() || dials.Load() != 1 {
		t.Fatal("releasing the last owner retired the shared connection")
	}
	third, err := b.DirectAcquire(ctx, root, "")
	if err != nil {
		t.Fatalf("fresh lease on the same approved connection failed: %v", err)
	}
	if third.BrowserLeaseID == first.BrowserLeaseID || third.BrowserLeaseID == second.BrowserLeaseID ||
		f.connections.Load() != 1 || dials.Load() != 1 {
		t.Fatal("new owner did not reuse the one connected transport")
	}
	meta, err := b.DirectRelease(ctx, root, third.BrowserLeaseID)
	if err != nil || meta.CleanupState != CleanupComplete || meta.CleanupError != "" ||
		len(native.workers) != 0 || commandCount(f.recorded(), "Target.closeTarget") != 3 {
		t.Fatalf("fresh lease cleanup did not close only owned targets: %+v, %v", meta, err)
	}
}

// Real RC13 Host evidence: two different leases work, sibling release works,
// but an additional lease after both releases fails at ConnectorStatus. This
// regression checks that the exact native rejection phase now survives the
// planner boundary, with NO re-dial or fallback after the rejected admission.
// A genuine macOS Edge run is still required to identify the failing phase.
func TestNativeEdgePostReleaseConnectorFailurePhase(t *testing.T) {
	for _, tc := range []struct {
		name, stage string
		err         error
	}{
		{"transport", "transport", &nativeEdgePeerStageFailure{stage: "transport"}},
		{"process_epoch", "process_epoch", &nativeEdgePeerStageFailure{stage: "process_epoch"}},
		{"profile_epoch", "profile_epoch", &nativeEdgePeerStageFailure{stage: "profile_epoch"}},
		{"held_socket", "held_socket", &nativeEdgePeerStageFailure{stage: "held_socket"}},
		{"postcondition", "postcondition", &nativeEdgePeerStageFailure{stage: "postcondition"}},
		{"unknown_provider_error", "", errors.New("private token=/secret/path PID=111")},
		{"unknown_stage", "", &nativeEdgePeerStageFailure{stage: "private token=/secret/path"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, native, _, root, dials, _ := nativeRuntimeFixture(t)
			ctx := directContext("chatgpt")
			if err := b.ConnectExternal(ctx, root, false); err != nil {
				t.Fatal(err)
			}
			a, err := b.DirectAcquire(ctx, root, "")
			if err != nil {
				t.Fatal(err)
			}
			second, err := b.DirectAcquire(ctx, root, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, lease := range []LeaseMetadata{a, second} {
				m, err := b.DirectRelease(ctx, root, lease.BrowserLeaseID)
				if err != nil || m.CleanupState != CleanupComplete || m.CleanupError != "" {
					t.Fatalf("expected clean sibling release: %+v, %v", m, err)
				}
			}
			if !native.connection.root.live() || len(native.workers) != 0 {
				t.Fatal("fixture prematurely lost the shared transport or workers")
			}
			native.deps.verify = func(context.Context, *nativeEdgeConnection) error { return tc.err }
			_, err = b.DirectAcquire(ctx, root, "")
			if err == nil || dials.Load() != 1 || len(native.workers) != 0 || native.connection.root.live() {
				t.Fatalf("unsafe fresh lease admitted or transport not quarantined: %v", err)
			}
			msg := err.Error()
			if strings.Contains(msg, "private") || strings.Contains(msg, "/secret") ||
				strings.Contains(msg, "PID=") {
				t.Fatal("raw provider evidence escaped the native planner")
			}
			if tc.stage != "" {
				if !strings.Contains(msg, "native Edge peer verification unavailable at "+tc.stage) {
					t.Fatalf("native phase lost after two releases: %v", err)
				}
			} else if strings.Contains(msg, " at ") {
				t.Fatalf("untrusted provider phase accepted: %v", err)
			}
		})
	}
}

func TestNativeEdgePlannerRejectsSpoofedStatusPhase(t *testing.T) {
	b, native, _, root, _, _ := nativeRuntimeFixture(t)
	ctx := directContext("chatgpt")
	if err := b.ConnectExternal(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	// Even a package-local provider able to return the internal error type
	// cannot impersonate the designated native Runtime in the planner.
	b.planner.status = testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) {
		return ConnectorRuntimeStatus{}, &nativeEdgePeerStageFailure{stage: "held_socket"}
	})
	_, err := b.DirectAcquire(ctx, root, "")
	var routeErr *Error
	if err == nil || !errors.As(err, &routeErr) || routeErr.Details == nil ||
		strings.Contains(err.Error(), "held_socket") ||
		routeErr.Details.Reason != "runtime connector verification unavailable" {
		t.Fatalf("untrusted connector supplied a native reason: %v", err)
	}
	if !native.connection.root.live() {
		t.Fatal("spoofed provider must not affect the native runtime")
	}
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
	if commandCount(f.recorded(), "Emulation.setDeviceMetricsOverride") != 2 {
		t.Fatal("native hidden pages have no usable virtual viewport")
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
