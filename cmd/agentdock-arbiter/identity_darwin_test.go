//go:build darwin

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNextArbiterRejectsWrongRootBeforeReadOrLock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, marker := range []string{"next", "unknown", ""} {
		t.Setenv("AGENTDOCK_DESKTOP_VARIANT", marker)
		root := filepath.Join(home, "Library", "Application Support", "AgentDock")
		if marker == "" {
			root = filepath.Join(home, "Library", "Application Support", "AgentDock Next")
		}
		if err := run(context.Background(), []string{"--root", root, "--transaction-id", "fixture", "--recover-if-unlocked"}); err == nil {
			t.Fatal("unsafe root accepted")
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatal("root created before validation", err)
		}
	}
}
