//go:build darwin

package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	processcontrol "github.com/uvwt/agentdock/internal/process"
)

func TestSharedBrowserLaunchParentOwnedFixtureHelper(t *testing.T) {
	if os.Getenv("AGENTDOCK_TEST_BROWSER_CHILD_PROOF") != "1" {
		return
	}
	fmt.Println("NEXT_OWNED_CHILD_READY")
	select {}
}

func TestSharedBrowserReadyWitnessRejectsUnownedPID(t *testing.T) {
	root := testPrivateNextState(t)
	guard, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	// The Core/test process exists, but is NOT its own child.
	if err := guard.recordManagedWorker(readyEvidence(os.Getpid())); !errors.Is(err, errSharedBrowserEvidence) {
		t.Fatalf("existing unrelated PID produced owned Worker proof: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, sharedBrowserRestartMarker))
	if err != nil || string(raw) != sharedBrowserEvidencePrefix {
		t.Fatalf("failed witness contaminated durable record: %q %v", raw, err)
	}
	if err := guard.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedBrowserReadyWitnessAcceptsOnlyOwnedFixture(t *testing.T) {
	// Only a fresh process running this test binary is launched. No npx,
	// Chrome, Edge, AGY or any unrelated program is started.
	root := testPrivateNextState(t)
	guard, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSharedBrowserLaunchParentOwnedFixtureHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AGENTDOCK_TEST_BROWSER_CHILD_PROOF=1")
	processcontrol.Configure(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	ready := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		ready <- scanner.Scan() && scanner.Text() == "NEXT_OWNED_CHILD_READY"
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("owned test fixture exited unexpectedly")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("owned test fixture startup timed out")
	}
	if err := guard.recordManagedWorker(readyEvidence(cmd.Process.Pid)); err != nil {
		t.Fatalf("Next shared worker rejected its own direct child: %v", err)
	}
	evidence, err := os.ReadFile(filepath.Join(root, sharedBrowserRestartMarker))
	if err != nil {
		t.Fatal(err)
	}
	workers, valid := parseSharedBrowserEvidence(string(evidence))
	if !valid || len(workers) != 1 || !workers[0].HasBirth ||
		workers[0].PID != cmd.Process.Pid || workers[0].PGID != cmd.Process.Pid {
		t.Fatalf("owned kernel birth identity not durably recorded")
	}
	if err := guard.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
}

// A separate ready fixture exits naturally on stdin EOF. The Next guard
// should observe NOTE_EXIT without sending a termination signal.
func TestSharedBrowserOwnedNaturalExitHelper(t *testing.T) {
	if os.Getenv("AGENTDOCK_TEST_BROWSER_NATURAL_EXIT") != "1" {
		return
	}
	fmt.Println("OWNED_NATURAL_EXIT_READY")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestSharedBrowserReadyWitnessObservesNaturalExit(t *testing.T) {
	root := testPrivateNextState(t)
	guard, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSharedBrowserOwnedNaturalExitHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AGENTDOCK_TEST_BROWSER_NATURAL_EXIT=1")
	processcontrol.Configure(cmd)
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
	defer func() { _ = stdin.Close(); _ = cmd.Wait() }()
	ready := make(chan bool, 1)
	go func() {
		scan := bufio.NewScanner(stdout)
		ready <- scan.Scan() && scan.Text() == "OWNED_NATURAL_EXIT_READY"
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("owned fixture not ready")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("owned fixture did not start")
	}
	if err := guard.recordManagedWorker(readyEvidence(cmd.Process.Pid)); err != nil {
		t.Fatal(err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	limit := time.Now().Add(3 * time.Second)
	for {
		guard.mu.Lock()
		observed, uncertain := guard.exitObserved, guard.exitUncertain
		guard.mu.Unlock()
		if uncertain {
			t.Fatal("owned child NOTE_EXIT became uncertain")
		}
		if observed == 1 {
			break
		}
		if time.Now().After(limit) {
			t.Fatal("owned child natural exit was not observed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := guard.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, sharedBrowserRestartMarker)); !os.IsNotExist(err) {
		t.Fatalf("clean test guard not released after explicit verified close: %v", err)
	}
}
