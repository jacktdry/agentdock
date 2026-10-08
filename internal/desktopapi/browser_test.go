package desktopapi

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/browserdesktop"
)

func browserTestSnapshot() browserdesktop.Snapshot {
	return browserdesktop.Snapshot{ObservedAt: "2026-10-08T00:00:00Z", Availability: "available", State: "leases_present", ConnectorHealth: "not_observed", Leases: 2, ManagedLeases: 2, ActiveLeases: 2}
}

func TestBrowserServiceSnapshotVerifiedReaderOnly(t *testing.T) {
	s := NewBrowserService(t.TempDir())
	called := 0
	s.read = func(_ context.Context, root string) (browserdesktop.Snapshot, error) {
		called++
		if root != s.runtimeRoot {
			t.Fatal("untrusted root substitution")
		}
		return browserTestSnapshot(), nil
	}
	result := s.Snapshot(context.Background())
	data, _ := json.Marshal(result)
	if called != 1 || result.Error != nil || result.Snapshot.ActiveLeases != 2 || strings.Contains(string(data), "pid") || strings.Contains(string(data), "token") {
		t.Fatalf("result %s", data)
	}
	for _, capability := range DefaultManifest().Capabilities {
		if capability.Domain == DomainBrowser && (capability.Availability != AvailabilityAvailable || len(capability.Operations) != 1 || capability.Operations[0].Name != "snapshot" || capability.Operations[0].Access != AccessRead) {
			t.Fatal(capability)
		}
	}
}

func TestBrowserServiceFailClosedAndRedactsTransportErrors(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{err: context.DeadlineExceeded, code: "BROWSER_TIMEOUT"},
		{err: errors.New("PRIVATE_CANARY token=secret /private/path"), code: "BROWSER_CORE_UNAVAILABLE"},
		{err: context.Canceled, code: "BROWSER_CORE_UNAVAILABLE"},
	} {
		s := NewBrowserService(t.TempDir())
		s.read = func(context.Context, string) (browserdesktop.Snapshot, error) {
			return browserdesktop.Snapshot{}, test.err
		}
		result := s.Snapshot(context.Background())
		data, _ := json.Marshal(result)
		if result.Error == nil || result.Error.Code != test.code || result.Snapshot.ObservedAt != "" || result.Snapshot.Availability != "core_unavailable" || strings.Contains(string(data), "PRIVATE_CANARY") || strings.Contains(string(data), "token=secret") {
			t.Fatalf("error leaked or unhandled: %s", data)
		}
	}
}

func TestBrowserServiceInvalidSnapshotRejected(t *testing.T) {
	for _, input := range []browserdesktop.Snapshot{
		{},
		{ObservedAt: "not-a-time", Availability: "available", State: "idle"},
		{ObservedAt: "2026-10-08T00:00:00Z", Availability: "PRIVATE_CANARY", State: "idle"},
		{ObservedAt: "2026-10-08T00:00:00Z", Availability: "available", State: "idle", Owners: -1},
	} {
		s := NewBrowserService(t.TempDir())
		s.read = func(context.Context, string) (browserdesktop.Snapshot, error) { return input, nil }
		result := s.Snapshot(context.Background())
		data, _ := json.Marshal(result)
		if result.Error == nil || result.Error.Code != "BROWSER_RESPONSE_INVALID" || strings.Contains(string(data), "PRIVATE_CANARY") {
			t.Fatalf("invalid snapshot accepted: %s", data)
		}
	}
}

func TestBrowserServiceUntrustedNextRootRejected(t *testing.T) {
	for _, marker := range []string{"stable", "next"} {
		t.Setenv("AGENTDOCK_DESKTOP_VARIANT", marker)
		s := NewBrowserService(t.TempDir())
		// Keep the real verified reader: no fixture or dummy TCP listener.
		result := s.Snapshot(context.Background())
		if result.Error == nil || result.Snapshot.Availability != "core_unavailable" {
			t.Fatal(result)
		}
	}
}

func TestBrowserServiceCanceledContextSkipsPeerCall(t *testing.T) {
	s := NewBrowserService(t.TempDir())
	s.read = func(context.Context, string) (browserdesktop.Snapshot, error) {
		t.Fatal("canceled peer call")
		return browserdesktop.Snapshot{}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s.Snapshot(ctx).Error == nil {
		t.Fatal("canceled request accepted")
	}
}

func TestBrowserServiceAggregateValidation(t *testing.T) {
	for _, field := range []string{"ConfiguredConnectors", "ConfiguredAuthenticatedEdgeProfiles", "ConfiguredRequiredExternalPolicies", "ManagedLeases", "RequiredExternalLeases", "ExplicitExternalLeases", "CompanyRequiredEdgePolicies", "Owners", "Leases", "ActiveLeases", "ExpiredLeases", "ReleasingLeases", "FailedLeases", "UnownedLeases", "Workers", "ReadyWorkers", "FailedWorkers", "ActiveOperations", "QueuedOperations", "MaxConcurrency", "QueueCapacity", "ManagedOrphans", "ExternalOrphans"} {
		t.Run(field, func(t *testing.T) {
			s := browserTestSnapshot()
			reflect.ValueOf(&s).Elem().FieldByName(field).SetInt(-1)
			if validBrowserSnapshot(s) {
				t.Fatal("negative aggregate accepted")
			}
		})
	}
	for _, health := range []string{"", "healthy", "authenticated", "reachable", "PRIVATE_CANARY"} {
		s := browserTestSnapshot()
		s.ConnectorHealth = health
		if validBrowserSnapshot(s) {
			t.Fatal("live or unknown health accepted")
		}
	}
	for _, mutate := range []func(*browserdesktop.Snapshot){
		func(s *browserdesktop.Snapshot) { s.ManagedLeases = 1 },
		func(s *browserdesktop.Snapshot) { s.RequiredExternalLeases = 1 },
		func(s *browserdesktop.Snapshot) { s.ExplicitExternalLeases = int(^uint(0) >> 1) },
		func(s *browserdesktop.Snapshot) { s.ActiveLeases = 3 },
		func(s *browserdesktop.Snapshot) { s.ReleasingLeases = 1 },
		func(s *browserdesktop.Snapshot) { s.State = "idle" },
		func(s *browserdesktop.Snapshot) { s.Stale = true },
		func(s *browserdesktop.Snapshot) { s.ExpiredLeases = 3 },
		func(s *browserdesktop.Snapshot) { s.ReadyWorkers = 1 },
		func(s *browserdesktop.Snapshot) { s.CompanyRequiredEdgePolicies = 1 },
		func(s *browserdesktop.Snapshot) { s.Availability = "broker_unavailable"; s.State = "unavailable" },
	} {
		s := browserTestSnapshot()
		mutate(&s)
		service := NewBrowserService(t.TempDir())
		service.read = func(context.Context, string) (browserdesktop.Snapshot, error) { return s, nil }
		result := service.Snapshot(context.Background())
		if result.Error == nil || result.Error.Code != "BROWSER_RESPONSE_INVALID" || result.Snapshot.Leases != 0 || result.Snapshot.ConfiguredConnectors != 0 {
			t.Fatal(result)
		}
	}
	for _, availability := range []string{"available", "browser_disabled", "acp_disabled", "broker_unavailable"} {
		s := browserdesktop.Snapshot{ObservedAt: "2026-10-08T00:00:00Z", Availability: availability, State: "unavailable", ConnectorHealth: "not_observed", ConfiguredConnectors: 2, ConfiguredAuthenticatedEdgeProfiles: 3, ConfiguredRequiredExternalPolicies: 2, CompanyRequiredEdgePolicies: 1}
		if availability == "available" {
			s.State = "idle"
		}
		if !validBrowserSnapshot(s) {
			t.Fatal("valid configured intent rejected", s)
		}
	}
}

func TestBrowserServiceRetainedRoutesAreNotHealth(t *testing.T) {
	s := browserTestSnapshot()
	s.Leases, s.ManagedLeases, s.RequiredExternalLeases, s.ExplicitExternalLeases = 6, 1, 2, 3
	s.ActiveLeases, s.ReleasingLeases, s.FailedLeases, s.ExpiredLeases, s.UnownedLeases = 1, 2, 3, 4, 5
	s.State, s.Stale = "stale", true
	s.Workers, s.ReadyWorkers, s.FailedWorkers = 2, 1, 1
	if !validBrowserSnapshot(s) {
		t.Fatal("overlapping anomalies or retained routes rejected", s)
	}
	service := NewBrowserService(t.TempDir())
	service.read = func(context.Context, string) (browserdesktop.Snapshot, error) { return s, nil }
	r := service.Snapshot(context.Background())
	if r.Error != nil || r.Snapshot.ConnectorHealth != "not_observed" || r.Snapshot.RequiredExternalLeases != 2 {
		t.Fatal(r)
	}
}
