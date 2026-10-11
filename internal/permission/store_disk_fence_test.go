package permission

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyDiskStateDetectsOtherCoreEpochPolicyAndTampering(t *testing.T) {
	root := t.TempDir()
	first, err := NewStore(root, "core-a-epoch")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.VerifyDiskState(); err != nil {
		t.Fatal(err)
	}
	second, err := NewStore(root, "core-b-epoch")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.VerifyDiskState(); !errors.Is(err, ErrRevision) {
		t.Fatalf("stale in-memory Core was trusted: %v", err)
	}
	if err := second.VerifyDiskState(); err != nil {
		t.Fatal(err)
	}
	policy := second.Policy()
	next := policy
	next.GlobalMode = ReadOnly
	if _, err := second.ReplacePolicy(policy.Revision, next); err != nil {
		t.Fatal(err)
	}
	if err := second.VerifyDiskState(); err != nil {
		t.Fatal(err)
	}
	third, err := NewStore(root, "core-b-epoch")
	if err != nil {
		t.Fatal(err)
	}
	policy = third.Policy()
	next = policy
	next.GlobalMode = Full
	if _, err := third.ReplacePolicy(policy.Revision, next); err != nil {
		t.Fatal(err)
	}
	if err := second.VerifyDiskState(); !errors.Is(err, ErrRevision) {
		t.Fatalf("in-memory stale policy accepted: %v", err)
	}
	path := filepath.Join(root, "permissions", "state.json")
	backup := filepath.Join(root, "state-backup")
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(backup, path); err != nil {
		t.Fatal(err)
	}
	if err := third.VerifyDiskState(); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("symlink state accepted: %v", err)
	}
}
