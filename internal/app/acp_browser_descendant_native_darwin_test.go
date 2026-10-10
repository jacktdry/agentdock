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
		_, _ = io.Copy(io.Discard, os.Stdin)
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
