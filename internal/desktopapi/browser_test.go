package desktopapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/browserdesktop"
)

func browserTestSnapshot() browserdesktop.Snapshot {
	return browserdesktop.Snapshot{ObservedAt: "2026-10-08T00:00:00Z", Availability: "available", State: "leases_present", ActiveLeases: 2}
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
