//go:build darwin

package process

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIndependentObserverRoleHelper(t *testing.T) {
	switch os.Getenv("AGENTDOCK_TEST_INDEPENDENT_ROLE") {
	case "worker":
		release := os.Getenv("AGENTDOCK_TEST_INDEPENDENT_RELEASE")
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(release); err == nil {
				return
			}
			time.Sleep(15 * time.Millisecond)
		}
		t.Fatal("owned test worker timed out before release")
	case "core":
		worker := exec.Command(os.Args[0], "-test.run=^TestIndependentObserverRoleHelper$", "-test.count=1")
		worker.Env = append(os.Environ(), "AGENTDOCK_TEST_INDEPENDENT_ROLE=worker")
		worker.Stdout = io.Discard
		worker.Stderr = os.Stderr
		Configure(worker)
		if err := worker.Start(); err != nil {
			t.Fatal(err)
		}
		fmt.Printf("OWNED_WRAPPER_PID=%d\n", worker.Process.Pid)
		// Intentionally exit only after the parent test kills this fake Core.
		select {}
	case "observer":
		var request IndependentExitRequest
		if err := json.NewDecoder(io.LimitReader(os.Stdin, 4096)).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if err := WatchIndependentWorkerExit(context.Background(), request, func() error {
			_, err := fmt.Fprintln(os.Stdout, "OWNED_OBSERVER_ARMED")
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIndependentObserverReceiptNameValidation(t *testing.T) {
	for _, bad := range []string{"../observer-name.v1", "observer-../etc.v1", "observer-ABC.v1", "observer.v1", "observer-x.v1/", "observer-x.v2", "observer-" + strings.Repeat("a", 70) + ".v1"} {
		if validIndependentReceiptName(bad) {
			t.Fatalf("unsafe receipt path accepted %q", bad)
		}
	}
	if !validIndependentReceiptName("observer-isolated-run.v1") {
		t.Fatal("safe receipt refused")
	}
}

func TestIndependentObserverSurvivesCoreCrashUntilOwnedWorkerExit(t *testing.T) {
	// Creates only three separate copies of this Go TEST executable. The
	// observer has no process signal API and monitors no user browser.
	state := t.TempDir()
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(state, "allow-worker-natural-exit")
	core := exec.Command(os.Args[0], "-test.run=^TestIndependentObserverRoleHelper$", "-test.count=1")
	core.Env = append(os.Environ(),
		"AGENTDOCK_TEST_INDEPENDENT_ROLE=core",
		"AGENTDOCK_TEST_INDEPENDENT_RELEASE="+release)
	stdout, err := core.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Start(); err != nil {
		t.Fatal(err)
	}
	coreWaited := false
	t.Cleanup(func() {
		if !coreWaited {
			_ = core.Process.Kill()
			_ = core.Wait()
		}
	})
	scan := bufio.NewScanner(stdout)
	ready := make(chan string, 1)
	go func() {
		if scan.Scan() {
			ready <- scan.Text()
		} else {
			ready <- ""
		}
	}()
	var message string
	select {
	case message = <-ready:
	case <-time.After(4 * time.Second):
		t.Fatal("core fixture startup timeout")
	}
	pidText, ok := strings.CutPrefix(message, "OWNED_WRAPPER_PID=")
	if !ok {
		t.Fatalf("unexpected core handshake %q", message)
	}
	workerPID, err := strconv.Atoi(pidText)
	if err != nil || workerPID <= 0 {
		t.Fatal("invalid own worker pid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	coreBirth, err := ObserveBirthIdentity(ctx, core.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	workerBirth, err := ObserveDirectChildBirth(ctx, core.Process.Pid, workerPID)
	if err != nil {
		t.Fatal(err)
	}
	req := IndependentExitRequest{Core: coreBirth, Worker: workerBirth, StateDir: state, ReceiptName: "observer-isolated-run.v1"}

	guard := exec.Command(os.Args[0], "-test.run=^TestIndependentObserverRoleHelper$", "-test.count=1")
	guard.Env = append(os.Environ(), "AGENTDOCK_TEST_INDEPENDENT_ROLE=observer")
	// A separate process group means killing Core's own process does not
	// kill this observer; a deployed supervisor would need launchd policy.
	Configure(guard)
	stdin, err := guard.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	guardStdout, err := guard.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.Start(); err != nil {
		t.Fatal(err)
	}
	guardWaited := false
	t.Cleanup(func() {
		_ = stdin.Close()
		if !guardWaited {
			_ = guard.Process.Kill()
			_ = guard.Wait()
		}
	})
	if err := json.NewEncoder(stdin).Encode(req); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	ack := make(chan string, 1)
	go func() {
		reader := bufio.NewScanner(guardStdout)
		if reader.Scan() {
			ack <- reader.Text()
		} else {
			ack <- ""
		}
	}()
	select {
	case value := <-ack:
		if value != "OWNED_OBSERVER_ARMED" {
			t.Fatalf("observer not armed: %q", value)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("observer did not arm")
	}
	receipt := filepath.Join(state, req.ReceiptName)
	before, err := os.ReadFile(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "state=armed\n") ||
		strings.Contains(string(before), "state=wrapper_exit_observed\n") {
		t.Fatalf("invalid before-crash state %q", before)
	}
	// Simulate unclean Core death. The observed worker remains live because
	// it is in its own process group and polls a private release sentinel.
	if err := core.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = core.Wait()
	coreWaited = true
	if _, err := os.Stat(receipt); err != nil {
		t.Fatal("Core crash destroyed observer receipt")
	}
	if err := os.WriteFile(release, []byte("natural exit"), 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- guard.Wait() }()
	select {
	case err := <-done:
		guardWaited = true
		if err != nil {
			t.Fatalf("independent observer did not observe owned wrapper exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("independent observer could not finish after Core death")
	}
	after, err := os.ReadFile(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(after), "state=armed\n") != 1 ||
		strings.Count(string(after), "state=wrapper_exit_observed\n") != 1 ||
		!strings.Contains(string(after), fmt.Sprintf("worker_pid=%d,", workerPID)) {
		t.Fatalf("durable exit receipt did not survive Core crash: %q", after)
	}
	st, err := os.Stat(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0600 {
		t.Fatalf("unexpected receipt permissions %o", st.Mode().Perm())
	}
}

func TestIndependentObserverRejectsUntrustedAndReusedBirth(t *testing.T) {
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	birth, err := ObserveBirthIdentity(context.Background(), os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	fake := birth
	fake.PID += 200
	fake.ParentPID = birth.PID
	fake.StartUsec = (fake.StartUsec + 1) % 1000000
	req := IndependentExitRequest{Core: birth, Worker: fake, StateDir: dir, ReceiptName: "observer-safe-fixture.v1"}
	called := false
	if err := WatchIndependentWorkerExit(context.Background(), req, func() error { called = true; return nil }); !errors.Is(err, ErrIndependentExitObserverUnavailable) {
		t.Fatalf("untrusted process birth accepted: %v", err)
	}
	if called {
		t.Fatal("invalid worker receipt acknowledged as armed")
	}
	if _, err := os.Stat(filepath.Join(dir, req.ReceiptName)); !os.IsNotExist(err) {
		t.Fatalf("untrusted process created receipt: %v", err)
	}
}
