package desktopapi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
	"github.com/uvwt/agentdock/internal/nexusbridge"
)

func nexusFixtureIdentity(token string) nexusbridge.Identity {
	return nexusbridge.Identity{Version: 1, Endpoint: "https://nexus.example/hidden-path", NodeID: "node_1", DeviceID: "device_1", DeviceToken: token}
}

func nexusFixture(token string) []byte {
	data, _ := json.Marshal(nexusFixtureIdentity(token))
	return data
}

func nexusFixtureGeneration(token string) string {
	return nexusbridge.Generation(nexusFixtureIdentity(token))
}

func TestNexusSnapshotStatesAndRedaction(t *testing.T) {
	gen := nexusFixtureGeneration("SECRET")
	for _, tc := range []struct {
		name                string
		data                []byte
		readErr             error
		status              desktopruntime.ServiceStatus
		observationErr      error
		pairing, connection string
		restart             bool
	}{
		{"unpaired", nil, os.ErrNotExist, desktopruntime.ServiceStatus{Running: true, Healthy: true}, nil, "not_paired", "disconnected", false},
		{"offline", nexusFixture("SECRET"), nil, desktopruntime.ServiceStatus{Running: true, Healthy: true, NexusIdentityGeneration: gen}, nil, "paired", "disconnected", false},
		{"connected with attested generation", nexusFixture("SECRET"), nil, desktopruntime.ServiceStatus{Running: true, Healthy: true, NexusConnected: true, NexusIdentityGeneration: gen}, nil, "paired", "connected", false},
		{"active identity generation mismatch", nexusFixture("SECRET"), nil, desktopruntime.ServiceStatus{Running: true, Healthy: true, NexusConnected: true, NexusIdentityGeneration: nexusFixtureGeneration("OLD")}, nil, "paired", "unknown", true},
		{"observed stopped", nexusFixture("SECRET"), nil, desktopruntime.ServiceStatus{Running: false, Healthy: false}, nil, "paired", "disconnected", false},
		{"unhealthy", nexusFixture("SECRET"), nil, desktopruntime.ServiceStatus{Running: true, NexusConnected: true, NexusIdentityGeneration: gen}, nil, "paired", "unknown", false},
		{"unavailable observation", nexusFixture("SECRET"), nil, desktopruntime.ServiceStatus{}, errors.New("SECRET /private/path stderr"), "paired", "unknown", false},
		{"malformed", []byte(`SECRET /private/path`), nil, desktopruntime.ServiceStatus{}, nil, "invalid", "unknown", false},
		{"unreadable", nil, errors.New("SECRET /private/path"), desktopruntime.ServiceStatus{}, nil, "invalid", "unavailable", false},
		{"foreign", nil, desktopruntime.ErrNextIdentityUnavailable, desktopruntime.ServiceStatus{}, nil, "unknown", "unavailable", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observations := 0
			s := NewNexusService("/private/path")
			s.deps.readIdentity = func(context.Context, string) ([]byte, error) { return tc.data, tc.readErr }
			s.deps.observe = func(context.Context, string) (desktopruntime.ServiceStatus, error) {
				observations++
				return tc.status, tc.observationErr
			}
			r := s.Snapshot(context.Background())
			if r.PairingState != tc.pairing || r.ConnectionState != tc.connection || r.RestartRequired != tc.restart {
				t.Fatalf("%+v", r)
			}
			if r.ObservedAt.IsZero() {
				t.Fatal("missing observed time")
			}
			if r.PairingState == "paired" || r.PairingState == "not_paired" {
				if !r.Capabilities.CanPair || r.Capabilities.PairDisabledReason != "" {
					t.Fatalf("pair capability: %+v", r.Capabilities)
				}
			}
			if r.PairingState == "paired" && !r.Capabilities.CanReconcile {
				t.Fatalf("reconcile capability: %+v", r.Capabilities)
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
	if nexusFixtureGeneration("a") == nexusFixtureGeneration("b") {
		t.Fatal("token revision not fenced")
	}
	if nexusbridge.AbsentGeneration() == "" || nexusbridge.AbsentGeneration() == nexusFixtureGeneration("a") {
		t.Fatal("absent generation")
	}
	s := NewNexusService("fixture")
	data := nexusFixture("a")
	s.deps.readIdentity = func(context.Context, string) ([]byte, error) { return data, nil }
	s.deps.observe = func(context.Context, string) (desktopruntime.ServiceStatus, error) {
		return desktopruntime.ServiceStatus{Running: true, Healthy: true, NexusConnected: true, NexusIdentityGeneration: nexusFixtureGeneration("a")}, nil
	}
	a, b := s.Snapshot(context.Background()), s.Snapshot(context.Background())
	if a.Generation != b.Generation || a.Generation == "" {
		t.Fatal("unstable generation")
	}
	s.deps.observe = func(context.Context, string) (desktopruntime.ServiceStatus, error) {
		data = nexusFixture("b")
		return desktopruntime.ServiceStatus{Running: true, Healthy: true, NexusConnected: true, NexusIdentityGeneration: nexusFixtureGeneration("a")}, nil
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

func TestNexusPairGenerationConfirmationAndRestartOutcome(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	data := []byte(nil)
	readErr := error(os.ErrNotExist)
	pairCalls := 0
	restartCalls := 0
	observeGeneration := ""
	s := NewNexusService(root)
	s.deps = nexusDependencies{
		readIdentity: func(context.Context, string) ([]byte, error) { return data, readErr },
		resolveHome:  func(context.Context, string) (string, error) { return home, nil },
		pair: func(_ context.Context, gotHome string, _ nexusbridge.PairOptions, expected string, confirm bool) (nexusbridge.Identity, error) {
			pairCalls++
			if gotHome != home || expected != nexusbridge.AbsentGeneration() || confirm {
				t.Fatalf("pair args home=%q expected=%q confirm=%v", gotHome, expected, confirm)
			}
			identity := nexusFixtureIdentity("NEW_SECRET")
			data = nexusFixture("NEW_SECRET")
			readErr = nil
			observeGeneration = nexusbridge.Generation(identity)
			return identity, nil
		},
		restart: func(context.Context, string, string) error {
			restartCalls++
			return nil
		},
		observe: func(context.Context, string) (desktopruntime.ServiceStatus, error) {
			return desktopruntime.ServiceStatus{Running: true, Healthy: true, NexusIdentityGeneration: observeGeneration}, nil
		},
	}

	stale := s.Pair(context.Background(), NexusPairRequest{Endpoint: "https://nexus.example", Code: "SUPERSECRET", ExpectedGeneration: "stale"})
	if stale.Error == nil || stale.Error.Code != "nexus_generation_conflict" || pairCalls != 0 || restartCalls != 0 {
		t.Fatalf("stale result=%+v pair=%d restart=%d", stale, pairCalls, restartCalls)
	}
	result := s.Pair(context.Background(), NexusPairRequest{Endpoint: "https://nexus.example", Code: "SUPERSECRET", ExpectedGeneration: nexusbridge.AbsentGeneration()})
	if result.Error != nil || !result.Completed || !result.IdentitySaved || result.RestartRequired || result.ObservedGeneration != observeGeneration || pairCalls != 1 || restartCalls != 1 {
		t.Fatalf("result=%+v pair=%d restart=%d", result, pairCalls, restartCalls)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "SUPERSECRET") || strings.Contains(string(encoded), "NEW_SECRET") || strings.Contains(string(encoded), home) {
		t.Fatalf("mutation result leaked secret/path: %s", encoded)
	}

	current := nexusFixtureGeneration("NEW_SECRET")
	pairCalls = 0
	denied := s.Pair(context.Background(), NexusPairRequest{Endpoint: "https://nexus.example", Code: "NEWCODE", ExpectedGeneration: current, ConfirmReplace: false})
	if denied.Error == nil || denied.Error.Code != "nexus_replace_confirmation_required" || pairCalls != 0 {
		t.Fatalf("replace confirmation result=%+v pair=%d", denied, pairCalls)
	}
}

func TestNexusPairRestartFailureIsTruthfulAndRedacted(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	data := []byte(nil)
	readErr := error(os.ErrNotExist)
	s := NewNexusService(root)
	s.deps = nexusDependencies{
		readIdentity: func(context.Context, string) ([]byte, error) { return data, readErr },
		resolveHome:  func(context.Context, string) (string, error) { return home, nil },
		pair: func(context.Context, string, nexusbridge.PairOptions, string, bool) (nexusbridge.Identity, error) {
			identity := nexusFixtureIdentity("SAVED_SECRET")
			data, readErr = nexusFixture("SAVED_SECRET"), nil
			return identity, nil
		},
		restart: func(context.Context, string, string) error { return errors.New("SAVED_SECRET /private/path stderr") },
		observe: func(context.Context, string) (desktopruntime.ServiceStatus, error) {
			t.Fatal("observe called after failed restart")
			return desktopruntime.ServiceStatus{}, nil
		},
	}
	result := s.Pair(context.Background(), NexusPairRequest{Endpoint: "https://nexus.example", Code: "ONE_TIME_SECRET", ExpectedGeneration: nexusbridge.AbsentGeneration()})
	if result.Completed || !result.IdentitySaved || !result.RestartRequired || result.Error == nil || result.Error.Code != "nexus_restart_required" {
		t.Fatalf("result=%+v", result)
	}
	encoded, _ := json.Marshal(result)
	for _, secret := range []string{"SAVED_SECRET", "ONE_TIME_SECRET", "/private/path", "stderr"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("leaked %q: %s", secret, encoded)
		}
	}
}

func TestNexusDesktopMutationsRetainCLIPairLockThroughRestart(t *testing.T) {
	for _, action := range []string{"pair", "reconcile"} {
		t.Run(action, func(t *testing.T) {
			root, home := t.TempDir(), t.TempDir()
			identity := nexusFixtureIdentity("LOCKED_SECRET")
			data, readErr := []byte(nil), error(os.ErrNotExist)
			if action == "reconcile" {
				data, readErr = nexusFixture("LOCKED_SECRET"), nil
			}
			enteredRestart, resumeRestart := make(chan struct{}), make(chan struct{})
			s := NewNexusService(root)
			s.deps = nexusDependencies{
				readIdentity: func(context.Context, string) ([]byte, error) { return data, readErr },
				resolveHome:  func(context.Context, string) (string, error) { return home, nil },
				pair: func(context.Context, string, nexusbridge.PairOptions, string, bool) (nexusbridge.Identity, error) {
					data, readErr = nexusFixture("LOCKED_SECRET"), nil
					return identity, nil
				},
				restart: func(context.Context, string, string) error {
					close(enteredRestart)
					<-resumeRestart
					return nil
				},
				observe: func(context.Context, string) (desktopruntime.ServiceStatus, error) {
					return desktopruntime.ServiceStatus{Running: true, Healthy: true, NexusIdentityGeneration: nexusbridge.Generation(identity)}, nil
				},
			}
			resultCh := make(chan NexusMutationResult, 1)
			go func() {
				if action == "pair" {
					resultCh <- s.Pair(context.Background(), NexusPairRequest{Endpoint: "https://nexus.example", Code: "ONE_TIME", ExpectedGeneration: nexusbridge.AbsentGeneration()})
				} else {
					resultCh <- s.Reconcile(context.Background(), nexusbridge.Generation(identity))
				}
			}()
			select {
			case <-enteredRestart:
			case result := <-resultCh:
				t.Fatalf("mutation exited before restart: %+v", result)
			case <-time.After(5 * time.Second):
				t.Fatal("mutation did not reach restart")
			}
			waitCtx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			release, err := nexusbridge.AcquirePairLock(waitCtx, home)
			cancel()
			if err == nil {
				release()
				close(resumeRestart)
				t.Fatal("CLI pair lock became available while Desktop restart was in progress")
			}
			close(resumeRestart)
			select {
			case result := <-resultCh:
				if !result.Completed || result.Error != nil {
					t.Fatalf("mutation failed after protected restart: %+v", result)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("mutation never released identity lock")
			}
			release, err = nexusbridge.AcquirePairLock(context.Background(), home)
			if err != nil {
				t.Fatalf("identity lock still held after completion: %v", err)
			}
			release()
		})
	}
}

func TestNexusPairPostRenameFailureCannotClaimUnsaved(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	identity := nexusFixtureIdentity("POST_RENAME_SECRET")
	data := []byte(nil)
	readErr := error(os.ErrNotExist)
	restarts := 0
	s := NewNexusService(root)
	s.deps = nexusDependencies{
		readIdentity: func(context.Context, string) ([]byte, error) { return data, readErr },
		resolveHome:  func(context.Context, string) (string, error) { return home, nil },
		pair: func(context.Context, string, nexusbridge.PairOptions, string, bool) (nexusbridge.Identity, error) {
			data, readErr = nexusFixture("POST_RENAME_SECRET"), nil
			return identity, nexusbridge.ErrIdentityCommitUncertain
		},
		restart: func(context.Context, string, string) error {
			restarts++
			return nil
		},
	}
	result := s.Pair(context.Background(), NexusPairRequest{Endpoint: "https://nexus.example", Code: "ONE_TIME_SECRET", ExpectedGeneration: nexusbridge.AbsentGeneration()})
	if result.Completed || !result.IdentitySaved || !result.RestartRequired || result.ObservedGeneration != nexusbridge.Generation(identity) || result.Error == nil || result.Error.Code != "nexus_identity_durability_unverified" || restarts != 0 {
		t.Fatalf("post-commit durability failure misreported: %+v restarts=%d", result, restarts)
	}
	encoded, _ := json.Marshal(result)
	for _, secret := range []string{"POST_RENAME_SECRET", "ONE_TIME_SECRET", home} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("post-commit failure leaked %q: %s", secret, encoded)
		}
	}
}

func TestNexusReconcileRequiresMatchingGenerationAndAttestsCore(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	data := nexusFixture("SECRET")
	generation := nexusFixtureGeneration("SECRET")
	restarts := 0
	s := NewNexusService(root)
	s.deps = nexusDependencies{
		readIdentity: func(context.Context, string) ([]byte, error) { return data, nil },
		resolveHome:  func(context.Context, string) (string, error) { return home, nil },
		restart: func(context.Context, string, string) error {
			restarts++
			return nil
		},
		observe: func(context.Context, string) (desktopruntime.ServiceStatus, error) {
			return desktopruntime.ServiceStatus{Running: true, Healthy: true, NexusIdentityGeneration: generation}, nil
		},
		pair: func(context.Context, string, nexusbridge.PairOptions, string, bool) (nexusbridge.Identity, error) {
			t.Fatal("pair called during reconcile")
			return nexusbridge.Identity{}, nil
		},
	}
	conflict := s.Reconcile(context.Background(), "stale")
	if conflict.Error == nil || conflict.Error.Code != "nexus_generation_conflict" || restarts != 0 {
		t.Fatalf("conflict=%+v restarts=%d", conflict, restarts)
	}
	result := s.Reconcile(context.Background(), generation)
	if result.Error != nil || !result.Completed || !result.IdentitySaved || result.RestartRequired || result.ObservedGeneration != generation || restarts != 1 {
		t.Fatalf("result=%+v restarts=%d", result, restarts)
	}
}
