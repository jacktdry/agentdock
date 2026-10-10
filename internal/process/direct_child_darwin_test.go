//go:build darwin

package process

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestDirectChildBirthFixtureHelper(t *testing.T) {
	if os.Getenv("AGENTDOCK_TEST_DIRECT_CHILD_BIRTH") != "1" {
		return
	}
	fmt.Println("DIRECT_CHILD_READY")
	select {}
}

func TestDirectChildBirthRealOwnedFixture(t *testing.T) {
	// Only creates a private copy of this test process; no browser or
	// external process is observed or changed.
	cmd := exec.Command(os.Args[0], "-test.run=^TestDirectChildBirthFixtureHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AGENTDOCK_TEST_DIRECT_CHILD_BIRTH=1")
	Configure(cmd) // dedicated group, exactly as real MCP stdio launch
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
		scan := bufio.NewScanner(stdout)
		ready <- scan.Scan() && scan.Text() == "DIRECT_CHILD_READY"
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("owned fixture failed to signal ready")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("owned fixture did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	id, err := ObserveDirectChildBirth(ctx, os.Getpid(), cmd.Process.Pid)
	if err != nil || id.PID != cmd.Process.Pid || id.ParentPID != os.Getpid() ||
		id.ProcessGroupID != cmd.Process.Pid {
		t.Fatalf("real managed-child kernel proof %+v %v", id, err)
	}
}
