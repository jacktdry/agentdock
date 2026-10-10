//go:build darwin

package process

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type nativeHandoffEnvelope struct {
	Challenge  SupervisorHandoffChallenge `json:"challenge"`
	Key        string                     `json:"key"`
	Core       BirthIdentity              `json:"core"`
	Supervisor BirthIdentity              `json:"supervisor"`
	Worker     BirthIdentity              `json:"worker"`
	Endpoint   string                     `json:"endpoint"`
}

func TestSupervisorHandoffNativeHelper(t *testing.T) {
	switch os.Getenv("AGENTDOCK_TEST_HANDOFF_ROLE") {
	case "worker":
		release := os.Getenv("AGENTDOCK_TEST_HANDOFF_RELEASE")
		limit := time.Now().Add(14 * time.Second)
		for time.Now().Before(limit) {
			if _, err := os.Stat(release); err == nil {
				return
			}
			time.Sleep(15 * time.Millisecond)
		}
		t.Fatal("owned handoff worker fixture did not exit")
	case "core":
		child := exec.Command(os.Args[0], "-test.run=^TestSupervisorHandoffNativeHelper$", "-test.count=1")
		child.Env = append(os.Environ(),
			"AGENTDOCK_TEST_HANDOFF_ROLE=worker",
			"AGENTDOCK_TEST_HANDOFF_RELEASE="+os.Getenv("AGENTDOCK_TEST_HANDOFF_RELEASE"))
		Configure(child)
		child.Stdout = io.Discard
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		fmt.Printf("OWNED_HANDOFF_WORKER=%d\n", child.Process.Pid)
		var env nativeHandoffEnvelope
		if err := json.NewDecoder(os.Stdin).Decode(&env); err != nil {
			t.Fatal(err)
		}
		secret, err := hex.DecodeString(env.Key)
		if err != nil {
			t.Fatal(err)
		}
		frame, err := HandoffAttachMAC(secret, env.Challenge, env.Core, env.Supervisor, env.Worker)
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
		if ack, err := bufio.NewReader(conn).ReadString('\n'); err != nil || ack != "HANDOFF_ACCEPTED\n" {
			t.Fatalf("supervisor failed to acknowledge verified Core: %q %v", ack, err)
		}
		_ = conn.Close()
		// This exact Core will be forcibly terminated by its owning test.
		select {}
	}
}

func TestSupervisorHandoffKernelPeerAndCoreCrashTransitions(t *testing.T) {
	// Only own Go test binaries. No browser/CDP/AGY or unrelated PID touched.
	root, err := os.MkdirTemp("", "ad-next-handoff-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(root, "supervisor.sock")
	listener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.Chmod(sock, 0600); err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(root, "worker-exit")
	t.Cleanup(func() { _ = os.WriteFile(release, []byte("exit"), 0600) })

	core := exec.Command(os.Args[0], "-test.run=^TestSupervisorHandoffNativeHelper$", "-test.count=1")
	core.Env = append(os.Environ(),
		"AGENTDOCK_TEST_HANDOFF_ROLE=core",
		"AGENTDOCK_TEST_HANDOFF_RELEASE="+release)
	Configure(core)
	stdout, err := core.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := core.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		_ = stdin.Close()
		if !waited {
			_ = core.Process.Kill()
			_ = core.Wait()
		}
	})
	ready := make(chan string, 1)
	go func() {
		s := bufio.NewScanner(stdout)
		if s.Scan() {
			ready <- s.Text()
		} else {
			ready <- ""
		}
	}()
	var line string
	select {
	case line = <-ready:
	case <-time.After(4 * time.Second):
		t.Fatal("owned Core never spawned")
	}
	workerText, ok := strings.CutPrefix(line, "OWNED_HANDOFF_WORKER=")
	if !ok {
		t.Fatalf("Core fixture output wrong: %q", line)
	}
	pid, err := strconv.Atoi(workerText)
	if err != nil || pid <= 0 {
		t.Fatal("bad fixture worker PID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	coreBirth, err := ObserveDirectChildBirth(ctx, os.Getpid(), core.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	workerBirth, err := ObserveDirectChildBirth(ctx, core.Process.Pid, pid)
	if err != nil {
		t.Fatal(err)
	}
	supervisorBirth, err := ObserveBirthIdentity(ctx, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	state, err := NewReadOnlySupervisorHandoff(coreBirth, supervisorBirth, workerBirth, key)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := state.Challenge()
	if err != nil {
		t.Fatal(err)
	}

	// A different live process (our test supervisor) cannot pose as the Core.
	fake, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	fakeConn, err := listener.(*net.UnixListener).AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifySupervisorHandoffPeer(ctx, fakeConn, coreBirth); !errors.Is(err, ErrSupervisorHandoffUnavailable) {
		t.Fatalf("test supervisor spoofed authenticated Core: %v", err)
	}
	_ = fakeConn.Close()
	_ = fake.Close()
	if state.Phase() != SupervisorAwaitingCore {
		t.Fatal("peer spoof mutated state")
	}

	coreWatch, err := WatchVerifiedExit(ctx, coreBirth)
	if err != nil {
		t.Fatal(err)
	}
	defer coreWatch.Close()
	workerWatch, err := WatchVerifiedExit(ctx, workerBirth)
	if err != nil {
		t.Fatal(err)
	}
	defer workerWatch.Close()
	env := nativeHandoffEnvelope{
		Challenge: challenge, Key: hex.EncodeToString(key),
		Core: coreBirth, Supervisor: supervisorBirth, Worker: workerBirth, Endpoint: sock,
	}
	if err := json.NewEncoder(stdin).Encode(env); err != nil {
		t.Fatal(err)
	}
	conn, err := listener.(*net.UnixListener).AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := VerifySupervisorHandoffPeer(ctx, conn, coreBirth); err != nil {
		t.Fatalf("OS attested Core connection rejected: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var frame SupervisorHandoffAttach
	if err := json.NewDecoder(io.LimitReader(conn, 2048)).Decode(&frame); err != nil {
		t.Fatal(err)
	}
	if err := AcceptVerifiedSupervisorHandoff(ctx, conn, state, frame); err != nil {
		t.Fatal(err)
	}
	if state.Phase() != SupervisorCoreAttached {
		t.Fatal("verified Core attach state not recorded")
	}
	if _, err := fmt.Fprintln(conn, "HANDOFF_ACCEPTED"); err != nil {
		t.Fatal(err)
	}
	if err := state.attachVerifiedPeer(coreBirth, frame); err == nil {
		t.Fatal("replay admitted")
	}
	_ = conn.Close()
	// Abruptly terminate ONLY the Core process created above. Supervisor and
	// worker have their own lifetime; supervisor retains OS kernel watches.
	if err := core.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := coreWatch.Wait(ctx); err != nil {
		t.Fatalf("missing Core NOTE_EXIT: %v", err)
	}
	_ = core.Wait()
	waited = true
	if err := state.recordCoreExit(coreBirth); err != nil {
		t.Fatal(err)
	}
	if state.Phase() != SupervisorCoreExited {
		t.Fatal("Core death not durable in state")
	}
	if err := state.attachVerifiedPeer(coreBirth, frame); err == nil {
		t.Fatal("crashed Core reattached")
	}
	if err := os.WriteFile(release, []byte("own worker natural exit"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := workerWatch.Wait(ctx); err != nil {
		t.Fatalf("owned worker never exited: %v", err)
	}
	if err := state.recordWorkerExit(workerBirth); err != nil {
		t.Fatal(err)
	}
	if state.Phase() != SupervisorWorkerExited {
		t.Fatal("wrapper exit not recorded after Core crash")
	}
	// This state is an observation only; never authorizes process cleanups.
}
