//go:build darwin || linux

package selfupdate

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeOptionsForTargetReadsManagedCoreVersion(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "agentdock")
	script := "#!/bin/sh\nprintf 'AgentDock v0.8.7\\n'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	opts, err := runtimeOptionsForTarget(context.Background(), io.Discard, binary, root)
	if err != nil {
		t.Fatal(err)
	}
	if opts.CurrentVersion != "v0.8.7" {
		t.Fatalf("current version = %q", opts.CurrentVersion)
	}
	if opts.ExecutablePath != binary {
		t.Fatalf("executable = %q, want %q", opts.ExecutablePath, binary)
	}
}
