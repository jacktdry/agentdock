//go:build darwin

package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeDarwinPortFixture(t *testing.T, request PortObservationRequest, label string) {
	t.Helper()
	data, err := json.Marshal(unixRuntimeManifest{SchemaVersion: 1, ServiceName: label, ServiceManager: "smappservice", AgentDockBinary: request.Runtime.Binary})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(request.Runtime.Root, "desktop-runtime.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPortDarwinManifestRequiresNextServiceAndExactBinary(t *testing.T) {
	request := portFixtureRequest(t, 0)
	writeDarwinPortFixture(t, request, "com.uvwt.agentdock.core")
	if darwinPortSelection(request.Runtime) {
		t.Fatal("stable fixture accepted")
	}
	writeDarwinPortFixture(t, request, "dev.dropabit.agentdock.next.core")
	if !darwinPortSelection(request.Runtime) {
		t.Fatal("Next fixture rejected")
	}
	request.Runtime.Binary += "-other"
	if darwinPortSelection(request.Runtime) {
		t.Fatal("another binary accepted")
	}
	for _, output := range []string{"", "0", "123 garbage"} {
		if _, err := darwinPortPIDs(output); err == nil {
			t.Fatalf("invalid PID evidence accepted: %q", output)
		}
	}
	pids, err := darwinPortPIDs("123\n123\n")
	if err != nil || len(pids) != 1 {
		t.Fatal("duplicate socket records should be one process", pids, err)
	}
}

func TestPortDarwinForeignTempListener(t *testing.T) {
	listener := tempPortListener(t)
	request := portFixtureRequest(t, listener.Addr().(*net.TCPAddr).Port)
	writeDarwinPortFixture(t, request, "dev.dropabit.agentdock.next.core")
	// !cgo cannot read kernel process identity and must return unknown.
	_, _, processErr := platformPortProcess(os.Getpid())
	if processErr != nil {
		assertDarwinPortFixture(t, request, PortUnknown, "ownership_unavailable")
	} else {
		assertDarwinPortFixture(t, request, PortConflict, "foreign_listener")
	}
}

func assertDarwinPortFixture(t *testing.T, request PortObservationRequest, state PortState, reason string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := time.Now()
	observation := ObservePort(ctx, request)
	t.Logf("native observation: %s, state=%s reason=%s", time.Since(started), observation.State, observation.ReasonCode)
	// Host load may exhaust the native scan budget. Accept only an explicit,
	// independently expired deadline as conservative rejection, never ownership.
	if observation.State == PortUnknown && observation.ReasonCode == "observation_cancelled" && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return
	}
	if observation.State != state || observation.ReasonCode != reason {
		t.Fatal(observation)
	}
}

func TestPortDarwinOwnedSelectedProcessFixture(t *testing.T) {
	listener := tempPortListener(t)
	request := portFixtureRequest(t, listener.Addr().(*net.TCPAddr).Port)
	path, instance, err := platformPortProcess(os.Getpid())
	if err != nil {
		if _, captureErr := PortProcessInstance(os.Getpid()); captureErr == nil {
			t.Fatal("unproven process accepted")
		}
		return
	}
	// The test executable stands in for the selected Core; never inspect a real
	// AgentDock process, app bundle, environment, or launchd service.
	request.Runtime.Binary, request.Runtime.PID, request.Runtime.ProcessInstance = path, os.Getpid(), instance
	writeDarwinPortFixture(t, request, "dev.dropabit.agentdock.next.core")
	assertDarwinPortFixture(t, request, PortOwnedByNext, "selected_next_process")
	request.Runtime.ProcessInstance += "-stale"
	assertDarwinPortFixture(t, request, PortUnknown, "process_instance_mismatch")
}

func TestPortDarwinCancelledOwnershipProbe(t *testing.T) {
	listener := tempPortListener(t)
	request := portFixtureRequest(t, listener.Addr().(*net.TCPAddr).Port)
	writeDarwinPortFixture(t, request, "dev.dropabit.agentdock.next.core")
	for _, deadline := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if deadline {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		} else {
			cancel()
		}
		state, reason := platformPortOwner(ctx, request)
		cancel()
		if state != PortUnknown || reason != "observation_cancelled" {
			t.Fatalf("cancelled native probe accepted or misclassified: %s %s", state, reason)
		}
	}
}
