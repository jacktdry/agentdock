package desktopruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/uvwt/agentdock/internal/fs/atomicfile"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTunnelTransactionRollbackAndRecovery(t *testing.T) {
	for _, failedRestore := range []bool{false, true} {
		t.Run(tunnelBool(failedRestore), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "private-config")
			if err := atomicfile.Write(path, []byte("before"), 0o600); err != nil {
				t.Fatal(err)
			}
			snapshots, err := captureTunnelFiles(path)
			if err != nil {
				t.Fatal(err)
			}
			err = runTunnelTransactionLocked(context.Background(), root, snapshots, func(context.Context) error {
				marker, err := os.ReadFile(filepath.Join(root, tunnelRecoveryFile))
				if err != nil || !json.Valid(marker) {
					t.Fatalf("invalid recovery marker: %v", err)
				}
				info, err := os.Stat(filepath.Join(root, tunnelRecoveryFile))
				if err != nil {
					t.Fatal(err)
				}
				assertTunnelPrivateMode(t, info)
				if err := atomicfile.Write(path, []byte("after"), 0o600); err != nil {
					return err
				}
				return errors.New("private failure must not escape")
			}, func(context.Context) error {
				if failedRestore {
					return errors.New("private recovery failure")
				}
				return nil
			})
			var outcome *TunnelMutationError
			if !errors.As(err, &outcome) {
				t.Fatalf("missing structured rollback outcome: %v", err)
			}
			if failedRestore {
				if !errors.Is(err, ErrTunnelRecoveryRequired) || !tunnelRecoveryPending(root) {
					t.Fatal("rollback failure hidden")
				}
			} else if outcome.Phase != "rolled_back" || tunnelRecoveryPending(root) {
				t.Fatalf("rollback: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "before" {
				t.Fatal("original local bytes not restored")
			}
		})
	}
}

func TestTunnelExistingOrMalformedRecoveryMarkerFailsClosed(t *testing.T) {
	for _, content := range []string{"{\"phase\":\"applying\"}", "{bad", ""} {
		t.Run(content, func(t *testing.T) {
			root := t.TempDir()
			if err := atomicfile.Write(filepath.Join(root, tunnelRecoveryFile), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			called := false
			err := runTunnelTransactionLocked(context.Background(), root, nil, func(context.Context) error { called = true; return nil }, func(context.Context) error { return nil })
			if !errors.Is(err, ErrTunnelRecoveryRequired) || called {
				t.Fatal("existing marker permitted mutation")
			}
			status := completeTunnelStatus(TunnelStatus{Mode: "named", Running: true, Ready: true, PublicURL: "https://next.example"}, root, "stored")
			if !status.Observation.RecoveryRequired || status.Ready || status.PublicURL != "" {
				t.Fatal("recovery marker did not fail closed in status")
			}
		})
	}
}

func TestTunnelNamedConnectionNeverVerifiesPublicRoute(t *testing.T) {
	status := completeTunnelStatus(TunnelStatus{Mode: "named", Running: true, Ready: true, PublicURL: "https://next.example"}, t.TempDir(), "stored")
	connected := true
	status.Observation.Connected = &connected
	if status.Ready || status.Observation.PublicEndpoint != "unchecked" || status.Observation.RemoteRoute != "manual_route_required" {
		t.Fatal("Named transport connection misrepresented as public verification")
	}
}

func TestTunnelRollbackRemovesPreviouslyAbsentFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "new-config")
	snapshots, err := captureTunnelFiles(path)
	if err != nil {
		t.Fatal(err)
	}
	err = runTunnelTransactionLocked(context.Background(), root, snapshots, func(context.Context) error {
		if err := atomicfile.Write(path, []byte("new"), 0o600); err != nil {
			return err
		}
		return errors.New("fail")
	}, func(context.Context) error { return nil })
	var outcome *TunnelMutationError
	if !errors.As(err, &outcome) || outcome.Phase != "rolled_back" {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rollback created previously absent configuration")
	}
}

func TestTunnelLogsRedactTokenAcrossWrites(t *testing.T) {
	var output bytes.Buffer
	writer := &tunnelSafeLogWriter{output: &output, token: "secret-fixture-token"}
	for _, data := range []string{"connected secret-", "fixture-token", " complete\n"} {
		if _, err := writer.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(output.String(), "secret") || !strings.Contains(output.String(), "[redacted]") {
		t.Fatal("token entered logs")
	}
}
