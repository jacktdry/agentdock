//go:build darwin

package client

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	processcontrol "github.com/uvwt/agentdock/internal/process"
)

type supervisorToolIPCTestEnvelope struct {
	Challenge  processcontrol.SupervisorHandoffChallenge `json:"challenge"`
	Core       processcontrol.BirthIdentity              `json:"core"`
	Supervisor processcontrol.BirthIdentity              `json:"supervisor"`
	Worker     processcontrol.BirthIdentity              `json:"worker"`
	Secret     string                                    `json:"secret"`
	Socket     string                                    `json:"socket"`
}

func TestSupervisorToolIPCCoreLikeHelper(t *testing.T) {
	if os.Getenv("AD_NEXT_IPC_CORE_ROLE") != "1" {
		return
	}
	var env supervisorToolIPCTestEnvelope
	if err := json.NewDecoder(os.Stdin).Decode(&env); err != nil {
		t.Fatal(err)
	}
	secret, err := hex.DecodeString(env.Secret)
	if err != nil {
		t.Fatal(err)
	}
	attach, err := processcontrol.HandoffAttachMACForWorkerParent(secret, env.Challenge, env.Core, env.Supervisor, env.Worker, processcontrol.SupervisorWorkerParentSupervisor)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("unix", env.Socket)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(conn).Encode(attach); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	ack, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || ack != "HANDOFF_ACCEPTED\n" {
		t.Fatalf("Supervisor handoff not accepted: %q %v", ack, err)
	}
	_ = conn.Close()
	call := func(frame processcontrol.SupervisorToolCallFrame, allowed bool, expected string) {
		t.Helper()
		peer, err := net.Dial("unix", env.Socket)
		if err != nil {
			t.Fatal(err)
		}
		reply, err := CallSupervisorScopedMCPPrototype(context.Background(), peer.(*net.UnixConn), frame)
		if err != nil {
			t.Fatalf("call transport failed: %v", err)
		}
		if reply.OK != allowed {
			t.Fatalf("call admission expected %v reply=%+v", allowed, reply)
		}
		if allowed {
			payload, ok := reply.Result["structuredContent"].(map[string]any)
			if !ok || payload["echo"] != expected {
				t.Fatalf("wrong real MCP echo result: %+v", reply)
			}
		} else if reply.Error != "denied" {
			t.Fatalf("denied call leaked detail: %+v", reply)
		}
	}
	signed := func(seq uint64, name, text string) processcontrol.SupervisorToolCallFrame {
		f, err := processcontrol.SignSupervisorOwnedToolCallPrototype(secret, env.Challenge,
			env.Core, env.Supervisor, env.Worker, seq, name, map[string]any{"text": text})
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	first := signed(1, "echo", "first-from-core")
	call(first, true, "first-from-core")
	call(first, false, "") // same sequence replay on a NEW authenticated socket
	denied := signed(2, "forbidden", "must-not-run")
	call(denied, false, "")
	wrongWorker := signed(2, "echo", "wrong-worker")
	wrongWorker.WorkerStartUsec++
	call(wrongWorker, false, "")
	tampered := signed(2, "echo", "legitimate")
	tampered.Arguments, _ = json.Marshal(map[string]any{"text": "tampered"})
	call(tampered, false, "")
	second := signed(2, "echo", "second-from-core")
	call(second, true, "second-from-core")
	fmt.Fprintln(os.Stdout, "IPC_CORE_DONE")
	// Owned Core test process intentionally exits ONLY when its parent kills
	// this exact Go helper; the independent Supervisor retains the MCP SDK.
	select {}
}

func TestSupervisorScopedIPCActualKernelPeerAndMCPTool(t *testing.T) {
	// ALL processes are private copies of this Go test executable.
	ctx, cancel := context.WithTimeout(context.Background(), 17*time.Second)
	defer cancel()
	root, err := os.MkdirTemp("", "next-ipc-prototype-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "tool.sock")
	server, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	listener := server.(*net.UnixListener)
	workerCmd := exec.Command(os.Args[0], "-test.run=^TestMCPStdioHelperProcess$", "-test.count=1")
	workerCmd.Env = append(os.Environ(), "GO_WANT_MCP_HELPER=1")
	owned, err := processcontrol.StartSupervisorOwnedStdio(ctx, workerCmd)
	if err != nil {
		t.Fatal(err)
	}
	workerReaped := false
	t.Cleanup(func() {
		if !workerReaped {
			_ = workerCmd.Process.Kill()
			_ = workerCmd.Wait()
		}
	})
	session, err := ConnectSupervisorOwnedMCPPrototype(ctx, ServerConfig{Name: "owned-core-ipc", Transport: TransportStdio, TimeoutMS: 5000}, owned)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	worker := owned.Birth()
	if worker.ParentPID != os.Getpid() {
		t.Fatal("Worker is not a direct child of Supervisor")
	}

	coreCmd := exec.Command(os.Args[0], "-test.run=^TestSupervisorToolIPCCoreLikeHelper$", "-test.count=1")
	coreCmd.Env = append(os.Environ(), "AD_NEXT_IPC_CORE_ROLE=1")
	processcontrol.Configure(coreCmd)
	stdin, err := coreCmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := coreCmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := coreCmd.Start(); err != nil {
		t.Fatal(err)
	}
	coreReaped := false
	t.Cleanup(func() {
		_ = stdin.Close()
		if !coreReaped {
			_ = coreCmd.Process.Kill()
			_ = coreCmd.Wait()
		}
	})
	core, err := processcontrol.ObserveDirectChildBirth(ctx, os.Getpid(), coreCmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := processcontrol.ObserveBirthIdentity(ctx, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	handoff, err := processcontrol.NewSupervisorOwnedReadOnlyHandoff(core, supervisor, worker, secret)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := handoff.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	coreExit, err := processcontrol.WatchVerifiedExit(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	defer coreExit.Close()
	env := supervisorToolIPCTestEnvelope{
		Challenge: challenge, Core: core, Supervisor: supervisor, Worker: worker,
		Secret: hex.EncodeToString(secret), Socket: path,
	}
	// The test-only secret moves ONLY over an inherited private stdin pipe.
	if err := json.NewEncoder(stdin).Encode(env); err != nil {
		t.Fatal(err)
	}

	// Even a valid MAC cannot be used by a different kernel PID / UID.
	spoof, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	spoofConn, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := processcontrol.HandoffAttachMACForWorkerParent(secret, challenge, core, supervisor, worker, processcontrol.SupervisorWorkerParentSupervisor)
	if err != nil {
		t.Fatal(err)
	}
	if err := processcontrol.AcceptVerifiedSupervisorHandoff(ctx, spoofConn, handoff, signed); !errors.Is(err, processcontrol.ErrSupervisorHandoffUnavailable) {
		t.Fatalf("test supervisor spoof accepted as Core: %v", err)
	}
	_ = spoofConn.Close()
	_ = spoof.Close()

	// One verified handoff and six single-request connections, each with the
	// ACTUAL OS peer still alive until the bounded response is received.
	attachConn, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	var attach processcontrol.SupervisorHandoffAttach
	_ = attachConn.SetDeadline(time.Now().Add(5 * time.Second))
	decoder := json.NewDecoder(attachConn)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&attach); err != nil {
		t.Fatal(err)
	}
	if err := processcontrol.AcceptVerifiedSupervisorHandoff(ctx, attachConn, handoff, attach); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(attachConn, "HANDOFF_ACCEPTED"); err != nil {
		t.Fatal(err)
	}
	_ = attachConn.Close()
	scope := map[string]struct{}{"echo": {}}
	admitted := 0
	rejected := 0
	for i := 0; i < 6; i++ {
		conn, err := listener.AcceptUnix()
		if err != nil {
			t.Fatal(err)
		}
		err = ServeSupervisorScopedMCPCallPrototype(ctx, conn, handoff, session, scope)
		if err == nil {
			admitted++
		} else if errors.Is(err, processcontrol.ErrSupervisorHandoffUnavailable) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if admitted != 2 || rejected != 4 {
		t.Fatalf("unexpected scope/replay admission: accepted=%d refused=%d", admitted, rejected)
	}
	scanned := bufio.NewScanner(stdout)
	done := make(chan string, 1)
	go func() {
		if scanned.Scan() {
			done <- scanned.Text()
		} else {
			done <- ""
		}
	}()
	select {
	case value := <-done:
		if strings.TrimSpace(value) != "IPC_CORE_DONE" {
			t.Fatalf("Core fixture call sequence failed: %q", value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Core fixture did not verify tool call results")
	}
	if err := coreCmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := coreExit.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	_ = coreCmd.Wait()
	coreReaped = true

	// A replayed old MAC is useless even though the Supervisor still owns its
	// SDK session and keeps the Worker alive after Core death.
	attacker, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	signed3, err := processcontrol.SignSupervisorOwnedToolCallPrototype(secret, challenge, core, supervisor, worker, 3, "echo", map[string]any{"text": "post-crash-spoof"})
	if err != nil {
		t.Fatal(err)
	}
	spoofDone := make(chan error, 1)
	go func() {
		_, err := CallSupervisorScopedMCPPrototype(ctx, attacker.(*net.UnixConn), signed3)
		spoofDone <- err
	}()
	if err := ServeSupervisorScopedMCPCallPrototype(ctx, conn, handoff, session, scope); !errors.Is(err, processcontrol.ErrSupervisorHandoffUnavailable) {
		t.Fatalf("dead Core generation impersonated from local Supervisor peer: %v", err)
	}
	if err := <-spoofDone; err != nil {
		t.Fatal(err)
	}

	// The Supervisor may still use its OWN SDK session; the dead Core may not.
	response, err := session.Call(ctx, "echo", map[string]any{"text": "supervisor-still-owns-sdk"})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := response["structuredContent"].(map[string]any)
	if !ok || payload["echo"] != "supervisor-still-owns-sdk" {
		t.Fatalf("Supervisor lost worker on Core death: %+v", response)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := owned.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	workerReaped = true
}
