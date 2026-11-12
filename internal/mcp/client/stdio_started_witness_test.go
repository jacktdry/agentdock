package client

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestEphemeralStartedWitnessBeforeHandshake(t *testing.T) {
	cfg := ServerConfig{Name: "spawned", Transport: TransportStdio, Command: os.Args[0],
		Args:      []string{"-test.run=TestMCPStdioHelperProcess"},
		StaticEnv: map[string]string{"GO_WANT_MCP_HELPER": "1"}}
	var observed int
	s, err := OpenStdioSessionWithStartedWitness(context.Background(), cfg, func(ctx context.Context, pid int) error {
		if ctx == nil || ctx.Err() != nil || pid <= 0 {
			return errors.New("invalid launched child")
		}
		observed = pid
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if observed == 0 || observed != s.Info().PID {
		t.Fatalf("spawn hook was not invoked before handshake: spawn=%d session=%+v", observed, s.Info())
	}
}

func TestEphemeralStartedWitnessFailureDeniesSession(t *testing.T) {
	cfg := ServerConfig{Name: "denied-spawn", Transport: TransportStdio, Command: os.Args[0],
		Args:      []string{"-test.run=TestMCPStdioHelperProcess"},
		StaticEnv: map[string]string{"GO_WANT_MCP_HELPER": "1"},
		TimeoutMS: 1000}
	var pid int
	started := time.Now()
	s, err := OpenStdioSessionWithStartedWitness(context.Background(), cfg, func(_ context.Context, childPID int) error {
		pid = childPID
		return errors.New("durable spawn attestation rejected")
	})
	if err == nil || s != nil || pid <= 0 {
		t.Fatalf("unattested transport published session: %v %+v pid=%d", err, s, pid)
	}
	if time.Since(started) > 8*time.Second {
		t.Fatalf("failed spawn witness cleanup exceeded bounded cleanup: %s", time.Since(started))
	}
	// Do not attach to/kill a PID after this point: cleanup owns its child and
	// process control happens only inside the existing stdio lifecycle.
}

func TestEphemeralLegacyPathNeverCallsStartedWitness(t *testing.T) {
	cfg := ServerConfig{Name: "legacy", Transport: TransportStdio, Command: os.Args[0],
		Args:      []string{"-test.run=TestMCPStdioHelperProcess"},
		StaticEnv: map[string]string{"GO_WANT_MCP_HELPER": "1"}}
	s, err := OpenStdioSession(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Info().PID <= 0 {
		t.Fatal("legacy ephemeral session did not start")
	}
}
