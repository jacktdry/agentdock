//go:build darwin

package updateidentity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOwnershipRejectsWritableAndLinkedAncestors(t *testing.T) {
	root := t.TempDir()
	unsafe := filepath.Join(root, "unsafe")
	if err := os.Mkdir(unsafe, 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafe, 0777); err != nil {
		t.Fatal(err)
	}
	if err := SafePath(filepath.Join(unsafe, "missing", "transaction.json")); err == nil {
		t.Fatal("writable ancestor accepted")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if err := SafePath(filepath.Join(link, "missing")); err == nil {
		t.Fatal("linked ancestor accepted")
	}
}
