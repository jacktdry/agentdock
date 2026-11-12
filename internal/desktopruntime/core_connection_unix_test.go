//go:build darwin || linux

package desktopruntime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadCoreConnectionUsesPrivateLoopbackRuntimeConfig(t *testing.T) {
	root := t.TempDir()
	envPath := filepath.Join(root, "agentdock.env")
	env := "AGENTDOCK_HOST=0.0.0.0" + "\n" +
		"AGENTDOCK_PORT=9876" + "\n" +
		"AGENTDOCK_AUTH_TOKEN='secret-token'" + "\n"
	if err := os.WriteFile(envPath, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := unixRuntimeManifest{
		SchemaVersion:   1,
		AgentDockBinary: filepath.Join(root, "agentdock"),
		EnvironmentFile: envPath,
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	connection, err := ReadCoreConnection(root)
	if err != nil {
		t.Fatal(err)
	}
	if connection.Endpoint() != "http://127.0.0.1:9876" || connection.AuthToken() != "secret-token" {
		t.Fatalf("connection = endpoint %q token %q", connection.Endpoint(), connection.AuthToken())
	}
}

func TestReadCoreConnectionRejectsNonLoopbackHost(t *testing.T) {
	root := t.TempDir()
	envPath := filepath.Join(root, "agentdock.env")
	env := "AGENTDOCK_HOST=192.0.2.10" + "\n" +
		"AGENTDOCK_PORT=9876" + "\n" +
		"AGENTDOCK_AUTH_TOKEN='secret-token'" + "\n"
	if err := os.WriteFile(envPath, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := unixRuntimeManifest{
		SchemaVersion:   1,
		AgentDockBinary: filepath.Join(root, "agentdock"),
		EnvironmentFile: envPath,
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadCoreConnection(root); err == nil {
		t.Fatal("non-loopback Core host was accepted")
	}
}
