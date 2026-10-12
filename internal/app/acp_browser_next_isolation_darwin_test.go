//go:build darwin

package app

import (
	"github.com/uvwt/agentdock/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestNextSourceOnlyCodexHomeNeverCopiesGlobalAuthOrConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv(nextCodexMCPRegistrationEnv, "source-only")
	next := filepath.Join(root, ".agentdock-next")
	isolated := filepath.Join(next, "codex-mcp-isolated")
	if err := os.Mkdir(next, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(isolated, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, ".codex")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"auth.json", "config.toml"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte("DO_NOT_COPY"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	original := runCodexMCPRemove
	runCodexMCPRemove = func(_, _, _ string) error { t.Fatal("Next source-only mutated MCP config"); return nil }
	t.Cleanup(func() { runCodexMCPRemove = original })
	profile := config.ACPProfile{ID: "codex", Kind: "codex", Enabled: true}
	env := map[string]string{"HOME": root, "CODEX_HOME": isolated, "CODEX_PATH": "/usr/bin/true", "AGENTDOCK_NEXT_CODEX_MCP_ISOLATION": "source-only"}
	got, err := prepareACPBrowserEnvironment(next, profile, env, false)
	if err != nil {
		t.Fatal(err)
	}
	if got["CODEX_HOME"] != isolated {
		t.Fatalf("isolated HOME unexpectedly rewritten: %q", got["CODEX_HOME"])
	}
	contents, err := os.ReadDir(isolated)
	if err != nil || len(contents) != 0 {
		t.Fatalf("global credentials copied into Next: %d %v", len(contents), err)
	}
	if _, err := os.Stat(filepath.Join(next, "acp", "codex", "codex-home")); !os.IsNotExist(err) {
		t.Fatalf("legacy cloned-home created: %v", err)
	}
	for _, name := range []string{"auth.json", "config.toml"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil || string(data) != "DO_NOT_COPY" {
			t.Fatalf("global source modified: %v", err)
		}
	}
	bad := map[string]string{"HOME": root, "CODEX_HOME": isolated, "CODEX_PATH": "/usr/bin/true", "AGENTDOCK_NEXT_CODEX_MCP_ISOLATION": "source-only", "CODEX_CONFIG": `{"mcp_servers":{"shared":{}}}`}
	if _, err := prepareACPBrowserEnvironment(next, profile, bad, false); err == nil {
		t.Fatal("inherited native MCP config escaped strict isolation")
	}
}

func TestNextSourceOnlyCodexHomeAllowsOnlyOwnedNativeStateOnRestart(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv(nextCodexMCPRegistrationEnv, "source-only")
	state := filepath.Join(root, ".agentdock-next")
	native := filepath.Join(state, "codex-mcp-isolated")
	if err := os.MkdirAll(native, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"HOME": root, "CODEX_HOME": native, "CODEX_PATH": "/usr/bin/true", "AGENTDOCK_NEXT_CODEX_MCP_ISOLATION": "source-only"}
	profile := config.ACPProfile{ID: "codex", Kind: "codex", Enabled: true}
	for _, name := range []string{".tmp", "tmp", "log", "skills"} {
		if err := os.Mkdir(filepath.Join(native, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"installation_id", "state_5.sqlite", "state_5.sqlite-wal", "state_5.sqlite-shm", "queue_1.sqlite", "logs_2.sqlite", "memories_1.sqlite", "goals_1.sqlite"} {
		if err := os.WriteFile(filepath.Join(native, name), []byte("native"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := prepareACPBrowserEnvironment(state, profile, env, false); err != nil {
		t.Fatalf("native state rejected on Next restart: %v", err)
	}
	evil := filepath.Join(native, "config.toml")
	if err := os.WriteFile(evil, []byte("[mcp_servers.spy]\ncommand='evil'"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareACPBrowserEnvironment(state, profile, env, false); err == nil {
		t.Fatal("unapproved native config accepted")
	}
	if err := os.Remove(evil); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("state_5.sqlite", evil); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareACPBrowserEnvironment(state, profile, env, false); err == nil {
		t.Fatal("symlink accepted")
	}
}
