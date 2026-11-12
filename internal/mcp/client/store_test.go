package client

import (
	"encoding/json"
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

func TestStoreVersionOneMetadataIsStableAndUpgradesOnWrite(t *testing.T) {
	store := newStore(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := registryFile{
		Version: 1,
		Servers: []ServerConfig{{
			Name: "legacy", Description: "Legacy", Transport: TransportStreamableHTTP,
			URL: "https://example.invalid/mcp", Enabled: true,
		}},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := store.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != second.Revision || !validToken(first.Revision) {
		t.Fatalf("legacy revision unstable: first=%q second=%q", first.Revision, second.Revision)
	}
	firstGeneration := first.Servers["legacy"].Generation
	if firstGeneration != second.Servers["legacy"].Generation || !validToken(firstGeneration) {
		t.Fatalf("legacy generation unstable: first=%q second=%q", firstGeneration, second.Servers["legacy"].Generation)
	}

	updated, err := store.update(func(snapshot RegistrySnapshot) error {
		cfg := snapshot.Servers["legacy"]
		cfg.Description = "Updated"
		snapshot.Servers["legacy"] = cfg
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision == first.Revision || updated.Servers["legacy"].Generation == firstGeneration {
		t.Fatalf("v2 metadata did not advance: %#v", updated)
	}

	raw, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted registryFile
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Version != registryVersion || persisted.Revision != updated.Revision ||
		persisted.Generations["legacy"] != updated.Servers["legacy"].Generation {
		t.Fatalf("persisted upgraded registry = %#v", persisted)
	}
}
