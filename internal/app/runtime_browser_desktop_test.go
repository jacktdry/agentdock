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
		if s.Availability != test.want || s.State != "unavailable" || s.Leases != 0 || s.ObservedAt == "" || s.ManagedPoolMode != "unknown" || s.SharedPoolBudget != nil {
			t.Fatalf("snapshot %#v", s)
		}
	}
}

func TestRuntimeBrowserDesktopAuthenticatedDirectLeaseIsOwned(t *testing.T) {
	now := time.Now().UTC()
	d := toolbrowser.BrokerDiagnostics{Leases: []toolbrowser.BrokerLeaseDiagnostic{{Route: browserpolicy.RouteRequiredExternal,
		AuthenticatedDirectOwner: true, CleanupState: toolbrowser.CleanupPending, ExpiresAt: now.Add(time.Minute)}}}
	s := aggregateBrowserDiagnostics(browserdesktop.Snapshot{Availability: "available"}, d, now)
	if s.Stale || s.UnownedLeases != 0 || s.ActiveLeases != 1 || s.RequiredExternalLeases != 1 {
		t.Fatal("authenticated direct lease falsely reported stale/unowned", s)
	}
	d.Leases[0].AuthenticatedDirectOwner = false
	s = aggregateBrowserDiagnostics(browserdesktop.Snapshot{Availability: "available"}, d, now)
	if !s.Stale || s.UnownedLeases != 1 || s.ActiveLeases != 0 {
		t.Fatal("missing owner evidence treated as active", s)
	}
}

func TestRuntimeBrowserDesktopCompletedLeaseDoesNotReportStale(t *testing.T) {
	now := time.Now().UTC()
	for _, test := range []struct {
		name         string
		failedLease  bool
		failedWorker bool
	}{
		{name: "completed"},
		{name: "failed lease", failedLease: true},
		{name: "failed worker", failedWorker: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := toolbrowser.BrokerDiagnostics{
				Leases:  []toolbrowser.BrokerLeaseDiagnostic{{Route: browserpolicy.RouteManaged, PageID: "PRIVATE_CANARY", CleanupState: toolbrowser.CleanupComplete, CleanupError: "PRIVATE_CANARY", ExpiresAt: now.Add(-time.Minute)}},
				Workers: []toolbrowser.BrokerWorkerDiagnostic{{State: toolbrowser.WorkerStopped, Error: "PRIVATE_CANARY"}},
			}
			if test.failedLease {
				d.Leases = append(d.Leases, toolbrowser.BrokerLeaseDiagnostic{Route: browserpolicy.RouteManaged, ACPSessionID: "PRIVATE_CANARY", CleanupState: toolbrowser.CleanupFailed, CleanupError: "PRIVATE_CANARY"})
			}
			if test.failedWorker {
				d.Workers = append(d.Workers, toolbrowser.BrokerWorkerDiagnostic{State: toolbrowser.WorkerFailed, Error: "PRIVATE_CANARY"})
			}
			s := aggregateBrowserDiagnostics(browserdesktop.Snapshot{Availability: "available"}, d, now)
			wantLeases, wantWorkers, wantState := 0, 0, "idle"
			if test.failedLease {
				wantLeases, wantState = 1, "stale"
			}
			if test.failedWorker {
				wantWorkers, wantState = 1, "stale"
			}
			if s.Availability != "available" || s.State != wantState || s.Stale != (test.failedLease || test.failedWorker) || s.Leases != wantLeases || s.ManagedLeases != wantLeases || s.RequiredExternalLeases != 0 || s.ExplicitExternalLeases != 0 || s.Workers != wantWorkers || s.FailedLeases != wantLeases || s.FailedWorkers != wantWorkers || s.ActiveLeases != 0 || s.ReleasingLeases != 0 || s.ReadyWorkers != 0 || s.UnownedLeases != 0 || s.ExpiredLeases != 0 {
				t.Fatalf("snapshot %#v", s)
			}
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "PRIVATE_CANARY") {
				t.Fatal("private diagnostics leaked")
			}
		})
	}
}

func TestRuntimeBrowserDesktopStaleProjectionIsAllowlisted(t *testing.T) {
	now := time.Now().UTC()
	d := toolbrowser.BrokerDiagnostics{
		AuthenticatedMCPRequestOwners: 1,
		Owners:                        []toolbrowser.BrokerOwnerDiagnostic{{ACPSessionID: "PRIVATE_CANARY", CanonicalWorkspaceRoot: "/PRIVATE_CANARY"}},
		Leases: []toolbrowser.BrokerLeaseDiagnostic{
			{Route: browserpolicy.RouteManaged, ACPSessionID: "PRIVATE_CANARY", PageID: "PRIVATE_CANARY", CleanupState: toolbrowser.CleanupPending, ExpiresAt: now.Add(time.Minute)},
			{Route: browserpolicy.RouteRequiredExternal, ACPSessionID: "PRIVATE_CANARY", CleanupState: toolbrowser.CleanupPending, ExpiresAt: now},
			{Route: browserpolicy.RouteExternal, CleanupState: toolbrowser.CleanupFailed, CleanupError: "PRIVATE_CANARY", ConnectorPID: 99123},
			{Route: browserpolicy.RouteRequiredExternal, ACPSessionID: "PRIVATE_CANARY", CleanupState: toolbrowser.CleanupReleasing},
		},
		Workers:            []toolbrowser.BrokerWorkerDiagnostic{{State: toolbrowser.WorkerReady, PID: 99123}, {State: toolbrowser.WorkerFailed, Error: "PRIVATE_CANARY"}},
		Queue:              toolbrowser.BrokerQueueDiagnostic{Active: 2, Queued: 3, MaxConcurrency: 4, QueueCapacity: 5, ManagedOrphans: 1, ExternalOrphans: 2},
		LifecycleLastError: "PRIVATE_CANARY",
		SharedBudget:       &toolbrowser.SharedManagedBudgetDiagnostic{CapacitySlots: 2, OccupiedSlots: 1, AvailableSlots: 1, ClaimedOwners: 3, UncertainSlots: 1},
	}
	s := aggregateBrowserDiagnostics(browserdesktop.Snapshot{Availability: "available"}, d, now)
	if s.AuthenticatedTransportOwners != 1 || s.State != "stale" || !s.Stale || s.ActiveLeases != 1 || s.ExpiredLeases != 1 || s.UnownedLeases != 1 || s.ReleasingLeases != 1 || s.FailedLeases != 1 || s.ReadyWorkers != 1 || s.FailedWorkers != 1 || s.QueuedOperations != 3 || !s.LifecycleError {
		t.Fatalf("snapshot %#v", s)
	}
	data, _ := json.Marshal(s)
	var keys map[string]any
	_ = json.Unmarshal(data, &keys)
	allowed := strings.Fields("managedPoolMode sharedPoolBudget configuredConnectors configuredAuthenticatedEdgeProfiles configuredRequiredExternalPolicies connectorHealth managedLeases requiredExternalLeases explicitExternalLeases observedAt availability state stale browserEnabled acpEnabled companyRequiredEdgePolicies owners authenticatedTransportOwners leases activeLeases expiredLeases releasingLeases failedLeases unownedLeases workers readyWorkers failedWorkers activeOperations queuedOperations maxConcurrency queueCapacity managedOrphans externalOrphans lifecycleError")
	if len(keys) != len(allowed) {
		t.Fatalf("DTO keys changed: %s", data)
	}
	for _, key := range allowed {
		if _, ok := keys[key]; !ok {
			t.Fatalf("missing allowlisted key %s", key)
		}
	}
	budget, ok := keys["sharedPoolBudget"].(map[string]any)
	if !ok || len(budget) != 5 {
		t.Fatalf("unsafe budget: %s", data)
	}
	for _, key := range []string{"capacity", "occupied", "available", "claimed", "uncertain"} {
		if _, ok := budget[key].(float64); !ok {
			t.Fatalf("non-numeric budget: %s", data)
		}
	}
	for _, forbidden := range []string{"PRIVATE_CANARY", "99123", "pid", "page_id", "lease_id", "workspace_id", "connector_id", "token", "cleanup", "endpoint", "url", "payload", "error\""} {
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
	if s := r.RuntimeBrowserDesktop()["snapshot"].(browserdesktop.Snapshot); s.State != "idle" || s.Availability != "available" || s.ManagedPoolMode != "exclusive" || s.SharedPoolBudget != nil {
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
	if s.ManagedLeases != 1 || s.RequiredExternalLeases != 0 || s.ExplicitExternalLeases != 0 || s.ConnectorHealth != "not_observed" || s.ActiveLeases != 1 || s.Owners != 1 || s.ReadyWorkers != 1 || s.State != "leases_present" || s.Stale {
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

// Definitions (including unused profiles) and all policy classes describe intent,
// independent of bridge availability; retained state never becomes live health.
func TestRuntimeBrowserDesktopConfiguredAndRetainedRoutes(t *testing.T) {
	cfg := config.Config{BrowserEnabled: true, ACPEnabled: true,
		BrowserCatalog: browserpolicy.CatalogDefinition{
			Profiles: []browserpolicy.ProfileDefinition{
				{ID: "private-edge", Browser: browserpolicy.BrowserEdge, Class: browserpolicy.ProfileAuthenticatedExternal, UserDataDir: "/private/profiles/edge"},
				{ID: "private-unused", Browser: browserpolicy.BrowserEdge, Class: browserpolicy.ProfileAuthenticatedExternal},
				{ID: "private-persistent", Browser: browserpolicy.BrowserEdge, Class: browserpolicy.ProfileExternalPersistent},
				{ID: "private-chrome", Browser: browserpolicy.BrowserChrome, Class: browserpolicy.ProfileAuthenticatedExternal},
			},
			Connectors: []browserpolicy.ConnectorDefinition{{ID: "private-connector", ProfileID: "private-edge", Driver: browserpolicy.DriverChromeDevToolsMCPWS, Endpoint: "ws://127.0.0.1:9222/devtools/browser"}},
		},
		BrowserWorkspacePolicies: []browserpolicy.WorkspaceRootPolicy{
			{Root: "/private/company", Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "private-connector", RequiredProfileID: "private-edge", AllowedOrigins: []string{"https://company.example.com"}}},
			{Root: "/private/default", Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceDefault, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "private-connector", RequiredProfileID: "private-edge"}},
			{Root: "/private/managed", Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceDefault, Route: browserpolicy.RouteManaged}},
		},
	}
	// Pure catalog/policy validation, deliberately no root canonicalization on read.
	catalog, err := browserpolicy.NewCatalog(cfg.BrowserCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := browserpolicy.ValidateWorkspacePolicies(cfg.BrowserWorkspacePolicies, catalog); err != nil {
		t.Fatal(err)
	}
	for _, flags := range [][2]bool{{true, true}, {false, true}, {true, false}, {false, false}} {
		cfg.BrowserEnabled, cfg.ACPEnabled = flags[0], flags[1]
		s := (&Runtime{cfg: cfg}).RuntimeBrowserDesktop()["snapshot"].(browserdesktop.Snapshot)
		if s.ConfiguredConnectors != 1 || s.ConfiguredAuthenticatedEdgeProfiles != 2 || s.ConfiguredRequiredExternalPolicies != 2 || s.CompanyRequiredEdgePolicies != 1 || s.ConnectorHealth != "not_observed" || s.Leases != 0 || s.ManagedLeases != 0 || s.RequiredExternalLeases != 0 || s.ExplicitExternalLeases != 0 {
			t.Fatal(s)
		}
		data, _ := json.Marshal(s)
		if strings.Contains(string(data), "private-") || strings.Contains(string(data), "/private/") || strings.Contains(string(data), "127.0.0.1") {
			t.Fatal(string(data))
		}
		s.Availability = "available"
		s = aggregateBrowserDiagnostics(s, toolbrowser.BrokerDiagnostics{Leases: []toolbrowser.BrokerLeaseDiagnostic{
			{Route: browserpolicy.RouteManaged, CleanupState: toolbrowser.CleanupFailed},
			{Route: browserpolicy.RouteRequiredExternal, CleanupState: toolbrowser.CleanupReleasing},
			{Route: browserpolicy.RouteRequiredExternal, ExpiresAt: time.Now().Add(-time.Hour)},
			{Route: browserpolicy.RouteExternal},
		}}, time.Now())
		if s.ManagedLeases != 1 || s.RequiredExternalLeases != 2 || s.ExplicitExternalLeases != 1 || s.Leases != 4 || s.ConnectorHealth != "not_observed" || s.ConfiguredRequiredExternalPolicies != 2 {
			t.Fatal(s)
		}
	}
}

func TestRuntimeBrowserDesktopInvalidDiagnosticsHideAllRetainedCounts(t *testing.T) {
	for _, d := range []toolbrowser.BrokerDiagnostics{
		{Leases: []toolbrowser.BrokerLeaseDiagnostic{{Route: browserpolicy.RouteManaged}, {Route: "PRIVATE_CANARY"}}},
		{Leases: []toolbrowser.BrokerLeaseDiagnostic{{Route: ""}}},
		{Leases: []toolbrowser.BrokerLeaseDiagnostic{{Route: "PRIVATE_CANARY", CleanupState: toolbrowser.CleanupComplete}}},
		{Queue: toolbrowser.BrokerQueueDiagnostic{Active: -1}},
		{AuthenticatedMCPRequestOwners: -1},
		{AuthenticatedMCPRequestOwners: 2, Owners: []toolbrowser.BrokerOwnerDiagnostic{{}}},
	} {
		s := aggregateBrowserDiagnostics(browserdesktop.Snapshot{Availability: "available", ConfiguredConnectors: 2}, d, time.Now())
		if s.Availability != "broker_unavailable" || s.State != "unavailable" || s.AuthenticatedTransportOwners != 0 || s.Owners != 0 || s.Leases != 0 || s.ManagedLeases != 0 || s.ConfiguredConnectors != 2 || s.ConnectorHealth != "not_observed" || s.ManagedPoolMode != "unknown" || s.SharedPoolBudget != nil {
			t.Fatal(s)
		}
	}
}

func TestRuntimeBrowserDesktopSharedPoolBookkeeping(t *testing.T) {
	valid := toolbrowser.SharedManagedBudgetDiagnostic{CapacitySlots: 2, OccupiedSlots: 1, AvailableSlots: 1, ClaimedOwners: 3, UncertainSlots: 1}
	tests := []struct {
		name   string
		budget toolbrowser.SharedManagedBudgetDiagnostic
		valid  bool
	}{
		{"occupied", valid, true},
		{"idle claimed", toolbrowser.SharedManagedBudgetDiagnostic{CapacitySlots: 2, AvailableSlots: 2, ClaimedOwners: 1}, true},
		{"uncertain without owner", toolbrowser.SharedManagedBudgetDiagnostic{CapacitySlots: 1, OccupiedSlots: 1, UncertainSlots: 1}, true},
		{"zero capacity", toolbrowser.SharedManagedBudgetDiagnostic{}, false},
	}
	for _, field := range []string{"capacity", "occupied", "available", "claimed", "uncertain", "over capacity", "over uncertain", "sum"} {
		b := valid
		switch field {
		case "capacity":
			b.CapacitySlots = -1
		case "occupied":
			b.OccupiedSlots = -1
		case "available":
			b.AvailableSlots = -1
		case "claimed":
			b.ClaimedOwners = -1
		case "uncertain":
			b.UncertainSlots = -1
		case "over capacity":
			b.OccupiedSlots = 3
		case "over uncertain":
			b.UncertainSlots = 2
		case "sum":
			b.AvailableSlots = 2
		}
		tests = append(tests, struct {
			name   string
			budget toolbrowser.SharedManagedBudgetDiagnostic
			valid  bool
		}{field, b, false})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := toolbrowser.BrokerDiagnostics{SharedBudget: &tt.budget, Leases: []toolbrowser.BrokerLeaseDiagnostic{{Route: browserpolicy.RouteManaged, ACPSessionID: "PRIVATE_CANARY"}}}
			s := aggregateBrowserDiagnostics(browserdesktop.Snapshot{Availability: "available"}, d, time.Now())
			if tt.valid {
				wantState := "leases_present"
				if tt.budget.UncertainSlots > 0 {
					wantState = "stale"
				}
				if s.ManagedPoolMode != "experimental_shared" || s.SharedPoolBudget == nil || s.ManagedLeases != 1 || s.State != wantState {
					t.Fatal(s)
				}
			} else if s.ManagedPoolMode != "unknown" || s.SharedPoolBudget != nil || s.State != "unavailable" || s.Availability != "broker_unavailable" || s.ManagedLeases != 0 || s.Leases != 0 {
				t.Fatal("invalid pool did not fail closed", s)
			}
			if tt.valid && *s.SharedPoolBudget != (browserdesktop.SharedPoolBudget{Capacity: tt.budget.CapacitySlots, Occupied: tt.budget.OccupiedSlots, Available: tt.budget.AvailableSlots, Claimed: tt.budget.ClaimedOwners, Uncertain: tt.budget.UncertainSlots}) {
				t.Fatal(s)
			}
			data, _ := json.Marshal(s)
			if !tt.valid && strings.Contains(string(data), "sharedPoolBudget") {
				t.Fatal("partial budget leaked")
			}
			d.Queue.Active = -1
			s = aggregateBrowserDiagnostics(browserdesktop.Snapshot{Availability: "available"}, d, time.Now())
			if s.ManagedPoolMode != "unknown" || s.SharedPoolBudget != nil {
				t.Fatal("unavailable broker exposed pool")
			}
		})
	}
}

func TestRuntimeBrowserDesktopUncertainPoolFlagsStaleWithoutLeases(t *testing.T) {
	d := toolbrowser.BrokerDiagnostics{SharedBudget: &toolbrowser.SharedManagedBudgetDiagnostic{
		CapacitySlots: 1, OccupiedSlots: 1, AvailableSlots: 0, ClaimedOwners: 1, UncertainSlots: 1,
	}}
	s := aggregateBrowserDiagnostics(browserdesktop.Snapshot{Availability: "available"}, d, time.Now())
	if s.ManagedPoolMode != "experimental_shared" || !s.Stale || s.State != "stale" || s.SharedPoolBudget == nil || s.SharedPoolBudget.Uncertain != 1 || s.Leases != 0 {
		t.Fatal("uncertain pool was misreported as idle", s)
	}
}

// Reading shared mode must not create a worker, even for an idle claimed pool.
func TestRuntimeBrowserDesktopSharedBridgeDoesNotStartWorker(t *testing.T) {
	catalog, _ := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	planner, _ := toolbrowser.NewRoutePlanner(nil, catalog, nil)
	registry := toolbrowser.NewWorkerRegistry(toolbrowser.WorkerDependencies{
		Probe: func(context.Context, mcpclient.ServerConfig) (string, error) {
			t.Fatal("snapshot probed worker")
			return "", nil
		},
		Open: func(context.Context, mcpclient.ServerConfig) (toolbrowser.WorkerSession, error) {
			t.Fatal("snapshot opened worker")
			return nil, nil
		},
	})
	bridge, err := toolbrowser.NewNextSingleWorkspaceSharedACPBridge(planner, registry, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := bridge.Close(); err != nil {
			t.Error(err)
		}
	}()
	r := &Runtime{cfg: config.Config{BrowserEnabled: true, ACPEnabled: true}, acpBrowser: bridge}
	before := bridge.Diagnostics()
	s := r.RuntimeBrowserDesktop()["snapshot"].(browserdesktop.Snapshot)
	if s.ManagedPoolMode != "experimental_shared" || s.SharedPoolBudget == nil || *s.SharedPoolBudget != (browserdesktop.SharedPoolBudget{Capacity: 1, Available: 1, Claimed: 1}) || s.Workers != 0 || s.State != "idle" {
		t.Fatal(s)
	}
	if !reflect.DeepEqual(before, bridge.Diagnostics()) {
		t.Fatal("snapshot mutated shared bridge")
	}
}
