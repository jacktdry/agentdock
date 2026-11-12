//go:build darwin

package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/config"
)

// nextCodexCLIPrerequisite is strictly for tests of Core policy, lifecycle, and
// negative admission. It satisfies the legacy Codex MCP removal prerequisite
// without using an installed Codex CLI or claiming native Codex/ACP coverage.
// The actual ACP adapter (if exercised) remains the separate test-owned binary
// configured on the profile, never this CLI-only fixture.
func nextCodexCLIPrerequisite(t *testing.T, profile *config.ACPProfile, stateHome string) {
	t.Helper()
	if profile.ID != "codex" || profile.Kind != "codex" {
		t.Fatal("Codex CLI prerequisite can only be used for a Codex test profile")
	}
	if !filepath.IsAbs(stateHome) {
		t.Fatal("test state home must be absolute")
	}
	if profile.EnvFromEnv == nil {
		profile.EnvFromEnv = make(map[string]string)
	}
	if _, exists := profile.EnvFromEnv["CODEX_PATH"]; exists {
		t.Fatal("test prerequisite may not replace an existing CODEX_PATH binding")
	}

	// This is NOT a Codex/ACP implementation: accept exactly the three
	// browser-MCP removal arguments during isolated NewRuntime setup.
	dir := t.TempDir()
	cli := filepath.Join(dir, "codex-prerequisite-only")
	log := filepath.Join(dir, "accepted-mcp-removals")
	const script = "#!/bin/sh\n" +
		"test \"$#\" -eq 3 && test \"$1\" = mcp && test \"$2\" = remove || exit 81\n" +
		"case \"$3\" in chrome-devtools|node_repl|computer-use) ;; *) exit 82 ;; esac\n" +
		"test \"$CODEX_HOME\" = \"$NEXT_TEST_CODEX_HOME\" && test -d \"$CODEX_HOME\" || exit 83\n" +
		"printf '%s\\n' \"$3\" >> \"$NEXT_TEST_CODEX_LOG\"\n"
	if err := os.WriteFile(cli, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEXT_TEST_CODEX_CLI_PATH", cli)
	t.Setenv("NEXT_TEST_CODEX_HOME", filepath.Join(stateHome, "acp", "codex", "codex-home"))
	t.Setenv("NEXT_TEST_CODEX_LOG", log)
	profile.EnvFromEnv["CODEX_PATH"] = "NEXT_TEST_CODEX_CLI_PATH"
	t.Cleanup(func() {
		data, err := os.ReadFile(log)
		if err != nil {
			t.Errorf("test-owned Codex prerequisite was not invoked: %v", err)
			return
		}
		observed := strings.Fields(string(data))
		for _, server := range codexACPBlockedMCPServers {
			if !slices.Contains(observed, server) {
				t.Errorf("test-owned Codex prerequisite did not remove %s: %v", server, observed)
			}
		}
	})
}

// Explicit CODEX_PATH must fail closed rather than silently using an
// installed Codex from macOS's supplemental /opt/homebrew/bin lookup.
func TestNextCodexCLIPrerequisiteExplicitMissingFailsClosed(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("PATH", t.TempDir())
	profile := config.ACPProfile{ID: "codex", Kind: "codex", Enabled: true}
	if _, err := prepareACPBrowserEnvironment(filepath.Join(root, ".agentdock-test"), profile,
		map[string]string{"CODEX_PATH": filepath.Join(root, "missing-codex")}, false); err == nil {
		t.Fatal("missing explicit Codex CLI was accepted or fell back to the host installation")
	}
}
