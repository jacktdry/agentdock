//go:build windows

package permission

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPermissionWindowsReplaceRefusesCrossDirectoryMove(t *testing.T) {
	root := t.TempDir()
	left := filepath.Join(root, "left")
	right := filepath.Join(root, "right")
	if err := os.Mkdir(left, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(right, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(left, "state.tmp")
	target := filepath.Join(right, "state.json")
	if err := os.WriteFile(source, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replacePermissionState(source, target, right); err == nil {
		t.Fatal("cross-directory move bypassed atomic same-directory boundary")
	}
	if content, err := os.ReadFile(source); err != nil || string(content) != "original" {
		t.Fatalf("rejected source was changed: %q, %v", content, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("rejected target exists: %v", err)
	}
}
