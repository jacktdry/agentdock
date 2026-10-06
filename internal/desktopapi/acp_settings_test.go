package desktopapi

import (
	"context"
	"os"
	"path/filepath"
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
	if len(got.Capabilities) != 3 || got.Capabilities[0].Name != "settings" || got.Capabilities[1].Name != "probeProfile" || got.Capabilities[2].Name != "saveSettings" {
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

func TestACPProbeProfileReturnsSafeDetectedMetadata(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	command := filepath.Join(home, ".agentdock-next", "bin", "antigravity-acp")
	if err := os.MkdirAll(filepath.Dir(command), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(command, []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 1.2.0-agentdock.6; fi\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	service := &ACPService{
		runtimeRoot: root,
		readConfiguration: func(context.Context, string) (desktopruntime.ACPConfiguration, error) {
			return desktopruntime.ACPConfiguration{
				Revision: "rev-1",
				ACPSettings: desktopruntime.ACPSettings{
					Profiles: []desktopruntime.ACPProfileSettings{{
						ID: "antigravity", DisplayName: "Antigravity", Kind: "custom", Command: command, Enabled: true,
					}},
				},
			}, nil
		},
	}
	got := service.ProbeProfile(context.Background(), "antigravity")
	if got.Error != nil || got.Profile == nil {
		t.Fatalf("probe = %#v", got)
	}
	if got.Profile.Preset != "antigravity" || got.Profile.Availability != "available" || got.Profile.DetectedCommand != command || got.Profile.InstalledVersion != "1.2.0-agentdock.6" {
		t.Fatalf("profile = %#v", got.Profile)
	}
	if got.Profile.CanUpdate {
		t.Fatal("probe must not imply update authority")
	}
}
