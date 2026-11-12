//go:build darwin

package client

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	processcontrol "github.com/uvwt/agentdock/internal/process"
)

func TestSupervisorOwnedMCPCoreLikeFixture(t *testing.T) {
	if os.Getenv("AD_NEXT_SUPERVISOR_MCP_CORE") == "1" {
		select {}
	}
}

func TestSupervisorOwnedMCPSDKAfterCoreCrash(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMCPStdioHelperProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "GO_WANT_MCP_HELPER=1")
	owned, err := processcontrol.StartSupervisorOwnedStdio(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	reaped := false
	t.Cleanup(func() {
		if !reaped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	root := owned.Birth()
	if root.ParentPID != os.Getpid() || root.ProcessGroupID != root.PID {
		t.Fatalf("Supervisor did not directly own verified Worker: %+v", root)
	}
	cfg := ServerConfig{Name: "supervised-mcp", Transport: TransportStdio, TimeoutMS: 5000}
	session, err := ConnectSupervisorOwnedMCPPrototype(ctx, cfg, owned)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if info := session.Info(); info.PID != root.PID || info.ServerName != "helper" ||
		info.ProtocolVersion != "2025-06-18" {
		t.Fatalf("SDK initialized with wrong process: %+v", info)
	}
	if len(session.Tools()) != 1 || session.Tools()["echo"].Name != "echo" {
		t.Fatalf("SDK tool listing unavailable: %+v", session.Tools())
	}
	if _, _, err := owned.ClaimMCPTransport(); err == nil {
		t.Fatal("attached stdio transport was published to a second SDK")
	}
	if owned.Stdin() != nil || owned.Stdout() != nil {
		t.Fatal("Supervisor-owned raw pipe escaped after exclusive MCP SDK claim")
	}
	echo := func(text string) {
		t.Helper()
		response, err := session.Call(ctx, "echo", map[string]any{"text": text})
		if err != nil {
			t.Fatal(err)
		}
		result, ok := response["structuredContent"].(map[string]any)
		if !ok || result["echo"] != text {
			t.Fatalf("wrong MCP tool reply: %+v", response)
		}
	}
	echo("before-core-crash")

	core := exec.Command(os.Args[0], "-test.run=^TestSupervisorOwnedMCPCoreLikeFixture$", "-test.count=1")
	core.Env = append(os.Environ(), "AD_NEXT_SUPERVISOR_MCP_CORE=1")
	processcontrol.Configure(core)
	if err := core.Start(); err != nil {
		t.Fatal(err)
	}
	coreReaped := false
	t.Cleanup(func() {
		if !coreReaped {
			_ = core.Process.Kill()
			_ = core.Wait()
		}
	})
	coreBirth, err := processcontrol.ObserveDirectChildBirth(ctx, os.Getpid(), core.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	watcher, err := processcontrol.WatchVerifiedExit(ctx, coreBirth)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if root.ParentPID == coreBirth.PID {
		t.Fatal("Worker incorrectly parented to Core")
	}
	if err := core.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := watcher.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	_ = core.Wait()
	coreReaped = true

	// MCP SDK still holds Supervisor-owned transport, after the Core died.
	echo("after-core-crash")
	if _, err := session.Call(ctx, "echo", nil); err == nil {
		t.Fatal("tool schema validation was bypassed in prototype")
	}
	if _, err := session.Call(ctx, "not-registered", nil); err == nil {
		t.Fatal("unknown MCP tool was accepted")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Call(ctx, "echo", map[string]any{"text": "closed"}); err == nil {
		t.Fatal("closed Supervisor MCP SDK accepted call")
	}
	if err := owned.Wait(ctx); err != nil {
		t.Fatal("natural MCP helper NOTE_EXIT or reap failed:", err)
	}
	reaped = true
	if err := owned.Wait(ctx); err != nil {
		t.Fatal("repeated Wait should not reuse PID:", err)
	}
}

func TestSupervisorOwnedMCPInvalidAdmissionPreservesFreshTransport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMCPStdioHelperProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "GO_WANT_MCP_HELPER=1")
	owned, err := processcontrol.StartSupervisorOwnedStdio(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	reaped := false
	t.Cleanup(func() {
		if !reaped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	if s, err := ConnectSupervisorOwnedMCPPrototype(ctx, ServerConfig{Name: "bad", Transport: TransportStreamableHTTP}, owned); s != nil || err == nil {
		t.Fatal("non-stdio config connected with owned pipes")
	}
	// Fail-closed config validation must occur before the one-time claim.
	session, err := ConnectSupervisorOwnedMCPPrototype(ctx, ServerConfig{Name: "valid", Transport: TransportStdio, TimeoutMS: 2500}, owned)
	if err != nil {
		t.Fatal(err)
	}
	if s, err := ConnectSupervisorOwnedMCPPrototype(ctx, ServerConfig{Name: "again", Transport: TransportStdio, TimeoutMS: 2500}, owned); s != nil || err == nil {
		t.Fatal("second ownership admission succeeded")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := owned.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	reaped = true
}

// An invalid MCP response must not publish a session or permit a second
// attachment to an already consumed Supervisor-owned worker transport.
func TestSupervisorOwnedMCPBrokenHandshakeFixture(t *testing.T) {
	if os.Getenv("AD_NEXT_BAD_MCP_HANDSHAKE") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "not-a-valid-json-rpc-frame")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestSupervisorOwnedMCPHandshakeRefusalKeepsOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestSupervisorOwnedMCPBrokenHandshakeFixture$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AD_NEXT_BAD_MCP_HANDSHAKE=1")
	owned, err := processcontrol.StartSupervisorOwnedStdio(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	reaped := false
	t.Cleanup(func() {
		if !reaped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	session, err := ConnectSupervisorOwnedMCPPrototype(ctx, ServerConfig{Name: "bad-handshake", Transport: TransportStdio, TimeoutMS: 800}, owned)
	if session != nil || err == nil {
		t.Fatal("malformed helper published an MCP SDK session")
	}
	if _, _, err := owned.ClaimMCPTransport(); err == nil {
		t.Fatal("failed initialization relinquished one-time transport claim")
	}
	// Invalid protocol output may result in SIGPIPE/nonzero exit when SDK
	// closes its pipe. It must STILL be OS-witnessed and reaped; do not
	// reinterpret an abnormal exit as clean or permit a second claim.
	_ = owned.Wait(ctx)
	if cmd.ProcessState == nil {
		t.Fatal("Supervisor failed to reap rejected MCP helper")
	}
	reaped = true
}
