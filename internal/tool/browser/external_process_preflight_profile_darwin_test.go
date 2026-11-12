//go:build darwin

package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMacEdgeProfileMetadataNegativeTempOnly(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("content must never be read"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, link, filepath.Join(dir, "missing")} {
		m, err := (macEdgeProcessProbe{}).profileMetadata(context.Background(), path)
		if err != errEdgeProcessUnqualified || m != (edgeProfileMetadata{}) {
			t.Fatal("native unsafe metadata accepted")
		}
	}
}
