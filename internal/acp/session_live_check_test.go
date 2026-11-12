package acp

import (
	"context"
	"testing"
)

func TestIsLiveManagedSessionRequiresLoadedCurrentAdapter(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	m, err := newTestManager(home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	if m.IsLiveManagedSession("unknown") || m.IsLiveManagedSession("") {
		t.Fatal("unknown session accepted")
	}
	created, err := m.NewSession(context.Background(), workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := created.Session.ID
	if !m.IsLiveManagedSession(id) {
		t.Fatalf("new Session %q not loaded", id)
	}
	if _, err := m.CloseSession(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if m.IsLiveManagedSession(id) {
		t.Fatal("closed Session passed active probe")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if m.IsLiveManagedSession(id) {
		t.Fatal("closed ACP Manager passed probe")
	}
}
