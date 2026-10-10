//go:build darwin

package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	processcontrol "github.com/uvwt/agentdock/internal/process"
)

func TestSharedBrowserDescendantFixtureHelper(t *testing.T) {
	switch os.Getenv("AGENTDOCK_TEST_APP_DESCENDANT_ROLE") {
	case "grandchild":
		// Emitted AFTER Setsid in the child, unlike the parent's cmd.Start.
		fmt.Println("GRANDCHILD_READY")
		_, _ = io.Copy(io.Discard, os.Stdin)
	case "late-parent":
		fmt.Println("LATE_PARENT_READY")
		scan := bufio.NewScanner(os.Stdin)
		if !scan.Scan() || scan.Text() != "spawn" {
			return
		}
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		grand := exec.Command(os.Args[0], "-test.run=^TestSharedBrowserDescendantFixtureHelper$", "-test.count=1")
		grand.Env = append(os.Environ(), "AGENTDOCK_TEST_APP_DESCENDANT_ROLE=grandchild")
		grand.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		grand.Stdin = r
		grandOutput, err := grand.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		grand.Stderr = os.Stderr
		if err := grand.Start(); err != nil {
			t.Fatal(err)
		}
		_ = r.Close()
		// Do not announce a Setsid child before its own program confirms it
		// has passed fork/exec and the session transition completed.
		grandScanner := bufio.NewScanner(grandOutput)
		if !grandScanner.Scan() || grandScanner.Text() != "GRANDCHILD_READY" {
			t.Fatal("child failed Setsid/exec handshake")
		}
		fmt.Printf("LATE_GRANDCHILD_PID=%d\n", grand.Process.Pid)
		_, _ = io.Copy(io.Discard, os.Stdin)
		_ = w.Close()
		_ = grand.Wait()
	case "parent":
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		grand := exec.Command(os.Args[0], "-test.run=^TestSharedBrowserDescendantFixtureHelper$", "-test.count=1")
		grand.Env = append(os.Environ(), "AGENTDOCK_TEST_APP_DESCENDANT_ROLE=grandchild")
		grand.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		grand.Stdin = r
		grand.Stdout = io.Discard
		grand.Stderr = os.Stderr
		if err := grand.Start(); err != nil {
			t.Fatal(err)
		}
		_ = r.Close()
		fmt.Printf("APP_DESCENDANT_PID=%d\n", grand.Process.Pid)
		_, _ = io.Copy(io.Discard, os.Stdin)
		_ = w.Close()
		_ = grand.Wait()
	}
}

func TestSharedBrowserNativeReadyRecordsDetachedGroupDescendant(t *testing.T) {
	// Fully isolated Go test binaries, not Chrome, npm, Edge, user processes.
	root := testPrivateNextState(t)
	g, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	g.requireStartedEvidence = true
	cmd := exec.Command(os.Args[0], "-test.run=^TestSharedBrowserDescendantFixtureHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AGENTDOCK_TEST_APP_DESCENDANT_ROLE=parent")
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
	waited := false
	t.Cleanup(func() {
		_ = stdin.Close()
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	msg := make(chan string, 1)
	go func() {
		scan := bufio.NewScanner(stdout)
		if scan.Scan() {
			msg <- scan.Text()
		} else {
			msg <- ""
		}
	}()
	var line string
	select {
	case line = <-msg:
	case <-time.After(4 * time.Second):
		t.Fatal("owned child fixture did not start")
	}
	raw, ok := strings.CutPrefix(line, "APP_DESCENDANT_PID=")
	if !ok {
		t.Fatalf("fixture output was not PID: %q", line)
	}
	grandPID, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := g.recordStartedWorker(ctx, cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	if err := g.recordManagedWorker(readyEvidence(cmd.Process.Pid)); err != nil {
		t.Fatal(err)
	}
	journal, err := os.ReadFile(filepath.Join(root, sharedBrowserRestartMarker))
	if err != nil {
		t.Fatal(err)
	}
	rows, valid := parseSharedBrowserEvidence(string(journal))
	if !valid || len(rows) < 3 {
		t.Fatalf("missing attested ready/descendants journal: valid=%v rows=%d", valid, len(rows))
	}
	verified := false
	for _, row := range rows {
		if row.Stage != sharedEvidenceDescendant || row.PID != grandPID {
			continue
		}
		verified = true
		if row.ParentPID != cmd.Process.Pid || row.RootPID != cmd.Process.Pid ||
			row.PGID == cmd.Process.Pid || row.PGID != grandPID {
			t.Fatalf("setsid child not bound to owned root in durable record: %+v", row)
		}
	}
	if !verified {
		t.Fatalf("grandchild PID=%d in new session not recorded at readiness", grandPID)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	waited = true
	end := time.Now().Add(2 * time.Second)
	for {
		g.mu.Lock()
		seen, uncertain := g.exitObserved, g.exitUncertain
		g.mu.Unlock()
		if uncertain {
			t.Fatal("own wrapper NOTE_EXIT became uncertain")
		}
		if seen >= 1 {
			break
		}
		if time.Now().After(end) {
			t.Fatal("own wrapper NOTE_EXIT missing")
		}
		time.Sleep(8 * time.Millisecond)
	}
	if err := g.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, sharedBrowserRestartMarker)); !os.IsNotExist(err) {
		t.Fatalf("clean fixture verified-close retained marker: %v", err)
	}
}

// Test only private Go child processes, with the grandchild launched AFTER
// worker ReadyWitness has already persisted root evidence and started the live
// sampler. The Go grandchild joins a separate session (no Chrome/Edge involved).
func TestSharedBrowserLiveLateSpawnCrossGroupNative(t *testing.T) {
	root := testPrivateNextState(t)
	g, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	g.requireStartedEvidence = true
	g.descendantRefreshInterval = 35 * time.Millisecond
	cmd := exec.Command(os.Args[0], "-test.run=^TestSharedBrowserDescendantFixtureHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AGENTDOCK_TEST_APP_DESCENDANT_ROLE=late-parent")
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
	waited := false
	t.Cleanup(func() {
		_ = stdin.Close()
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	output := make(chan string, 3)
	go func() {
		scan := bufio.NewScanner(stdout)
		for scan.Scan() {
			output <- scan.Text()
		}
		close(output)
	}()
	readLine := func() string {
		t.Helper()
		select {
		case line, ok := <-output:
			if !ok {
				t.Fatal("owned worker unexpectedly closed its stdout")
			}
			return line
		case <-time.After(4 * time.Second):
			t.Fatal("owned fixture did not respond")
			return ""
		}
	}
	if got := readLine(); got != "LATE_PARENT_READY" {
		t.Fatalf("unexpected ready: %q", got)
	}
	if err := g.recordStartedWorker(context.Background(), cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	if err := g.recordManagedWorker(readyEvidence(cmd.Process.Pid)); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(root, sharedBrowserRestartMarker)
	initial, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	rows, valid := parseSharedBrowserEvidence(string(initial))
	if !valid || len(rows) != 2 || rows[1].Stage != sharedEvidenceReady {
		t.Fatalf("late worker unexpectedly had children before spawn: %+v", rows)
	}
	if _, err := io.WriteString(stdin, "spawn\n"); err != nil {
		t.Fatal(err)
	}
	line := readLine()
	pidText, ok := strings.CutPrefix(line, "LATE_GRANDCHILD_PID=")
	if !ok {
		t.Fatalf("unexpected grandchild line %q", line)
	}
	childPID, err := strconv.Atoi(pidText)
	if err != nil {
		t.Fatal(err)
	}
	// Kernel group may have been sampled DURING fork, before the child called
	// setsid: such a historical PGID is valid evidence, not a false lineage.
	// The separate live kernel assertion proves the child DID leave the group.
	confirmed, err := processcontrol.ObserveBirthIdentity(context.Background(), childPID)
	if err != nil || confirmed.ProcessGroupID != childPID || confirmed.ParentPID != cmd.Process.Pid {
		t.Fatalf("child did not become a separate session: %+v %v", confirmed, err)
	}
	var witnessed bool
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		raw, readErr := os.ReadFile(journalPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		entries, ok := parseSharedBrowserEvidence(string(raw))
		if !ok {
			t.Fatalf("periodic child append corrupted journal: %q", raw)
		}
		for _, entry := range entries {
			if entry.PID != childPID || entry.Stage != sharedEvidenceDescendant {
				continue
			}
			if entry.RootPID != cmd.Process.Pid || entry.ParentPID != cmd.Process.Pid ||
				(entry.PGID != childPID && entry.PGID != cmd.Process.Pid) {
				t.Fatalf("late child lineage or historical group incorrectly captured: %+v", entry)
			}
			witnessed = true
		}
		if witnessed {
			break
		}
		time.Sleep(12 * time.Millisecond)
	}
	if !witnessed {
		t.Fatalf("ready-after-spawn worker PID=%d child PID=%d was not observed", cmd.Process.Pid, childPID)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	waited = true
	end := time.Now().Add(3 * time.Second)
	for time.Now().Before(end) {
		g.mu.Lock()
		observed, uncertain := g.exitObserved, g.exitUncertain
		g.mu.Unlock()
		if uncertain {
			t.Fatal("live witness became uncertain during natural exit")
		}
		if observed > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := g.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("owned Go fixtures failed clean guard close: %v", err)
	}
}
