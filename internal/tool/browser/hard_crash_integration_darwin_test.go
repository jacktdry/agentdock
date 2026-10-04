//go:build browser_integration && darwin

package browser

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestManagedWorkerHardCrashCleanupHelper(t *testing.T) {
	if os.Getenv("AGENTDOCK_MANAGED_CRASH_HELPER") != "1" {
		return
	}
	registry := NewWorkerRegistry(WorkerDependencies{})
	manager := NewManagedLeaseManager(registry)
	scope := leaseScope()
	scope.CanonicalWorkspaceRoot = t.TempDir()
	meta, _, err := manager.Acquire(context.Background(), scope, leaseStart(t), "about:blank")
	if err != nil {
		t.Fatal(err)
	}
	if meta.ConnectorPID.PID <= 0 {
		t.Fatalf("managed worker pid=%d", meta.ConnectorPID.PID)
	}
	fmt.Printf("AGENTDOCK_CRASH_READY %d\n", meta.ConnectorPID.PID)
	select {}
}

func TestManagedWorkerHardCrashReturnsOwnedProcessGroupToBaseline(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestManagedWorkerHardCrashCleanupHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AGENTDOCK_MANAGED_CRASH_HELPER=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(stdout)
	workerPID := 0
	ready := make(chan error, 1)
	go func() {
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "AGENTDOCK_CRASH_READY ") {
				continue
			}
			pid, parseErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "AGENTDOCK_CRASH_READY ")))
			if parseErr != nil || pid <= 0 {
				ready <- fmt.Errorf("invalid worker pid line %q: %v", line, parseErr)
				return
			}
			workerPID = pid
			ready <- nil
			return
		}
		ready <- fmt.Errorf("helper exited before ready: %s", stderr.String())
	}()
	select {
	case err := <-ready:
		if err != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
			t.Fatal(err)
		}
	case <-time.After(45 * time.Second):
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		t.Fatal("managed crash helper did not become ready")
	}
	if err := syscall.Kill(-workerPID, 0); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		t.Fatalf("managed worker process group not alive before crash: %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_, _ = cmd.Process.Wait()
	deadline := time.Now().Add(15 * time.Second)
	for {
		err := syscall.Kill(-workerPID, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			t.Fatalf("probe managed worker process group: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("managed worker process group %d survived host hard crash", workerPID)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
