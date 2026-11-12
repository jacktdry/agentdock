//go:build !windows

package permission

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPermissionPrivateModeDriftFailsClosed(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "private-mode-epoch")
	statePath := store.StatePath()
	stateDir := filepath.Dir(statePath)
	if err := os.Chmod(statePath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.VerifyDiskState(); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("world-readable state was trusted: %v", err)
	}
	if err := os.Chmod(statePath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.VerifyDiskState(); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("world-traversable permissions directory was trusted: %v", err)
	}
	if err := os.Chmod(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.VerifyDiskState(); err != nil {
		t.Fatalf("repaired private mode rejected: %v", err)
	}
}
