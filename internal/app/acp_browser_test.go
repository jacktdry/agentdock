package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/uvwt/agentdock/internal/config"
)

func TestPrepareACPBrowserEnvironmentIsolatesCodexBrowserBackends(t *testing.T) {
	source := t.TempDir()
	t.Setenv("CODEX_HOME", source)
	configText := `[mcp_servers.chrome-devtools]
command = "npx"
[mcp_servers.node_repl]
command = "node"
[mcp_servers.computer-use]
command = "cua"
[mcp_servers.gitlab]
url = "https://gitlab.example/mcp"
`
	if err := os.WriteFile(filepath.Join(source, "config.toml"), []byte(configText), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "auth.json"), []byte(`{"token":"test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "AGENTS.md"), []byte("rules"), 0o600); err != nil {
		t.Fatal(err)
	}

	previous := runCodexMCPRemove
	var removed []string
	codexPath := filepath.Join(t.TempDir(), codexFixtureName())
	if err := os.WriteFile(codexPath, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	runCodexMCPRemove = func(path string, home, server string) error {
		if path != codexPath {
			t.Fatalf("wrong resolved executable: %q", path)
		}
		if home == "" {
			t.Fatal("sandbox home missing")
		}
		removed = append(removed, server)
		return nil
	}
	defer func() { runCodexMCPRemove = previous }()

	env := map[string]string{
		"CODEX_PATH":   codexPath,
		"CODEX_CONFIG": `{"model":"gpt-test","mcp_servers":{"gitlab":{"url":"https://gitlab.example/mcp"},"chrome-devtools":{"command":"npx"},"node_repl":{"command":"node"},"computer-use":{"command":"cua"}}}`,
	}
	got, err := prepareACPBrowserEnvironment(t.TempDir(), config.ACPProfile{ID: "codex", Kind: "codex"}, env, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(removed, codexACPBlockedMCPServers) {
		t.Fatalf("removed=%v", removed)
	}
	if got["CODEX_HOME"] == "" || got["CODEX_HOME"] == source {
		t.Fatalf("isolated CODEX_HOME=%q", got["CODEX_HOME"])
	}
	for _, name := range []string{"config.toml", "auth.json", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(got["CODEX_HOME"], name)); err != nil {
			t.Fatalf("sandbox %s: %v", name, err)
		}
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(got["CODEX_CONFIG"]), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["model"] != "gpt-test" {
		t.Fatalf("model lost: %#v", cfg)
	}
	servers := cfg["mcp_servers"].(map[string]any)
	if _, ok := servers["gitlab"]; !ok {
		t.Fatalf("non-browser MCP lost: %#v", servers)
	}
	for _, blocked := range codexACPBlockedMCPServers {
		if _, ok := servers[blocked]; ok {
			t.Fatalf("blocked MCP %s survived: %#v", blocked, servers)
		}
	}
	features := cfg["features"].(map[string]any)
	for _, key := range []string{"in_app_browser", "browser_use_full_cdp_access", "browser_use_external", "in_app_local_automation"} {
		if features[key] != false {
			t.Fatalf("feature %s not disabled: %#v", key, features)
		}
	}
	plugins := cfg["plugins"].(map[string]any)
	for _, key := range []string{"chrome@openai-bundled", "computer-use@openai-bundled", "unified-computer-use@openai-bundled"} {
		if plugins[key].(map[string]any)["enabled"] != false {
			t.Fatalf("plugin %s not disabled: %#v", key, plugins[key])
		}
	}
	if acpBrowserServerName(config.ACPProfile{ID: "codex", Kind: "codex"}) != "agentdock-browser" {
		t.Fatal("Codex Broker MCP should use dedicated server name")
	}
}

func TestPrepareACPBrowserEnvironmentMarksAntigravityBrokerRequired(t *testing.T) {
	got, err := prepareACPBrowserEnvironment(t.TempDir(), config.ACPProfile{ID: "agy", Kind: "custom", Command: "/Users/test/.local/bin/antigravity-acp"}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got["AGENTDOCK_BROWSER_BROKER_REQUIRED"] != "1" {
		t.Fatalf("env=%#v", got)
	}
}

func TestPrepareACPBrowserEnvironmentDisablesAntigravityBrowserBackendsWithoutBroker(t *testing.T) {
	got, err := prepareACPBrowserEnvironment(t.TempDir(), config.ACPProfile{ID: "agy", Kind: "custom", Command: "/Users/test/.local/bin/antigravity-acp"}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got["AGENTDOCK_BROWSER_BACKENDS_DISABLED"] != "1" {
		t.Fatalf("env=%#v", got)
	}
	if _, ok := got["AGENTDOCK_BROWSER_BROKER_REQUIRED"]; ok {
		t.Fatalf("broker should not be required without an available bridge: %#v", got)
	}
}

func TestResolveCodexCLIExplicitFailsClosed(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, codexFixtureName())
	if err := os.WriteFile(valid, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	nonexec := filepath.Join(dir, "nonexec")
	if err := os.WriteFile(nonexec, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	invalid := []string{"", "codex", filepath.Join(dir, "missing-secret"), dir}
	if runtime.GOOS != "windows" {
		invalid = append(invalid, nonexec)
	}
	for _, path := range invalid {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := resolveCodexCLI(map[string]string{"CODEX_PATH": path}); err == nil {
				t.Fatal("invalid explicit path fell back to available CLI")
			}
		})
	}
	got, err := resolveCodexCLI(map[string]string{"CODEX_PATH": valid})
	if err != nil || got != valid {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func codexFixtureName() string {
	if runtime.GOOS == "windows" {
		return "codex.exe"
	}
	return "codex"
}
