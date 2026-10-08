package app

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserdesktop"
	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/config"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

func TestRuntimeBrowserDesktopAvailability(t *testing.T) {
	for _, test := range []struct {
		r    *Runtime
		want string
	}{
		{nil, "core_unavailable"}, {&Runtime{}, "browser_disabled"},
		{&Runtime{cfg: config.Config{BrowserEnabled: true}}, "acp_disabled"},
		{&Runtime{cfg: config.Config{BrowserEnabled: true, ACPEnabled: true}}, "broker_unavailable"},
	} {
		s := test.r.RuntimeBrowserDesktop()["snapshot"].(browserdesktop.Snapshot)
		if s.Availability != test.want || s.State != "unavailable" || s.Leases != 0 || s.ObservedAt == "" {
			t.Fatalf("snapshot %#v", s)
		}
	}
}

func TestRuntimeBrowserDesktopStaleProjectionIsAllowlisted(t *testing.T) {
	now := time.Now().UTC()
	d := toolbrowser.BrokerDiagnostics{
		Owners: []toolbrowser.BrokerOwnerDiagnostic{{ACPSessionID: "PRIVATE_CANARY", CanonicalWorkspaceRoot: "/PRIVATE_CANARY"}},
		Leases: []toolbrowser.BrokerLeaseDiagnostic{
			{ACPSessionID: "PRIVATE_CANARY", PageID: "PRIVATE_CANARY", CleanupState: toolbrowser.CleanupPending, ExpiresAt: now.Add(time.Minute)},
			{ACPSessionID: "PRIVATE_CANARY", CleanupState: toolbrowser.CleanupPending, ExpiresAt: now},
			{CleanupState: toolbrowser.CleanupFailed, CleanupError: "PRIVATE_CANARY", ConnectorPID: 99123},
			{ACPSessionID: "PRIVATE_CANARY", CleanupState: toolbrowser.CleanupReleasing},
		},
		Workers:            []toolbrowser.BrokerWorkerDiagnostic{{State: toolbrowser.WorkerReady, PID: 99123}, {State: toolbrowser.WorkerFailed, Error: "PRIVATE_CANARY"}},
		Queue:              toolbrowser.BrokerQueueDiagnostic{Active: 2, Queued: 3, MaxConcurrency: 4, QueueCapacity: 5, ManagedOrphans: 1, ExternalOrphans: 2},
		LifecycleLastError: "PRIVATE_CANARY",
	}
	s := aggregateBrowserDiagnostics(browserdesktop.Snapshot{Availability: "available"}, d, now)
	if s.State != "stale" || !s.Stale || s.ActiveLeases != 1 || s.ExpiredLeases != 1 || s.UnownedLeases != 1 || s.ReleasingLeases != 1 || s.FailedLeases != 1 || s.ReadyWorkers != 1 || s.FailedWorkers != 1 || s.QueuedOperations != 3 || !s.LifecycleError {
		t.Fatalf("snapshot %#v", s)
	}
	data, _ := json.Marshal(s)
	var keys map[string]any
	_ = json.Unmarshal(data, &keys)
	allowed := strings.Fields("observedAt availability state stale browserEnabled acpEnabled companyRequiredEdgePolicies owners leases activeLeases expiredLeases releasingLeases failedLeases unownedLeases workers readyWorkers failedWorkers activeOperations queuedOperations maxConcurrency queueCapacity managedOrphans externalOrphans lifecycleError")
	if len(keys) != len(allowed) {
		t.Fatalf("DTO keys changed: %s", data)
	}
	for _, key := range allowed {
		if _, ok := keys[key]; !ok {
			t.Fatalf("missing allowlisted key %s", key)
		}
	}
	for _, forbidden := range []string{"PRIVATE_CANARY", "99123", "pid", "page_id", "lease_id", "workspace", "connector", "token", "cleanup", "endpoint", "url", "payload", "error\""} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("unsafe %s", data)
		}
	}
}

type browserSnapshotSession struct {
	tools map[string]mcpclient.Tool
	calls int
}

func (s *browserSnapshotSession) Info() mcpclient.SessionInfo {
	return mcpclient.SessionInfo{ServerName: toolbrowser.ManagedEngineServerName, ServerVersion: toolbrowser.ManagedEngineVersion, ProtocolVersion: "2025-06-18"}
}
func (s *browserSnapshotSession) Tools() map[string]mcpclient.Tool { return s.tools }
func (s *browserSnapshotSession) Close() error                     { s.calls++; return nil }
func (s *browserSnapshotSession) Call(_ context.Context, name string, args map[string]any) (map[string]any, error) {
	s.calls++
	if name == "new_page" {
		return map[string]any{"structuredContent": map[string]any{"pages": []any{map[string]any{"id": float64(1), "isolatedContext": args["isolatedContext"]}}}}, nil
	}
	return map[string]any{}, nil
}

func TestRuntimeBrowserDesktopGenuineBrokerReadDoesNotOperateWorker(t *testing.T) {
	raw, err := os.ReadFile("../tool/browser/testdata/managed-1.7.0-schema-projection.json")
	if err != nil {
		t.Fatal(err)
	}
	var tools []mcpclient.Tool
	if err := json.Unmarshal(raw, &tools); err != nil {
		t.Fatal(err)
	}
	session := &browserSnapshotSession{tools: map[string]mcpclient.Tool{}}
	for _, tool := range tools {
		session.tools[tool.Name] = tool
	}
	catalog, _ := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	planner, _ := toolbrowser.NewRoutePlanner(nil, catalog, nil)
	registry := toolbrowser.NewWorkerRegistry(toolbrowser.WorkerDependencies{
		Probe: func(context.Context, mcpclient.ServerConfig) (string, error) {
			return toolbrowser.ManagedEngineVersion, nil
		},
		Open: func(context.Context, mcpclient.ServerConfig) (toolbrowser.WorkerSession, error) { return session, nil },
	})
	bridge, err := toolbrowser.NewACPBridge(planner, registry)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bridge.Close() }()
	r := &Runtime{cfg: config.Config{BrowserEnabled: true, ACPEnabled: true}, acpBrowser: bridge}
	if s := r.RuntimeBrowserDesktop()["snapshot"].(browserdesktop.Snapshot); s.State != "idle" || s.Availability != "available" {
		t.Fatal(s)
	}
	token, err := bridge.RegisterSession("PRIVATE_CANARY", "PRIVATE_CANARY", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.Acquire(context.Background(), token, "about:blank"); err != nil {
		t.Fatal(err)
	}
	before, calls := bridge.Diagnostics(), session.calls
	s := r.RuntimeBrowserDesktop()["snapshot"].(browserdesktop.Snapshot)
	if s.ActiveLeases != 1 || s.Owners != 1 || s.ReadyWorkers != 1 || s.State != "leases_present" || s.Stale {
		t.Fatalf("snapshot %#v", s)
	}
	if !reflect.DeepEqual(before, bridge.Diagnostics()) || calls != session.calls {
		t.Fatal("passive snapshot changed broker or operated worker")
	}
}

func TestRuntimeBrowserDesktopCompanyEdgeIsOnlyIntent(t *testing.T) {
	r := &Runtime{cfg: config.Config{BrowserEnabled: true, ACPEnabled: true, BrowserWorkspacePolicies: []browserpolicy.WorkspaceRootPolicy{{Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "PRIVATE_CANARY", RequiredProfileID: "PRIVATE_CANARY"}}}}}
	s := r.RuntimeBrowserDesktop()["snapshot"].(browserdesktop.Snapshot)
	if s.CompanyRequiredEdgePolicies != 1 || s.Availability != "broker_unavailable" || s.ActiveLeases != 0 || s.ReadyWorkers != 0 {
		t.Fatal(s)
	}
	data, _ := json.Marshal(s)
	if strings.Contains(string(data), "PRIVATE_CANARY") {
		t.Fatal("route identifiers leaked")
	}
}
