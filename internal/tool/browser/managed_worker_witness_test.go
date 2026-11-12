package browser

import (
	"context"
	"errors"
	"strings"
	"testing"

	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

func TestManagedWorkerReadyWitnessGatesLeasePublication(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail bool
	}{
		{"accept", false},
		{"durability-failure", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := fakeManagedSession(t, tc.name)
			calls := 0
			registry := NewWorkerRegistry(WorkerDependencies{
				Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return ManagedEngineVersion, nil },
				Open:  func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) { return session, nil },
				ReadyWitness: func(info WorkerInfo) error {
					calls++
					if info.State != WorkerReady || info.Session.PID != 123 {
						t.Errorf("witness got unvalidated info: %+v", info)
					}
					if tc.fail {
						return errors.New("witness-canary-no-lease")
					}
					return nil
				},
			})
			info, err := registry.StartManaged(context.Background(), ManagedWorkerOptions{Cwd: t.TempDir()})
			if calls != 1 {
				t.Fatalf("witness calls=%d", calls)
			}
			if tc.fail {
				if err == nil || !strings.Contains(err.Error(), "witness-canary-no-lease") || info.State != WorkerFailed {
					t.Fatalf("worker admitted without witness: %+v %v", info, err)
				}
				if !session.closed {
					t.Fatal("unwitnessed child session was not closed")
				}
				if _, err := registry.Call(context.Background(), info.WorkerID, "list_pages", nil); err == nil {
					t.Fatal("unwitnessed worker accepted a tool call")
				}
			} else if err != nil || info.State != WorkerReady || session.closed {
				t.Fatalf("witness success broken: %+v %v", info, err)
			}
			if err := registry.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestManagedWorkerReadyWitnessNotCalledOnFailedVersion(t *testing.T) {
	calls := 0
	registry := NewWorkerRegistry(WorkerDependencies{
		Probe:        func(context.Context, mcpclient.ServerConfig) (string, error) { return "incompatible", nil },
		ReadyWitness: func(WorkerInfo) error { calls++; return nil },
	})
	if _, err := registry.StartManaged(context.Background(), ManagedWorkerOptions{Cwd: t.TempDir()}); err == nil {
		t.Fatal("wrong managed version accepted")
	}
	if calls != 0 {
		t.Fatal("witness called for unverified worker")
	}
	if err := registry.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
