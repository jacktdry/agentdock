//go:build windows

package desktopruntime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestACPWindowsUsesExistingDPAPIAndSettingsWithoutCreatingCredentials(t *testing.T) {
	root := t.TempDir()
	manifest := Manifest{SchemaVersion: SchemaVersion, AgentDockBinary: filepath.Join(root, "agentdock.exe"), CloudflaredBinary: filepath.Join(root, "cloudflared.exe"), Host: "127.0.0.1", Port: 8765, LocalMCPURL: "http://127.0.0.1:8765/mcp", TunnelMode: "quick"}
	if err := Save(filepath.Join(root, "runtime.json"), manifest); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(root, "auth-token.dpapi")
	access, err := ReadLocalCoreAccess(context.Background(), root)
	if err != nil || access.AuthToken != "" {
		t.Fatalf("missing token: %v", err)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatal("read created credential")
	}
	if err := writeProtectedText(tokenPath, "private-token", "agentdock.startup.v1"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(tokenPath)
	access, err = ReadLocalCoreAccess(context.Background(), root)
	if err != nil || access.AuthToken != "private-token" || access.MCPURL != manifest.LocalMCPURL {
		t.Fatalf("access=%+v err=%v", access, err)
	}
	settingsJSON := []byte(`{"port":8765,"acp_enabled":true,"acp_default_profile":"codex","acp_profiles":[{"id":"codex","kind":"codex","enabled":true,"env_from_env":{"SECRET":"PRIVATE_ENV"}}]}`)
	if err := os.WriteFile(filepath.Join(root, "control-panel-settings.json"), settingsJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := ReadACPSettings(context.Background(), root)
	encoded, _ := json.Marshal(settings)
	if err != nil || !settings.Enabled || settings.DefaultProfile != "codex" || len(settings.Profiles) != 1 || strings.Contains(string(encoded), "PRIVATE_ENV") {
		t.Fatalf("settings=%+v err=%v", settings, err)
	}
	after, _ := os.ReadFile(tokenPath)
	if string(before) != string(after) {
		t.Fatal("read rewrote token")
	}
	if err := writeProtectedText(tokenPath, "private-token", "wrong-entropy"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLocalCoreAccess(context.Background(), root); err == nil {
		t.Fatal("incorrect DPAPI entropy accepted")
	}
}
