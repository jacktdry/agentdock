package browser

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

func TestManagedRegistryDefaultOpenStartedWitnessBeforeHandshake(t *testing.T) {
	var started atomic.Int32
	var ready atomic.Int32
	registry := NewWorkerRegistry(WorkerDependencies{
		Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return ManagedEngineVersion, nil },
		StartedWitness: func(_ context.Context, pid int) error {
			if pid <= 0 {
				return errors.New("missing stdio pid")
			}
			started.Add(1)
			return errors.New("deliberate durable spawn refusal")
		},
		ReadyWitness: func(WorkerInfo) error { ready.Add(1); return nil },
	})
	cfg := mcpclient.ServerConfig{Name: ManagedEngineServerName, Transport: mcpclient.TransportStdio,
		Command: os.Args[0], Args: []string{"-test.run=^TestManagedRegistryDefaultOpenStartedWitnessBeforeHandshake$"},
		TimeoutMS: 1000}
	info, err := registry.startWorker(context.Background(), cfg)
	if err == nil || info.State != WorkerFailed || started.Load() != 1 || ready.Load() != 0 {
		t.Fatalf("pre-handshake hook was bypassed: info=%+v err=%v started=%d ready=%d",
			info, err, started.Load(), ready.Load())
	}
	if err := registry.Shutdown(context.Background()); err != nil {
		t.Fatalf("failed-start worker cleanup: %v", err)
	}
}
