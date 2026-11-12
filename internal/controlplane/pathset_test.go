package controlplane

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathSetContainsIntersectsAndResolvesSymlinks(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "state", "permissions")
	if err := os.MkdirAll(protected, 0o700); err != nil {
		t.Fatal(err)
	}
	set, err := NewPathSet(protected)
	if err != nil {
		t.Fatal(err)
	}
	if !set.Contains(filepath.Join(protected, "state.json")) {
		t.Fatal("protected child was not contained")
	}
	if !set.Intersects(filepath.Join(root, "state")) {
		t.Fatal("protected ancestor was not intersecting")
	}
	if set.Contains(filepath.Join(root, "state")) {
		t.Fatal("ancestor must not be treated as contained")
	}
	if set.Intersects(filepath.Join(root, "unrelated")) {
		t.Fatal("unrelated path intersected protected root")
	}

	alias := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Join(root, "state"), alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	viaAlias, err := canonicalPath(filepath.Join(alias, "permissions"))
	if err != nil {
		t.Fatal(err)
	}
	if !set.Contains(viaAlias) {
		t.Fatalf("symlink alias did not resolve into protected root: %s", viaAlias)
	}
	if !set.Contains(filepath.Join(alias, "permissions", "state.json")) {
		t.Fatal("raw symlink alias bypassed protected child")
	}
	if !set.Intersects(alias) {
		t.Fatal("raw symlink alias bypassed protected ancestor")
	}
	if got := set.Descendants(alias); len(got) != 1 || got[0] != viaAlias {
		t.Fatalf("raw symlink alias omitted protected descendant: %v", got)
	}
}

func TestPathSetCanonicalizesMissingProtectedLeaf(t *testing.T) {
	root := t.TempDir()
	set, err := NewPathSet(filepath.Join(root, "future", "credential"))
	if err != nil {
		t.Fatal(err)
	}
	if !set.Contains(filepath.Join(root, "future", "credential")) {
		t.Fatal("missing protected leaf was not canonicalized")
	}
}
