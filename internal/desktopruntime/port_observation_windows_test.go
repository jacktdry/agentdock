//go:build windows

package desktopruntime

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/updateengine"
)

func TestPortWindowsRequiresSelectedActiveGeneration(t *testing.T) {
	request := portFixtureRequest(t, 0)
	root := request.Runtime.Root
	layout, err := updateengine.NewWindowsLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	request.Runtime.Binary = layout.GenerationCore("v0.9.0")
	if err := os.MkdirAll(filepath.Dir(request.Runtime.Binary), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(request.Runtime.Binary, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{SchemaVersion: SchemaVersion, InstallRoot: root, AgentDockBinary: layout.CoreShim(), Host: "127.0.0.1", Port: 18765,
		LocalMCPURL: "http://127.0.0.1:18765/mcp", TunnelMode: "none"}
	if err := Save(filepath.Join(root, "runtime.json"), manifest); err != nil {
		t.Fatal(err)
	}
	if windowsPortSelection(request.Runtime) {
		t.Fatal("missing active generation accepted")
	}
	store, err := updateengine.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteActive(updateengine.ActiveVersion{SchemaVersion: updateengine.SchemaVersion, ActiveVersion: "v0.9.0", State: updateengine.StateCommitted, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if windowsPortSelection(request.Runtime) {
		t.Fatal("generation without explicit Next variant accepted")
	}
	request.Runtime.Binary = layout.CoreShim()
	if windowsPortSelection(request.Runtime) {
		t.Fatal("legacy shim accepted")
	}
	request.Runtime.Binary = layout.GenerationCore("v0.8.0")
	if windowsPortSelection(request.Runtime) {
		t.Fatal("inactive generation accepted")
	}
}
