//go:build darwin || linux

package desktopruntime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCoreBinaryForRuntimeUsesSelectedManifest(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "managed-agentdock")
	if err := os.WriteFile(binary, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := unixRuntimeManifest{
		SchemaVersion:   1,
		AgentDockBinary: binary,
		EnvironmentFile: filepath.Join(root, "agentdock.env"),
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := CoreBinaryForRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != binary {
		t.Fatalf("core binary = %q, want %q", got, binary)
	}
}
