//go:build !darwin

package acp

import (
	"context"
	"errors"
	"testing"

	"github.com/uvwt/agentdock/internal/process"
)

// No non-Darwin process birth observer is implemented. A real locally spawned
// Adapter and loaded ACP Session must not be mistaken for OS birth evidence.
// In particular, allowEmpty only relaxes the loaded-Session precondition; it
// must never relax the process identity requirement.
func TestMCPDiagnosticScopeUnsupportedPlatformFailsClosed(t *testing.T) {
	workspace := t.TempDir()
	manager, err := newTestManager(t.TempDir(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Errorf("close owned test adapter: %v", err)
		}
	})
	created, err := manager.NewSession(context.Background(), workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created.Session.ID == "" || manager.process == nil || manager.process.pid <= 0 ||
		manager.process.command == nil || manager.process.command.Process == nil ||
		manager.process.command.Process.Pid != manager.process.pid || manager.process.connection == nil {
		t.Fatal("test prerequisite: missing real owned Adapter process or loaded Session")
	}
	if identity, err := process.ObserveBirthIdentity(context.Background(), manager.process.pid); !errors.Is(err, process.ErrIdentityUnavailable) || identity != (process.BirthIdentity{}) {
		t.Fatalf("unsupported OS must not manufacture birth evidence: %+v %v", identity, err)
	}

	called := 0
	callback := func(context.Context, diagnosticManagerScope) error {
		called++
		return nil
	}
	if err := manager.withMCPDiagnosticScope(context.Background(), callback); !errors.Is(err, errMCPDiagnosticNotQualified) || called != 0 {
		t.Fatalf("loaded Session incorrectly qualified without OS evidence: err=%v calls=%d", err, called)
	}
	if err := manager.withMCPDiagnosticScopePolicy(context.Background(), true, callback); !errors.Is(err, errMCPDiagnosticNotQualified) || called != 0 {
		t.Fatalf("allowEmpty bypassed birth evidence: err=%v calls=%d", err, called)
	}
	manager.mu.Lock()
	manager.loaded = make(map[string]sessionLifecycleResponse)
	manager.mu.Unlock()
	if err := manager.withMCPDiagnosticScopePolicy(context.Background(), true, callback); !errors.Is(err, errMCPDiagnosticNotQualified) || called != 0 {
		t.Fatalf("empty early scope bypassed birth evidence: err=%v calls=%d", err, called)
	}
}
