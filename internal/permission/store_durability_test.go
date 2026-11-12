package permission

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPermissionReplaceFailurePreservesDiskAndRequiresReload(t *testing.T) {
	root := t.TempDir()
	store := newPermissionStore(t, root, "durability-epoch")
	before := readStateFile(t, store.StatePath())
	failure := errors.New("injected replace failure")
	called := false
	store.replaceState = func(tempPath, statePath, stateDir string) error {
		called = true
		if filepath.Dir(tempPath) != stateDir || statePath != store.StatePath() {
			t.Fatal("replacement escaped permission directory")
		}
		if _, err := os.Stat(tempPath); err != nil {
			t.Fatalf("synchronized temp file missing: %v", err)
		}
		return failure
	}
	policy := store.Policy()
	policy.GlobalMode = ReadOnly
	if _, err := store.ReplacePolicy(policy.Revision, policy); !errors.Is(err, failure) {
		t.Fatalf("injected replacement error: %v", err)
	}
	if !called || store.state.Policy.Revision != before.Policy.Revision || readStateFile(t, store.StatePath()).Revision != before.Revision {
		t.Fatal("failed replacement changed disk or in-memory policy")
	}
	if err := store.VerifyDiskState(); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("uncertain replacement must quarantine Store: %v", err)
	}
	if _, err := store.Snapshot(); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("quarantined Store returned state: %v", err)
	}
	if _, err := Evaluate(store.Policy(), PermissionFacts{}); err == nil {
		t.Fatal("quarantined Store returned an authorizing policy")
	}
	matches, err := filepath.Glob(filepath.Join(root, "permissions", ".state.json.tmp.*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("failed replacement leaked temporary files: %v, %v", matches, err)
	}
	store.replaceState = nil
	if _, err := store.ReplacePolicy(policy.Revision, policy); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("uncertain Store should not retry silently: %v", err)
	}
	reopened := newPermissionStore(t, root, "durability-epoch")
	if _, err := reopened.ReplacePolicy(policy.Revision, policy); err != nil {
		t.Fatalf("replacement after verified reload: %v", err)
	}
	if err := reopened.VerifyDiskState(); err != nil {
		t.Fatalf("replacement after reload not verified: %v", err)
	}
	reopened = newPermissionStore(t, root, "durability-epoch")
	if reopened.Policy().GlobalMode != ReadOnly {
		t.Fatalf("replacement did not survive reload: %+v", reopened.Policy())
	}
}

// Even a successful rename may be followed by an error (e.g. Unix directory
// fsync failure). Returning stale in-memory approvals in that case is unsafe.
func TestPermissionAmbiguousPostReplaceErrorQuarantinesStore(t *testing.T) {
	root := t.TempDir()
	store := newPermissionStore(t, root, "ambiguous-epoch")
	injected := errors.New("injected post-replace sync failure")
	store.replaceState = func(tempPath, statePath, stateDir string) error {
		if err := replacePermissionState(tempPath, statePath, stateDir); err != nil {
			return err
		}
		return injected
	}
	before := store.Policy()
	next := before
	next.GlobalMode = ReadOnly
	if _, err := store.ReplacePolicy(before.Revision, next); !errors.Is(err, injected) {
		t.Fatalf("unexpected ambiguous replacement error: %v", err)
	}
	if _, err := store.Snapshot(); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("ambiguous write allowed stale in-memory state: %v", err)
	}
	if err := store.VerifyDiskState(); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("ambiguous write trusted disk/memory drift: %v", err)
	}
	if _, err := Evaluate(store.Policy(), PermissionFacts{}); err == nil {
		t.Fatal("ambiguous write exposed stale policy")
	}
	reopened := newPermissionStore(t, root, "ambiguous-epoch")
	if reopened.Policy().GlobalMode != ReadOnly || reopened.Policy().Revision != before.Revision+1 {
		t.Fatalf("reload did not resolve the published state: %+v", reopened.Policy())
	}
	if err := reopened.VerifyDiskState(); err != nil {
		t.Fatalf("reloaded Store remained untrusted: %v", err)
	}
}
