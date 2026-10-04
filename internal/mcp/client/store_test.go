package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRejectsTrailingRegistryJSON(t *testing.T) {
	store := newStore(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"version":1,"servers":[]} {"version":1,"servers":[]}`)
	if err := os.WriteFile(store.path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.load(); err == nil {
		t.Fatal("registry with trailing JSON was accepted")
	}
}

func TestStoreRejectsOversizedRegistryBeforeDecode(t *testing.T) {
	store := newStore(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		t.Fatal(err)
	}
	data := `{"version":1,"servers":[]}` + strings.Repeat(" ", (1<<20)+1)
	if err := os.WriteFile(store.path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.load(); err == nil {
		t.Fatal("oversized registry was accepted")
	}
}

func TestStoreValidatesProtocolVersionPin(t *testing.T) {
	valid := ServerConfig{Name: "legacy", Description: "legacy", Transport: TransportStreamableHTTP, URL: "http://127.0.0.1:8766/mcp", Enabled: true, TimeoutMS: 30000, ProtocolVersion: "2025-11-25"}
	if err := validateServerConfig(valid); err != nil {
		t.Fatalf("valid protocol pin rejected: %v", err)
	}
	invalid := valid
	invalid.ProtocolVersion = "2027-01-01"
	if err := validateServerConfig(invalid); err == nil || !strings.Contains(err.Error(), "unsupported protocol_version") {
		t.Fatalf("invalid protocol pin error=%v", err)
	}
}
