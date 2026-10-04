package desktopruntime

import "testing"

func TestACPSettingsFromEnvironmentProfilesAndLegacy(t *testing.T) {
	got, err := acpSettingsFromEnvironment(map[string]string{"AGENTDOCK_ACP_ENABLED": "true", "AGENTDOCK_ACP_PROFILES_JSON": `[{"id":"codex","kind":"codex","command":"/bin/codex-acp","enabled":true,"env_from_env":{"SECRET":"HOST_SECRET"}}]`, "AGENTDOCK_ACP_DEFAULT_PROFILE": "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.DefaultProfile != "codex" || len(got.Profiles) != 1 || got.Profiles[0].ID != "codex" || got.Profiles[0].Command != "/bin/codex-acp" {
		t.Fatalf("profiles=%+v", got)
	}
	legacy, err := acpSettingsFromEnvironment(map[string]string{"AGENTDOCK_ACP_ENABLED": "true", "AGENTDOCK_ACP_AGENT": "agy", "AGENTDOCK_ACP_COMMAND": "/bin/antigravity-acp", "AGENTDOCK_ACP_ARGS_JSON": `["--x"]`})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.DefaultProfile != "agy" || len(legacy.Profiles) != 1 || legacy.Profiles[0].Kind != "custom" || legacy.Profiles[0].Args[0] != "--x" {
		t.Fatalf("legacy=%+v", legacy)
	}
}
