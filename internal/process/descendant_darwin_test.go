//go:build darwin

package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDescendantOwnedFixtureHelper(t *testing.T) {
	switch os.Getenv("AGENTDOCK_TEST_DESCENDANT_ROLE") {
	case "grandchild":
		_, _ = io.Copy(io.Discard, os.Stdin)
	case "parent":
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		child := exec.Command(os.Args[0], "-test.run=^TestDescendantOwnedFixtureHelper$", "-test.count=1")
		child.Env = append(os.Environ(), "AGENTDOCK_TEST_DESCENDANT_ROLE=grandchild")
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		child.Stdin = r
		child.Stdout = io.Discard
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		_ = r.Close()
		fmt.Printf("OWNED_GRANDCHILD_PID=%d\n", child.Process.Pid)
		_, _ = io.Copy(io.Discard, os.Stdin)
		_ = w.Close()
		_ = child.Wait()
	}
}

func TestDescendantBirthOwnDifferentGroupGrandchild(t *testing.T) {
	// Only starts two private copies of this Go test executable. Never any
	// browser, existing user process, npx, or unrelated profile.
	cmd := exec.Command(os.Args[0], "-test.run=^TestDescendantOwnedFixtureHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AGENTDOCK_TEST_DESCENDANT_ROLE=parent")
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
	got := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			got <- scanner.Text()
		} else {
			got <- ""
		}
	}()
	var line string
	select {
	case line = <-got:
	case <-time.After(4 * time.Second):
		t.Fatal("owned fixture did not start")
	}
	raw, ok := strings.CutPrefix(line, "OWNED_GRANDCHILD_PID=")
	if !ok {
		t.Fatalf("unexpected fixture handshake: %q", line)
	}
	grandPID, err := strconv.Atoi(raw)
	if err != nil || grandPID <= 0 {
		t.Fatal("invalid grandchild PID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	root, err := ObserveDirectChildBirth(ctx, os.Getpid(), cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	members, err := ObserveDescendantBirths(ctx, root)
	if err != nil {
		t.Fatalf("kernel UID/process birth sample unavailable: %v", err)
	}
	found := false
	for _, m := range members {
		if m.PID != grandPID {
			continue
		}
		found = true
		if m.ParentPID != root.PID || m.ProcessGroupID == root.ProcessGroupID ||
			m.ProcessGroupID != grandPID {
			t.Fatalf("setsid descendant lineage falsely identified: %+v root=%+v", m, root)
		}
	}
	if !found {
		t.Fatalf("setsid child PID=%d not linked to owned root PID=%d", grandPID, root.PID)
	}
}
