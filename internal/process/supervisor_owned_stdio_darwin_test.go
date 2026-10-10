//go:build darwin

package process

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSupervisorOwnedNativeHelper(t *testing.T) {
	switch os.Getenv("AD_NEXT_OWNED_FIXTURE_ROLE") {
	case "worker":
		fmt.Fprintln(os.Stdout, "SUPERVISOR_WORKER_READY")
		scan := bufio.NewScanner(os.Stdin)
		for scan.Scan() {
			line := scan.Text()
			if line == "exit" {
				return
			}
			fmt.Fprintln(os.Stdout, "WORKER_ECHO:"+line)
		}
	case "core":
		var env nativeHandoffEnvelope
		if err := json.NewDecoder(os.Stdin).Decode(&env); err != nil {
			t.Fatal(err)
		}
		key, err := hex.DecodeString(env.Key)
		if err != nil {
			t.Fatal(err)
		}
		frame, err := HandoffAttachMACForWorkerParent(key, env.Challenge, env.Core, env.Supervisor, env.Worker, SupervisorWorkerParentSupervisor)
		if err != nil {
			t.Fatal(err)
		}
		conn, err := net.Dial("unix", env.Endpoint)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(conn).Encode(frame); err != nil {
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
		ack, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil || ack != "HANDOFF_ACCEPTED\n" {
			t.Fatalf("no verified supervisor ack: %q %v", ack, err)
		}
		_ = conn.Close()
		// Real Core-like test process intentionally stays alive until the test
		// explicitly terminates ONLY this test-owned child process.
		select {}
	}
}

func TestSupervisorOwnedStdioSurvivesCoreCrashWithLiveTransport(t *testing.T) {
	root, err := os.MkdirTemp("", "next-owned-stdio-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	listenPath := filepath.Join(root, "handoff.sock")
	listener, err := net.Listen("unix", listenPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.Chmod(listenPath, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	cmd := exec.Command(os.Args[0], "-test.run=^TestSupervisorOwnedNativeHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AD_NEXT_OWNED_FIXTURE_ROLE=worker")
	owned, err := StartSupervisorOwnedStdio(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	reaped := false
	t.Cleanup(func() {
		if !reaped {
			// This is the one test-owned command, not an observed/browser PID.
			_ = cmd.Process.Kill()
			_ = owned.Stdin().Close()
			_ = cmd.Wait()
		}
	})
	child := owned.Birth()
	if child.ParentPID != os.Getpid() || child.ProcessGroupID != child.PID {
		t.Fatalf("not a Supervisor-owned direct process: %+v", child)
	}
	lines := make(chan string, 4)
	go func() {
		scanner := bufio.NewScanner(owned.Stdout())
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	readLine := func() string {
		t.Helper()
		select {
		case value, ok := <-lines:
			if !ok {
				t.Fatal("worker stdout unexpectedly closed")
			}
			return value
		case <-time.After(3 * time.Second):
			t.Fatal("worker stdout timed out")
			return ""
		}
	}
	if got := readLine(); got != "SUPERVISOR_WORKER_READY" {
		t.Fatalf("unready worker: %q", got)
	}

	core := exec.Command(os.Args[0], "-test.run=^TestSupervisorOwnedNativeHelper$", "-test.count=1")
	core.Env = append(os.Environ(), "AD_NEXT_OWNED_FIXTURE_ROLE=core")
	Configure(core)
	coreStdin, err := core.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Start(); err != nil {
		t.Fatal(err)
	}
	coreReaped := false
	t.Cleanup(func() {
		_ = coreStdin.Close()
		if !coreReaped {
			_ = core.Process.Kill()
			_ = core.Wait()
		}
	})
	coreIdentity, err := ObserveDirectChildBirth(ctx, os.Getpid(), core.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := ObserveBirthIdentity(ctx, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentPID == coreIdentity.PID {
		t.Fatal("Worker inadvertently owned by Core")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	handoff, err := NewSupervisorOwnedReadOnlyHandoff(coreIdentity, supervisor, child, secret)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := handoff.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	coreWatch, err := WatchVerifiedExit(ctx, coreIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer coreWatch.Close()
	env := nativeHandoffEnvelope{
		Challenge: challenge, Key: hex.EncodeToString(secret),
		Core: coreIdentity, Supervisor: supervisor, Worker: child, Endpoint: listenPath,
	}
	if err := json.NewEncoder(coreStdin).Encode(env); err != nil {
		t.Fatal(err)
	}
	conn, err := listener.(*net.UnixListener).AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var frame SupervisorHandoffAttach
	if err := json.NewDecoder(conn).Decode(&frame); err != nil {
		t.Fatal(err)
	}
	if err := AcceptVerifiedSupervisorHandoff(ctx, conn, handoff, frame); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(conn, "HANDOFF_ACCEPTED"); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if err := core.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := coreWatch.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	_ = core.Wait()
	coreReaped = true
	if err := handoff.recordCoreExit(coreIdentity); err != nil {
		t.Fatal(err)
	}
	if handoff.Phase() != SupervisorCoreExited {
		t.Fatal("Core crash not recorded")
	}

	// The Supervisor still has LIVE worker stdin/stdout and exact Process handle.
	if _, err := fmt.Fprintln(owned.Stdin(), "ping-after-core-crash"); err != nil {
		t.Fatal(err)
	}
	if got := readLine(); got != "WORKER_ECHO:ping-after-core-crash" {
		t.Fatalf("owned Worker transport died with Core: %q", got)
	}
	if _, err := fmt.Fprintln(owned.Stdin(), "exit"); err != nil {
		t.Fatal(err)
	}
	if err := owned.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	reaped = true
	if err := handoff.recordWorkerExit(child); err != nil {
		t.Fatal(err)
	}
	if handoff.Phase() != SupervisorWorkerExited {
		t.Fatal("Worker exit was not recorded")
	}
	if err := owned.Wait(ctx); err != nil {
		t.Fatal("idempotent Wait failed:", err)
	}
	if _, err := handoff.Challenge(); err == nil {
		t.Fatal("completed generation reopened")
	}
	// No Chrome descendants, PID-kill reconciliation or marker unlink.
}

func TestSupervisorOwnedStdioRejectsUnownedSetup(t *testing.T) {
	ctx := context.Background()
	if _, err := StartSupervisorOwnedStdio(ctx, nil); err == nil {
		t.Fatal("nil command admitted")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSupervisorOwnedNativeHelper$")
	Configure(cmd)
	if _, err := StartSupervisorOwnedStdio(ctx, cmd); err == nil {
		t.Fatal("preconfigured OS group admitted")
	}
	cmd = exec.Command(os.Args[0], "-test.run=^TestSupervisorOwnedNativeHelper$")
	cmd.Stdin = strings.NewReader("")
	if _, err := StartSupervisorOwnedStdio(ctx, cmd); err == nil {
		t.Fatal("external stdin accepted")
	}
}
