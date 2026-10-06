//go:build darwin

package commandpath

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMacOSDiscovery(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	local := filepath.Join(home, ".local", "bin")
	brew := filepath.Join(root, "opt", "homebrew", "bin")
	usrLocal := filepath.Join(root, "usr", "local", "bin")
	inherited := filepath.Join(root, "inherited")
	dirs := []string{local, brew, usrLocal, inherited}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "codex"), nil, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	path := searchPath(inherited+":"+brew, home, []string{brew, usrLocal})
	if want := strings.Join(dirs, ":"); path != want {
		t.Fatalf("path=%q want=%q", path, want)
	}
	// Removing each preferred candidate proves precedence and inherited PATH fallback.
	for _, dir := range dirs {
		got, err := lookPathIn("codex", path)
		if err != nil || got != filepath.Join(dir, "codex") {
			t.Fatalf("got=%q err=%v", got, err)
		}
		if err := os.Remove(got); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := lookPathIn("codex", path); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("missing CLI: %v", err)
	}
}

func TestLookPathUsesInheritedPATH(t *testing.T) {
	// A unique name prevents real user/system CLI installations from affecting the test.
	name := "agentdock-codex-path-fixture"
	dir := t.TempDir()
	want := filepath.Join(dir, name)
	if err := os.WriteFile(want, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", dir)
	got, err := LookPath(name)
	if err != nil || got != want {
		t.Fatalf("got=%q err=%v", got, err)
	}
}
