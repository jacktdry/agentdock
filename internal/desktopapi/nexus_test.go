package desktopapi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

func nexusFixture(token string) []byte {
	return []byte(`{"version":1,"endpoint":"https://nexus.example/hidden-path","node_id":"node_1","device_id":"device_1","device_token":"` + token + `"}`)
}

func TestNexusSnapshotStatesAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		name                string
		data                []byte
		readErr             error
		status              desktopruntime.ServiceStatus
		observationErr      error
		pairing, connection string
	}{
		{"unpaired", nil, os.ErrNotExist, desktopruntime.ServiceStatus{Running: true, Healthy: true}, nil, "not_paired", "disconnected"},
		{"offline", nexusFixture("SECRET"), nil, desktopruntime.ServiceStatus{Running: true, Healthy: true}, nil, "paired", "disconnected"},
		{"Core connected but identity generation unverified", nexusFixture("SECRET"), nil, desktopruntime.ServiceStatus{Running: true, Healthy: true, NexusConnected: true}, nil, "paired", "unknown"},
		{"observed stopped", nexusFixture("SECRET"), nil, desktopruntime.ServiceStatus{Running: false, Healthy: false}, nil, "paired", "disconnected"},
		{"unhealthy", nexusFixture("SECRET"), nil, desktopruntime.ServiceStatus{Running: true, NexusConnected: true}, nil, "paired", "unknown"},
		{"unavailable observation", nexusFixture("SECRET"), nil, desktopruntime.ServiceStatus{}, errors.New("SECRET /private/path stderr"), "paired", "unknown"},
		{"malformed", []byte(`SECRET /private/path`), nil, desktopruntime.ServiceStatus{}, nil, "invalid", "unknown"},
		{"unreadable", nil, errors.New("SECRET /private/path"), desktopruntime.ServiceStatus{}, nil, "invalid", "unavailable"},
		{"foreign", nil, desktopruntime.ErrNextIdentityUnavailable, desktopruntime.ServiceStatus{}, nil, "unknown", "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observations := 0
			s := NewNexusService("/private/path")
			s.deps = nexusDependencies{readIdentity: func(context.Context, string) ([]byte, error) { return tc.data, tc.readErr }, observe: func(context.Context, string) (desktopruntime.ServiceStatus, error) {
				observations++
				return tc.status, tc.observationErr
			}}
			r := s.Snapshot(context.Background())
			if r.PairingState != tc.pairing || r.ConnectionState != tc.connection {
				t.Fatalf("%+v", r)
			}
			if r.Capabilities.CanPair || r.Capabilities.CanReconcile || r.Capabilities.PairDisabledReason == "" || r.ObservedAt.IsZero() {
				t.Fatalf("invalid capabilities: %+v", r)
			}
			if r.PairingState == "invalid" || r.PairingState == "unknown" {
				if observations != 0 {
					t.Fatal("observed Core before identity authority")
				}
			}
			if r.PairingState == "paired" && (r.SafeOrigin != "https://nexus.example" || !r.DeviceTokenStored || r.NodeID != "node_1") {
				t.Fatal(r)
			}
			data, _ := json.Marshal(r)
			for _, secret := range []string{"SECRET", "/private/path", "hidden-path", "stderr", "device_token", "device_1"} {
				if strings.Contains(string(data), secret) {
					t.Fatalf("leaked %q: %s", secret, data)
				}
			}
		})
	}
}

func TestNexusGenerationTracksSecretRevisionAndRejectsStaleObservation(t *testing.T) {
	if nexusGeneration(nexusFixture("a"), false) == nexusGeneration(nexusFixture("b"), false) {
		t.Fatal("token revision not fenced")
	}
	if nexusGeneration(nil, true) != nexusGeneration(nil, true) || nexusGeneration(nil, true) == nexusGeneration(nil, false) {
		t.Fatal("absent generation")
	}
	s := NewNexusService("fixture")
	data := nexusFixture("a")
	s.deps.readIdentity = func(context.Context, string) ([]byte, error) { return data, nil }
	s.deps.observe = func(context.Context, string) (desktopruntime.ServiceStatus, error) {
		return desktopruntime.ServiceStatus{Running: true, Healthy: true, NexusConnected: true}, nil
	}
	a, b := s.Snapshot(context.Background()), s.Snapshot(context.Background())
	if a.Generation != b.Generation || a.Generation == "" {
		t.Fatal("unstable generation")
	}
	s.deps.observe = func(context.Context, string) (desktopruntime.ServiceStatus, error) {
		data = nexusFixture("b")
		return desktopruntime.ServiceStatus{Running: true, Healthy: true, NexusConnected: true}, nil
	}
	r := s.Snapshot(context.Background())
	if r.PairingState != "unknown" || r.ConnectionState != "unknown" || r.Generation != "" || r.SafeOrigin != "" || r.DeviceTokenStored {
		t.Fatalf("stale observation escaped: %+v", r)
	}
}

func TestNexusInvalidIdentityAndOrigin(t *testing.T) {
	for _, endpoint := range []string{"http://external.example", "file:///secret", "https://user:SECRET@example.com", "https://example.com?SECRET", "https://example.com#SECRET", "https://[fe80::1%25en0]", "https://example.com:99999"} {
		if safeNexusOrigin(endpoint) != "" {
			t.Fatal(endpoint)
		}
	}
	for _, endpoint := range []string{"https://example.com/private", "http://localhost:123/private", "http://127.0.0.1/private", "http://[::1]/private"} {
		if safeNexusOrigin(endpoint) == "" {
			t.Fatal(endpoint)
		}
	}
	for _, node := range []string{"", "/private/path", "node\nSECRET", strings.Repeat("a", 129)} {
		if safeNexusNodeID(node) {
			t.Fatal(node)
		}
	}
	for _, raw := range []string{`{"version":2}`, strings.Replace(string(nexusFixture("SECRET")), "node_1", "/private/path", 1), strings.Replace(string(nexusFixture("SECRET")), "https://nexus.example/hidden-path", "http://external.example", 1)} {
		s := NewNexusService("fixture")
		s.deps.readIdentity = func(context.Context, string) ([]byte, error) { return []byte(raw), nil }
		s.deps.observe = func(context.Context, string) (desktopruntime.ServiceStatus, error) {
			t.Fatal("invalid identity contacted Core")
			return desktopruntime.ServiceStatus{}, nil
		}
		if r := s.Snapshot(context.Background()); r.PairingState != "invalid" || r.DeviceTokenStored || r.SafeOrigin != "" {
			t.Fatal(r)
		}
	}
}

func TestNexusConstructorNeverUsesEnvironmentFallback(t *testing.T) {
	t.Setenv(runtimeRootEnv, "/private/renderer-path")
	if NewNexusService("").runtimeRoot != "" {
		t.Fatal("adopted process default")
	}
}

func TestNexusDefaultObservationDoesNotDialUnverifiedCore(t *testing.T) {
	s := NewNexusService("fixture")
	s.deps.readIdentity = func(context.Context, string) ([]byte, error) {
		return nexusFixture("SECRET"), nil
	}
	r := s.Snapshot(context.Background())
	if r.PairingState != "paired" || r.ConnectionState != "unknown" ||
		r.Error == nil || r.Error.Code != "nexus_observation_unavailable" {
		t.Fatalf("unexpected unverified control channel status: %+v", r)
	}
	data, _ := json.Marshal(r)
	for _, secret := range []string{"SECRET", "/private/path", "device_token"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("unverified Core status leaked %q", secret)
		}
	}
}
