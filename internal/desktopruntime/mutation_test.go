package desktopruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopMutationLockIsOutsideRuntimeAndScoped(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	parent := t.TempDir()
	first := filepath.Join(parent, "first")
	second := filepath.Join(parent, "second")
	for _, root := range []string{first, second} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if got := desktopMutationLockPath(first); filepath.Dir(got) != parent || filepath.Base(got) == ".desktop-mutation.lock" {
		t.Fatalf("unexpected first lock path: %q", got)
	}
	if desktopMutationLockPath(first) == desktopMutationLockPath(second) {
		t.Fatal("distinct runtime roots shared a mutation lock")
	}
	release, err := AcquireDesktopMutation(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(first, ".desktop-mutation.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock was written inside runtime root: %v", err)
	}
	if _, err := os.Lstat(desktopMutationLockPath(first)); err != nil {
		t.Fatalf("sibling lock missing while held: %v", err)
	}
	release()
	if _, err := os.Lstat(desktopMutationLockPath(first)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sibling lock remained after release: %v", err)
	}
}

func TestDesktopMutationRejectsSymlinkRootWithoutTouchingTarget(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	parent := t.TempDir()
	target := filepath.Join(parent, "stable")
	link := filepath.Join(parent, "next")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := AcquireDesktopMutation(context.Background(), link); err == nil {
		t.Fatal("symlinked runtime root was accepted")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink target was touched: %+v", entries)
	}
}

func TestDesktopMutationStableUsesOriginalLockPath(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "stable")
	root := t.TempDir()
	release, err := AcquireDesktopMutation(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root, ".desktop-mutation.lock")
	if _, err := os.Lstat(legacy); err != nil {
		release()
		t.Fatalf("stable runtime-local lock missing: %v", err)
	}
	if _, err := os.Lstat(desktopMutationLockPath(root)); !errors.Is(err, os.ErrNotExist) {
		release()
		t.Fatalf("stable unexpectedly switched to Next sibling lock: %v", err)
	}
	release()
	if _, err := os.Lstat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stable lock was not released: %v", err)
	}
}
