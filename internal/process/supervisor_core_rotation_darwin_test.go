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
	"testing"
	"time"
)

type coreRotationFixtureEnvelope struct {
	Core       BirthIdentity              `json:"core"`
	Supervisor BirthIdentity              `json:"supervisor"`
	Worker     BirthIdentity              `json:"worker"`
	Key        string                     `json:"key"`
	Challenge  SupervisorHandoffChallenge `json:"challenge"`
	Socket     string                     `json:"socket"`
}

func TestSupervisorCoreRotationGoHelper(t *testing.T) {
	switch os.Getenv("AGENTDOCK_SUPERVISOR_ROTATION_ROLE") {
	case "core":
		var env coreRotationFixtureEnvelope
		if err := json.NewDecoder(io.LimitReader(os.Stdin, 4096)).Decode(&env); err != nil {
			t.Fatal(err)
		}
		key, err := hex.DecodeString(env.Key)
		if err != nil {
			t.Fatal(err)
		}
		frame, err := HandoffAttachMACForWorkerParent(key, env.Challenge, env.Core, env.Supervisor,
			env.Worker, SupervisorWorkerParentSupervisor)
		if err != nil {
			t.Fatal(err)
		}
		conn, err := net.Dial("unix", env.Socket)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
		if err := json.NewEncoder(conn).Encode(frame); err != nil {
			t.Fatal(err)
		}
		reply, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil || reply != "ROTATION_ATTACHED\n" {
			t.Fatalf("handoff refused %q: %v", reply, err)
		}
		_ = conn.Close()
		// Never cleans up any real process; parent owns this test-child PID.
		select {}
	case "worker":
		_, _ = io.Copy(io.Discard, os.Stdin)
	}
}

func startRotationCore(t *testing.T, ctx context.Context) (*exec.Cmd, io.WriteCloser, BirthIdentity) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestSupervisorCoreRotationGoHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AGENTDOCK_SUPERVISOR_ROTATION_ROLE=core")
	Configure(cmd)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	birth, err := ObserveDirectChildBirth(ctx, os.Getpid(), cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	return cmd, stdin, birth
}

func startRotationWorker(t *testing.T, ctx context.Context) (*SupervisorOwnedStdio, BirthIdentity) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestSupervisorCoreRotationGoHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AGENTDOCK_SUPERVISOR_ROTATION_ROLE=worker")
	owned, err := StartSupervisorOwnedStdio(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// An unexpected failure may leave this test-private child alive.
		// Only our exact unreaped cmd handle is signaled, never a snapshot PID.
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return owned, owned.Birth()
}

func startRotationSocket(t *testing.T) (*net.UnixListener, string) {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "ad-n-rotation-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "generation.sock")
	raw, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	listener := raw.(*net.UnixListener)
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener, path
}

func deliverAndAttachRotationCore(t *testing.T, ctx context.Context,
	listener *net.UnixListener, endpoint string, stdin io.Writer,
	h *ReadOnlySupervisorHandoff, core, supervisor, worker BirthIdentity, key []byte) SupervisorHandoffAttach {
	t.Helper()
	challenge, err := h.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	env := coreRotationFixtureEnvelope{Core: core, Supervisor: supervisor, Worker: worker,
		Challenge: challenge, Key: hex.EncodeToString(key), Socket: endpoint}
	if err := json.NewEncoder(stdin).Encode(env); err != nil {
		t.Fatal(err)
	}
	if err := listener.SetDeadline(time.Now().Add(4 * time.Second)); err != nil {
		t.Fatal(err)
	}
	conn, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	var received SupervisorHandoffAttach
	if err := json.NewDecoder(io.LimitReader(conn, 2048)).Decode(&received); err != nil {
		t.Fatal(err)
	}
	if err := AcceptVerifiedSupervisorHandoff(ctx, conn, h, received); err != nil {
		t.Fatalf("live Core generation rejected by kernel Unix peer: %v", err)
	}
	if _, err := fmt.Fprintln(conn, "ROTATION_ATTACHED"); err != nil {
		t.Fatal(err)
	}
	return received
}

func TestSupervisorCoreRotationNativeFreshCoreNoToolDispatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	listener, path := startRotationSocket(t)
	owned, worker := startRotationWorker(t, ctx)
	supervisor, err := ObserveBirthIdentity(ctx, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	first, firstStdin, oldCore := startRotationCore(t, ctx)
	keyA := make([]byte, 32)
	_, _ = rand.Read(keyA)
	rotation, err := NewSupervisorCoreRotation(ctx, oldCore, supervisor, worker, keyA)
	if err != nil {
		t.Fatal(err)
	}
	old, err := rotation.CurrentHandoff()
	if err != nil {
		t.Fatal(err)
	}
	oldMAC := deliverAndAttachRotationCore(t, ctx, listener, path, firstStdin, old, oldCore, supervisor, worker, keyA)
	if rotation.Generation() != 1 {
		t.Fatal("wrong initial generation")
	}
	// A still-running authenticated Core is NOT a reauthentication event.
	candidate, candidateStdin, candidateCore := startRotationCore(t, ctx)
	_ = candidate // Still test-owned, no secret issued until after Core A exits.
	keyB := make([]byte, 32)
	_, _ = rand.Read(keyB)
	if _, err := rotation.AdmitSuccessor(ctx, candidateCore, keyB); err == nil {
		t.Fatal("successor admitted before kernel Core NOTE_EXIT")
	}
	if err := first.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := rotation.WitnessCoreExit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := first.Wait(); err == nil {
		t.Fatal("test forced Core exit returned zero")
	}
	if old.Phase() != SupervisorCoreExited {
		t.Fatal("old HMAC epoch not invalidated")
	}
	if _, err := rotation.CurrentHandoff(); err == nil {
		t.Fatal("old handoff accessible after Core exit")
	}
	if _, err := rotation.AdmitSuccessor(ctx, candidateCore, keyA); err == nil {
		t.Fatal("old Core secret reused")
	}
	forged := candidateCore
	forged.StartUsec++
	if _, err := rotation.AdmitSuccessor(ctx, forged, keyB); err == nil {
		t.Fatal("wrong Core kernel birth claimed succession")
	}
	updated, err := rotation.AdmitSuccessor(ctx, candidateCore, keyB)
	if err != nil {
		t.Fatal(err)
	}
	if rotation.Generation() != 2 {
		t.Fatal("generation not advanced")
	}
	// Old attach proof is both the wrong Core and wrong MAC epoch.
	if err := updated.attachVerifiedPeer(oldCore, oldMAC); err == nil {
		t.Fatal("old Core reattached new generation")
	}
	signedOld, _ := SignSupervisorOwnedToolCallPrototype(keyA, old.challenge, oldCore, supervisor,
		worker, 1, "echo", map[string]any{"text": "old"})
	if err := old.acceptVerifiedToolCall(oldCore, signedOld, map[string]struct{}{"echo": {}}); err == nil {
		t.Fatal("old Core able to call after witnessed NOTE_EXIT")
	}
	attached2 := deliverAndAttachRotationCore(t, ctx, listener, path, candidateStdin,
		updated, candidateCore, supervisor, worker, keyB)
	if attached2.Session == oldMAC.Session || attached2.MAC == oldMAC.MAC {
		t.Fatal("replacement reused retired Core session or MAC")
	}
	if updated.Phase() != SupervisorCoreAttached {
		t.Fatal("successor did not complete real Unix attach")
	}
	if err := candidate.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := rotation.WitnessCoreExit(ctx); err != nil {
		t.Fatal(err)
	}
	_ = candidate.Wait()
	// Supervisor-owned worker never dies when the first Core is terminated.
	if _, err := fmt.Fprintln(owned.Stdin(), "worker-is-still-owned"); err != nil {
		t.Fatal(err)
	}
	_ = owned.Stdin().Close()
	if err := owned.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorCoreRotationNativeUnsafeDispatchFreezes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	listener, path := startRotationSocket(t)
	owned, worker := startRotationWorker(t, ctx)
	supervisor, err := ObserveBirthIdentity(ctx, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	first, stdin, oldCore := startRotationCore(t, ctx)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	rotation, err := NewSupervisorCoreRotation(ctx, oldCore, supervisor, worker, key)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := rotation.CurrentHandoff()
	deliverAndAttachRotationCore(t, ctx, listener, path, stdin, old, oldCore, supervisor, worker, key)
	challenge := old.challenge
	frame, err := SignSupervisorOwnedToolCallPrototype(key, challenge, oldCore, supervisor, worker,
		1, "echo", map[string]any{"text": "possibly-performed"})
	if err != nil {
		t.Fatal(err)
	}
	if err := old.acceptVerifiedToolCall(oldCore, frame, map[string]struct{}{"echo": {}}); err != nil {
		t.Fatal(err)
	}
	if err := first.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := rotation.WitnessCoreExit(ctx); !errors.Is(err, ErrSupervisorHandoffUnavailable) {
		t.Fatalf("ambiguous previous tool execution must freeze reauth: %v", err)
	}
	_ = first.Wait()
	if old.Phase() != SupervisorUncertain {
		t.Fatal("uncertain call did not leave sticky hold")
	}
	second, _, newCore := startRotationCore(t, ctx)
	_ = second
	fresh := make([]byte, 32)
	_, _ = rand.Read(fresh)
	if _, err := rotation.AdmitSuccessor(ctx, newCore, fresh); err == nil {
		t.Fatal("ambiguous previous tool reauthenticated after Core crash")
	}
	if _, err := rotation.CurrentHandoff(); err == nil {
		t.Fatal("uncertain state leaked session")
	}
	_ = owned.Stdin().Close()
	if err := owned.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

// An interrupted kernel witness is not a Core crash certificate. Even a
// later valid Core PID cannot be used to reset an uncertain generation.
func TestSupervisorCoreRotationCanceledWitnessFailClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	owned, worker := startRotationWorker(t, ctx)
	supervisor, err := ObserveBirthIdentity(ctx, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	_, _, core := startRotationCore(t, ctx)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	rotation, err := NewSupervisorCoreRotation(ctx, core, supervisor, worker, key)
	if err != nil {
		t.Fatal(err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if err := rotation.WitnessCoreExit(canceled); !errors.Is(err, ErrSupervisorHandoffUnavailable) {
		t.Fatalf("cancelled wait was accepted as kernel proof: %v", err)
	}
	if _, err := rotation.CurrentHandoff(); err == nil {
		t.Fatal("cancelled observer still issued session")
	}
	_, _, another := startRotationCore(t, ctx)
	fresh := make([]byte, 32)
	_, _ = rand.Read(fresh)
	if _, err := rotation.AdmitSuccessor(ctx, another, fresh); err == nil {
		t.Fatal("cancelled witness allowed successor")
	}
	_ = owned.Stdin().Close()
	if err := owned.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorCoreRotationRejectsForgedOwnerAndWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	owned, worker := startRotationWorker(t, ctx)
	supervisor, err := ObserveBirthIdentity(ctx, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	_, _, core := startRotationCore(t, ctx)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	wrongSupervisor := supervisor
	wrongSupervisor.StartUsec++
	if _, err := NewSupervisorCoreRotation(ctx, core, wrongSupervisor, worker, key); err == nil {
		t.Fatal("forged Supervisor kernel birth admitted")
	}
	forgedWorker := worker
	forgedWorker.StartUsec++
	if _, err := NewSupervisorCoreRotation(ctx, core, supervisor, forgedWorker, key); err == nil {
		t.Fatal("forged Worker kernel birth admitted")
	}
	_ = owned.Stdin().Close()
	if err := owned.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorCoreRotationWaitDoesNotBlockCurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	owned, worker := startRotationWorker(t, ctx)
	supervisor, err := ObserveBirthIdentity(ctx, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	_, _, core := startRotationCore(t, ctx)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	rotation, err := NewSupervisorCoreRotation(ctx, core, supervisor, worker, key)
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, stop := context.WithCancel(ctx)
	outcome := make(chan error, 1)
	go func() { outcome <- rotation.WitnessCoreExit(waitCtx) }()
	// Never hold the manager lock while the Core is alive and the
	// kernel watcher is still blocked on its eventual NOTE_EXIT.
	for i := 0; i < 20; i++ {
		if _, err := rotation.CurrentHandoff(); err != nil {
			t.Fatal("live Core session blocked by observer:", err)
		}
		time.Sleep(time.Millisecond)
	}
	stop()
	select {
	case err := <-outcome:
		if !errors.Is(err, ErrSupervisorHandoffUnavailable) {
			t.Fatal("cancelled witness accepted:", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent kernel watcher blocked active session indefinitely")
	}
	if _, err := rotation.CurrentHandoff(); err == nil {
		t.Fatal("cancelled watcher did not quarantine generation")
	}
	_ = owned.Stdin().Close()
	if err := owned.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}
