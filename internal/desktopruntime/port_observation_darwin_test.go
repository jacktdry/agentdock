//go:build darwin

package desktopruntime

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
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
	observation := ObservePort(context.Background(), request)
	// !cgo cannot read kernel process identity and must return unknown.
	_, _, processErr := platformPortProcess(os.Getpid())
	if processErr != nil {
		if observation.State != PortUnknown {
			t.Fatal(observation)
		}
	} else if observation.State != PortConflict {
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
	if observation := ObservePort(context.Background(), request); observation.State != PortOwnedByNext {
		t.Fatal(observation)
	}
	request.Runtime.ProcessInstance += "-stale"
	if observation := ObservePort(context.Background(), request); observation.State != PortUnknown {
		t.Fatal("reused process accepted", observation)
	}
}
