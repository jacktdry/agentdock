package browser

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

func TestBrokerDiagnosticsTraceOwnershipWithoutCapabilityToken(t *testing.T) {
	bridge := newTestACPBridge(t)
	root := t.TempDir()
	token, err := bridge.RegisterSessionWithToken("acps-diag", "antigravity", root, "super-secret-capability")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("token missing")
	}
	now := time.Now().UTC()
	leaseID := "lease-diag"
	workerID := "worker-diag"
	bridge.mu.Lock()
	owner := bridge.bySession["acps-diag"]
	owner.leases[leaseID] = acpBrowserLease{route: browserpolicy.RouteManaged}
	bridge.mu.Unlock()
	bridge.managed.manager.mu.Lock()
	bridge.managed.manager.leases[leaseID] = &managedLease{metadata: LeaseMetadata{
		BrowserLeaseID: leaseID, BrowserSessionID: "browser-session-diag", Scope: owner.scope,
		WorkerID: workerID, PageID: "7", IsolationContextName: "agentdock-diag",
		OwnerType: OwnerAgentDockIsolated, Ownership: ResourceOwnership{Process: OwnerAgentDockIsolated, Profile: OwnerAgentDockIsolated, Connector: OwnerAgentDockIsolated},
		ForegroundPolicy: ForegroundForbidden, LifecyclePolicy: LifecycleOwned,
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now.Add(time.Minute), CleanupState: CleanupPending,
	}, binding: EngineBinding{BrowserLeaseID: leaseID, WorkerID: workerID, PageID: "7"}}
	bridge.managed.manager.mu.Unlock()
	bridge.registry.mu.Lock()
	bridge.registry.workers[workerID] = &managedWorker{info: WorkerInfo{WorkerID: workerID, CreatedAt: now, State: WorkerReady, Session: mcpclient.SessionInfo{PID: 4321, ServerName: ManagedEngineServerName, ServerVersion: PreferredEngineVersion}, Compatibility: ManagedCompatibility{PageIDRouting: true}}}
	bridge.registry.mu.Unlock()

	diagnostics := bridge.Diagnostics()
	encoded, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, want := range []string{"acps-diag", "antigravity", leaseID, workerID, "agentdock-diag", `"page_id":"7"`, `"process":"agentdock-isolated"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("diagnostics missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, token) || strings.Contains(text, "super-secret-capability") {
		t.Fatalf("diagnostics leaked capability token: %s", text)
	}
	// This test injects metadata without a real MCP session; mark it terminal so
	// bridge cleanup does not treat the synthetic worker as an executable resource.
	bridge.managed.manager.mu.Lock()
	injected := bridge.managed.manager.leases[leaseID]
	bridge.managed.manager.mu.Unlock()
	injected.mu.Lock()
	injected.metadata.CleanupState = CleanupComplete
	injected.mu.Unlock()
	bridge.registry.mu.Lock()
	delete(bridge.registry.workers, workerID)
	bridge.registry.mu.Unlock()
}
