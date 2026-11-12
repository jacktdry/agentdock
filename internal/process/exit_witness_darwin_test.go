//go:build darwin

package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestExitWitnessOwnedFixtureHelper(t *testing.T) {
	if os.Getenv("AGENTDOCK_TEST_EXIT_WITNESS") != "1" {
		return
	}
	fmt.Println("EXIT_WITNESS_READY")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func startExitWitnessFixture(t *testing.T) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestExitWitnessOwnedFixtureHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AGENTDOCK_TEST_EXIT_WITNESS=1")
	Configure(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Wait() })
	ready := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		ready <- scanner.Scan() && scanner.Text() == "EXIT_WITNESS_READY"
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("fixture did not start")
		}
	case <-time.After(3 * time.Second):
		_ = stdin.Close()
		t.Fatal("fixture startup timed out")
	}
	return cmd, stdin
}

func TestExitWitnessActualKernelNOTEEXIT(t *testing.T) {
	cmd, stdin := startExitWitnessFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	id, err := ObserveDirectChildBirth(ctx, os.Getpid(), cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	watch, err := WatchVerifiedExit(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := watch.Wait(ctx); err != nil {
		t.Fatalf("missing kernel NOTE_EXIT: %v", err)
	}
}

func TestExitWitnessDeniedReusedIdentity(t *testing.T) {
	cmd, _ := startExitWitnessFixture(t)
	ctx := context.Background()
	id, err := ObserveDirectChildBirth(ctx, os.Getpid(), cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	id.StartUsec = (id.StartUsec + 1) % 1000000
	if _, err := WatchVerifiedExit(ctx, id); !errors.Is(err, ErrExitWitnessUnavailable) {
		t.Fatalf("mismatched kernel birth granted witness: %v", err)
	}
	id.PID = os.Getpid()
	if _, err := WatchVerifiedExit(ctx, id); err == nil {
		t.Fatal("wrong PID admitted")
	}
}

func TestExitWitnessCancellationAndClose(t *testing.T) {
	cmd, _ := startExitWitnessFixture(t)
	id, err := ObserveDirectChildBirth(context.Background(), os.Getpid(), cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	watch, err := WatchVerifiedExit(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := watch.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancel ignored: %v", err)
	}
	if err := watch.Close(); err != nil {
		t.Fatal(err)
	}
	if err := watch.Close(); err != nil {
		t.Fatal(err)
	}
}
