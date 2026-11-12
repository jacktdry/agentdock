//go:build darwin

package app

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testPrivateNextState(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".agentdock-next")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSharedBrowserRestartGuardCrashAndVerifiedRecovery(t *testing.T) {
	root := testPrivateNextState(t)
	marker := filepath.Join(root, sharedBrowserRestartMarker)
	first, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	// A killed process leaves the marker. In this synthetic crash fixture the
	// original guard is intentionally not released before the next admission.
	if _, err := os.Lstat(marker); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, err := acquireSharedBrowserRestartGuard(root); !errors.Is(err, errSharedBrowserRestartGuard) {
			t.Fatalf("stale/crashed owner was admitted: %v", err)
		}
	}
	if err := first.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
	if err := first.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("guard not cleared: %v", err)
	}
	next, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := next.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedBrowserRestartGuardConcurrentAndTamper(t *testing.T) {
	root := testPrivateNextState(t)
	marker := filepath.Join(root, sharedBrowserRestartMarker)
	var wg sync.WaitGroup
	results := make(chan *sharedBrowserRestartGuard, 24)
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			guard, err := acquireSharedBrowserRestartGuard(root)
			if err == nil {
				results <- guard
			} else {
				results <- nil
			}
		}()
	}
	wg.Wait()
	close(results)
	var owner *sharedBrowserRestartGuard
	for g := range results {
		if g != nil {
			if owner != nil {
				t.Fatal("multiple concurrent marker owners")
			}
			owner = g
		}
	}
	if owner == nil {
		t.Fatal("no owner admitted")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := owner.releaseAfterVerifiedClose(); !errors.Is(err, errSharedBrowserRestartGuard) {
		t.Fatalf("replacement inode was removed: %v", err)
	}
	if err := owner.releaseAfterVerifiedClose(); !errors.Is(err, errSharedBrowserRestartGuard) {
		t.Fatalf("failed release was forgotten: %v", err)
	}
	raw, err := os.ReadFile(marker)
	if err != nil || string(raw) != "replacement" {
		t.Fatalf("wrong inode affected: %q %v", raw, err)
	}
}

func TestSharedBrowserRestartGuardRejectsUntrustedState(t *testing.T) {
	root := testPrivateNextState(t)
	marker := filepath.Join(root, sharedBrowserRestartMarker)
	// A symlink may not be used as a trusted state root or existing marker.
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireSharedBrowserRestartGuard(alias); !errors.Is(err, errSharedBrowserRestartGuard) {
		t.Fatalf("symlink root: %v", err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), marker); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireSharedBrowserRestartGuard(root); !errors.Is(err, errSharedBrowserRestartGuard) {
		t.Fatalf("symlink marker: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireSharedBrowserRestartGuard(root); !errors.Is(err, errSharedBrowserRestartGuard) {
		t.Fatalf("world-writable state: %v", err)
	}
}

func TestSharedBrowserRestartGuardSubprocessHelper(t *testing.T) {
	if os.Getenv("AGENTDOCK_TEST_SHARED_GUARD_CHILD") != "1" {
		return
	}
	root := os.Getenv("AGENTDOCK_TEST_SHARED_GUARD_ROOT")
	if _, err := acquireSharedBrowserRestartGuard(root); err != nil {
		t.Fatal(err)
	}
	fmt.Println("AGENTDOCK_SHARED_GUARD_READY")
	select {}
}

func TestSharedBrowserRestartGuardHardCrashMarkerSurvives(t *testing.T) {
	root := testPrivateNextState(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestSharedBrowserRestartGuardSubprocessHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AGENTDOCK_TEST_SHARED_GUARD_CHILD=1", "AGENTDOCK_TEST_SHARED_GUARD_ROOT="+root)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	defer func() {
		if !stopped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	ready := make(chan bool, 1)
	go func() {
		scan := bufio.NewScanner(pipe)
		for scan.Scan() {
			if scan.Text() == "AGENTDOCK_SHARED_GUARD_READY" {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("child did not create guard")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("guard helper never became ready")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	stopped = true
	if _, err := acquireSharedBrowserRestartGuard(root); !errors.Is(err, errSharedBrowserRestartGuard) {
		t.Fatalf("hard crash freed ownership marker: %v", err)
	}
	if err := os.Remove(filepath.Join(root, sharedBrowserRestartMarker)); err != nil {
		t.Fatal(err)
	}
}
