package desktopapi

import (
	"context"
	"testing"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

func TestACPSettingsSnapshotIncludesDisabledProfilesAndPresetMetadata(t *testing.T) {
	service := &ACPService{
		runtimeRoot: "/runtime",
		readConfiguration: func(context.Context, string) (desktopruntime.ACPConfiguration, error) {
			return desktopruntime.ACPConfiguration{
				Revision: "rev-1",
				ACPSettings: desktopruntime.ACPSettings{
					Enabled:        false,
					DefaultProfile: "",
					Profiles: []desktopruntime.ACPProfileSettings{
						{ID: "codex", DisplayName: "Codex", Kind: "codex", Command: "/opt/codex-acp", Enabled: false},
						{ID: "agy", DisplayName: "Antigravity", Kind: "custom", Command: "/next/bin/antigravity-acp", Enabled: false},
						{ID: "claude", DisplayName: "Legacy Claude", Kind: "claude", Command: "/opt/claude-agent-acp", Enabled: false},
					},
				},
			}, nil
		},
	}
	got := service.Settings(context.Background())
	if got.Error != nil || got.Enabled || got.Revision != "rev-1" || len(got.Profiles) != 3 {
		t.Fatalf("settings = %#v", got)
	}
	if got.Profiles[0].Preset != "codex" || got.Profiles[0].Source != "builtin" {
		t.Fatalf("codex = %#v", got.Profiles[0])
	}
	if got.Profiles[1].Preset != "antigravity" || got.Profiles[1].Source != "custom-fork" {
		t.Fatalf("antigravity = %#v", got.Profiles[1])
	}
	if got.Profiles[2].Preset != "legacy" || got.Profiles[2].Source != "builtin" {
		t.Fatalf("legacy = %#v", got.Profiles[2])
	}
	if len(got.Capabilities) != 2 || got.Capabilities[0].Name != "settings" || got.Capabilities[1].Name != "saveSettings" {
		t.Fatalf("capabilities = %#v", got.Capabilities)
	}
}

func TestACPSaveSettingsMapsConflictAndReportsPersistenceOnly(t *testing.T) {
	service := &ACPService{
		runtimeRoot: "/runtime",
		saveConfiguration: func(_ context.Context, _ string, revision string, settings desktopruntime.ACPSettings) (desktopruntime.ACPConfiguration, error) {
			if revision == "stale" {
				return desktopruntime.ACPConfiguration{}, desktopruntime.ErrACPSettingsConflict
			}
			return desktopruntime.ACPConfiguration{Revision: "rev-2", ACPSettings: settings}, nil
		},
	}
	conflict := service.SaveSettings(context.Background(), "stale", false, "", nil)
	if conflict.Error == nil || conflict.Error.Code != "acp_settings_conflict" || conflict.Completed || conflict.Persisted {
		t.Fatalf("conflict = %#v", conflict)
	}

	saved := service.SaveSettings(context.Background(), "rev-1", false, "", []desktopruntime.ACPProfileSettings{})
	if saved.Error != nil || !saved.Completed || !saved.Persisted || saved.Applied || !saved.RestartRequired {
		t.Fatalf("saved = %#v", saved)
	}
	if saved.RuntimeImpact != "existing_runtime_unchanged" || saved.Snapshot == nil || saved.Snapshot.Revision != "rev-2" {
		t.Fatalf("saved details = %#v", saved)
	}
}
