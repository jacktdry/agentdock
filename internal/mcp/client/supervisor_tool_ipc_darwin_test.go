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

	"github.com/uvwt/agentdock/internal/permission"
	processcontrol "github.com/uvwt/agentdock/internal/process"
)

type supervisorToolIPCTestEnvelope struct {
	Challenge  processcontrol.SupervisorHandoffChallenge `json:"challenge"`
	Core       processcontrol.BirthIdentity              `json:"core"`
	Supervisor processcontrol.BirthIdentity              `json:"supervisor"`
	Worker     processcontrol.BirthIdentity              `json:"worker"`
	Secret     string                                    `json:"secret"`
	Socket     string                                    `json:"socket"`
	M8         bool                                      `json:"m8"`
	Durable    bool                                      `json:"durable"`
	CoreACK    bool                                      `json:"core_ack"`
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
	var acknowledgments []processcontrol.SupervisorDispatchAckFrame
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
			if env.CoreACK {
				ack, err := processcontrol.SignSupervisorDispatchAckPrototype(secret, env.Core,
					env.Supervisor, env.Worker, frame, reply)
				if err != nil {
					t.Fatal(err)
				}
				fresh, err := net.Dial("unix", env.Socket)
				if err != nil {
					t.Fatal(err)
				}
				if err := SendSupervisorCoreACKPrototype(context.Background(), fresh.(*net.UnixConn), frame, ack); err != nil {
					t.Fatalf("authenticated Core ACK failed: %v", err)
				}
				acknowledgments = append(acknowledgments, ack)
			}
		} else if reply.Error != "denied" {
			t.Fatalf("denied call leaked detail: %+v", reply)
		}
	}
	var coreAdmission CoreSupervisorM8Context
	if env.M8 {
		store, err := permission.NewStore(t.TempDir(), "core-m8-test-epoch")
		if err != nil {
			t.Fatal(err)
		}
		gate, err := permission.NewAdmissionGate(store)
		if err != nil {
			t.Fatal(err)
		}
		coreAdmission = CoreSupervisorM8Context{Gate: gate,
			Binding: permission.PermissionBinding{
				Principal:    permission.AuthorizationPrincipal{Kind: "native-test-sso", ID: "trusted-core-user", Authenticated: true, Stable: true},
				RuntimeEpoch: store.RuntimeEpoch(), Source: "supervisor_core", Provider: "test-mcp",
			}, Audit: permission.AuditBinding{CallID: "initial"},
		}
	}
	signed := func(seq uint64, name, text string) processcontrol.SupervisorToolCallFrame {
		if env.M8 {
			coreAdmission.Audit.CallID = fmt.Sprintf("native-m8-%d-%s", seq, name)
			frame, decision, err := AdmitAndSignSupervisorM8Call(context.Background(), coreAdmission, secret, env.Challenge,
				env.Core, env.Supervisor, env.Worker, seq, name, map[string]any{"text": text})
			if err != nil || decision.Decision.Effect != permission.Allow {
				t.Fatalf("M8 trusted Core did not admit: %+v %v", decision, err)
			}
			return frame
		}
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
	if env.M8 {
		// Even an attested Core may not bypass AdmissionGate by using the
		// earlier allowlist-only signer against the M8 endpoint.
		legacy, err := processcontrol.SignSupervisorOwnedToolCallPrototype(secret, env.Challenge,
			env.Core, env.Supervisor, env.Worker, 2, "echo", map[string]any{"text": "legacy"})
		if err != nil {
			t.Fatal(err)
		}
		call(legacy, false, "")
		modifiedProof := signed(2, "echo", "proof-changed")
		modifiedProof.Permission.PolicyRevision++
		call(modifiedProof, false, "")
	}
	second := signed(2, "echo", "second-from-core")
	call(second, true, "second-from-core")
	fmt.Fprintln(os.Stdout, "IPC_CORE_DONE")
	if env.Durable {
		rawFirst, err := json.Marshal(first)
		if err != nil {
			t.Fatal(err)
		}
		rawSecond, err := json.Marshal(second)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(os.Stdout, string(rawFirst))
		fmt.Fprintln(os.Stdout, string(rawSecond))
		if env.CoreACK {
			if len(acknowledgments) != 2 {
				t.Fatalf("unexpected ACK count %d", len(acknowledgments))
			}
			for _, ack := range acknowledgments {
				raw, err := json.Marshal(ack)
				if err != nil {
					t.Fatal(err)
				}
				fmt.Fprintln(os.Stdout, string(raw))
			}
		}
	}
	// Owned Core test process intentionally exits ONLY when its parent kills
	// this exact Go helper; the independent Supervisor retains the MCP SDK.
	select {}
}

func TestSupervisorScopedIPCActualKernelPeerAndMCPTool(t *testing.T) {
	runSupervisorScopedIPCTest(t, false, false, false)
}

func TestSupervisorM8ScopedIPCActualKernelPeerAndMCPTool(t *testing.T) {
	runSupervisorScopedIPCTest(t, true, false, false)
}

func TestSupervisorM8DurableScopedIPCActualKernelPeerAndMCPTool(t *testing.T) {
	runSupervisorScopedIPCTest(t, true, true, false)
}
func TestSupervisorM8DurableCoreACKActualKernelPeer(t *testing.T) {
	runSupervisorScopedIPCTest(t, true, true, true)
}
func runSupervisorScopedIPCTest(t *testing.T, m8, durable, coreACK bool) {
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
	var ledger *processcontrol.SupervisorDispatchLedger
	if durable {
		privateDir := filepath.Join(root, "dispatch")
		if err := os.Mkdir(privateDir, 0700); err != nil {
			t.Fatal(err)
		}
		var openErr error
		ledger, openErr = processcontrol.OpenSupervisorDispatchLedger(privateDir)
		if openErr != nil {
			t.Fatal(openErr)
		}
		defer ledger.Close()
	}
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
		Secret: hex.EncodeToString(secret), Socket: path, M8: m8, Durable: durable, CoreACK: coreACK,
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

	// Only NOW let the real Core open its handoff socket. Never race the
	// fake-peer negative assertion with the genuine Core in AcceptUnix.
	if err := json.NewEncoder(stdin).Encode(env); err != nil {
		t.Fatal(err)
	}

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
	calls := 6
	if m8 {
		calls = 8
	}
	for i := 0; i < calls; i++ {
		conn, err := listener.AcceptUnix()
		if err != nil {
			t.Fatal(err)
		}
		if durable {
			err = ServeSupervisorM8DurableMCPCallPrototype(ctx, conn, handoff, session, scope, ledger)
		} else if m8 {
			err = ServeSupervisorM8ScopedMCPCallPrototype(ctx, conn, handoff, session, scope)
		} else {
			err = ServeSupervisorScopedMCPCallPrototype(ctx, conn, handoff, session, scope)
		}
		if err == nil {
			admitted++
			if coreACK {
				ackConn, err := listener.AcceptUnix()
				if err != nil {
					t.Fatal(err)
				}
				if err := ServeSupervisorCoreACKPrototype(ctx, ackConn, handoff, ledger); err != nil {
					t.Fatalf("Core received reply but ACK rejected: %v", err)
				}
			}
		} else if errors.Is(err, processcontrol.ErrSupervisorHandoffUnavailable) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	expectedRejected := 4
	if m8 {
		expectedRejected = 6
	}
	if admitted != 2 || rejected != expectedRejected {
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
	if durable {
		acknowledgedRequests := make([]processcontrol.SupervisorToolCallFrame, 0, 2)
		for _, sequence := range []uint64{1, 2} {
			if !scanned.Scan() {
				t.Fatal("Core did not return signed frame for durable test")
			}
			var frame processcontrol.SupervisorToolCallFrame
			if err := json.Unmarshal(scanned.Bytes(), &frame); err != nil {
				t.Fatal(err)
			}
			if frame.Sequence != sequence {
				t.Fatal("wrong signed Core sequence")
			}
			acknowledgedRequests = append(acknowledgedRequests, frame)
			snapshot, err := processcontrol.InspectSupervisorDispatchReceipt(filepath.Join(root, "dispatch"), frame)
			if err != nil || snapshot.State != "settled" || snapshot.Outcome != "succeeded" ||
				snapshot.Core != core || snapshot.Worker != worker || len(snapshot.ReplySHA256) != 64 || snapshot.Acknowledged != coreACK {
				t.Fatalf("successful Core IPC did not durably settle before reply: %+v %v", snapshot, err)
			}
		}
		if coreACK {
			for _, frame := range acknowledgedRequests {
				if !scanned.Scan() {
					t.Fatal("Core did not report its signed ACK")
				}
				var ack processcontrol.SupervisorDispatchAckFrame
				if err := json.Unmarshal(scanned.Bytes(), &ack); err != nil {
					t.Fatal(err)
				}
				if ack.Sequence != frame.Sequence {
					t.Fatal("ACK sequence mismatch")
				}
				stalePeer, err := net.Dial("unix", path)
				if err != nil {
					t.Fatal(err)
				}
				received, err := listener.AcceptUnix()
				if err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- SendSupervisorCoreACKPrototype(ctx, stalePeer.(*net.UnixConn), frame, ack) }()
				if err := ServeSupervisorCoreACKPrototype(ctx, received, handoff, ledger); err == nil {
					t.Fatal("forged Core ACK accepted from wrong process")
				}
				if err := <-done; err == nil {
					t.Fatal("forged ACK received positive response")
				}
			}
		}
		entries, err := os.ReadDir(filepath.Join(root, "dispatch"))
		if err != nil || len(entries) != 2 {
			t.Fatalf("rejected calls incorrectly persisted or receipts missing: %d %v", len(entries), err)
		}
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
	var refusedErr error
	if durable {
		refusedErr = ServeSupervisorM8DurableMCPCallPrototype(ctx, conn, handoff, session, scope, ledger)
	} else if m8 {
		refusedErr = ServeSupervisorM8ScopedMCPCallPrototype(ctx, conn, handoff, session, scope)
	} else {
		refusedErr = ServeSupervisorScopedMCPCallPrototype(ctx, conn, handoff, session, scope)
	}
	if !errors.Is(refusedErr, processcontrol.ErrSupervisorHandoffUnavailable) {
		t.Fatalf("dead Core generation impersonated from local Supervisor peer: %v", refusedErr)
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
