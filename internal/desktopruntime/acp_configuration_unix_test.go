//go:build darwin || linux

package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uvwt/agentdock/internal/envstore"
)

func writeACPConfigurationFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "agentdock.env")
	profiles := []map[string]any{
		{
			"id":           "codex",
			"display_name": "Codex",
			"kind":         "codex",
			"command":      "/missing/codex-acp",
			"args":         []string{"--existing"},
			"enabled":      false,
			"env_from_env": map[string]string{"OPENAI_API_KEY": "OPENAI_API_KEY"},
			"future_field": "keep-me",
		},
	}
	raw, err := json.Marshal(profiles)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"AGENTDOCK_ACP_ENABLED":         "false",
		"AGENTDOCK_ACP_DEFAULT_PROFILE": "",
		"AGENTDOCK_ACP_PROFILES_JSON":   string(raw),
		"CUSTOM_SETTING":                "preserve-me",
	}
	if err := os.WriteFile(path, envstore.Marshal(values), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func TestACPConfigurationReadsProfilesWhileDisabledAndPreservesHiddenFields(t *testing.T) {
	root, path := writeACPConfigurationFixture(t)
	ctx := context.Background()

	before, err := ReadACPConfiguration(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if before.Enabled || before.DefaultProfile != "" || len(before.Profiles) != 1 {
		t.Fatalf("before = %#v", before)
	}
	if before.Profiles[0].ID != "codex" || before.Profiles[0].Enabled {
		t.Fatalf("profile = %#v", before.Profiles[0])
	}
	if before.Revision == "" {
		t.Fatal("empty revision")
	}

	requested := before.ACPSettings
	requested.Profiles[0].DisplayName = "Codex Updated"
	requested.Profiles[0].Args = []string{"--updated"}
	after, err := SaveACPSettings(ctx, root, before.Revision, requested)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision == "" || after.Revision == before.Revision {
		t.Fatalf("revision did not advance: before=%q after=%q", before.Revision, after.Revision)
	}
	if len(after.Profiles) != 1 || after.Profiles[0].DisplayName != "Codex Updated" {
		t.Fatalf("after = %#v", after)
	}

	values, err := envstore.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["CUSTOM_SETTING"] != "preserve-me" {
		t.Fatalf("unrelated setting changed: %q", values["CUSTOM_SETTING"])
	}
	var persisted []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(values["AGENTDOCK_ACP_PROFILES_JSON"]), &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 1 {
		t.Fatalf("persisted profiles = %#v", persisted)
	}
	var envFromEnv map[string]string
	if err := json.Unmarshal(persisted[0]["env_from_env"], &envFromEnv); err != nil {
		t.Fatal(err)
	}
	if envFromEnv["OPENAI_API_KEY"] != "OPENAI_API_KEY" {
		t.Fatalf("env_from_env lost: %#v", envFromEnv)
	}
	var future string
	if err := json.Unmarshal(persisted[0]["future_field"], &future); err != nil {
		t.Fatal(err)
	}
	if future != "keep-me" {
		t.Fatalf("future field lost: %q", future)
	}
}

func TestACPConfigurationRejectsStaleRevisionWithoutWriting(t *testing.T) {
	root, path := writeACPConfigurationFixture(t)
	ctx := context.Background()
	initial, err := ReadACPConfiguration(ctx, root)
	if err != nil {
		t.Fatal(err)
	}

	first := initial.ACPSettings
	first.Profiles[0].DisplayName = "First writer"
	saved, err := SaveACPSettings(ctx, root, initial.Revision, first)
	if err != nil {
		t.Fatal(err)
	}
	beforeStale, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	stale := initial.ACPSettings
	stale.Profiles[0].DisplayName = "Stale writer"
	if _, err := SaveACPSettings(ctx, root, initial.Revision, stale); !errors.Is(err, ErrACPSettingsConflict) {
		t.Fatalf("stale save error = %v", err)
	}
	afterStale, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterStale) != string(beforeStale) {
		t.Fatal("stale save modified configuration")
	}
	if saved.Revision == initial.Revision {
		t.Fatal("first save did not change revision")
	}
}

func TestACPConfigurationRejectsInvalidInventory(t *testing.T) {
	root, _ := writeACPConfigurationFixture(t)
	ctx := context.Background()
	if _, err := ReadACPConfiguration(ctx, root); err != nil {
		t.Fatal(err)
	}
	adapter := filepath.Join(root, "custom-adapter")
	if err := os.WriteFile(adapter, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		enabled  bool
		def      string
		profiles []ACPProfileSettings
	}{
		{
			name:    "duplicate ids",
			enabled: false,
			profiles: []ACPProfileSettings{
				{ID: "dup", Kind: "custom", Command: adapter},
				{ID: "dup", Kind: "custom", Command: adapter},
			},
		},
		{
			name:    "reserved custom id",
			enabled: false,
			profiles: []ACPProfileSettings{
				{ID: "codex", Kind: "custom", Command: adapter},
			},
		},
		{
			name:     "enabled without enabled profile",
			enabled:  true,
			profiles: []ACPProfileSettings{{ID: "custom-one", Kind: "custom", Command: adapter, Enabled: false}},
		},
		{
			name:    "default points to disabled profile",
			enabled: true,
			def:     "disabled",
			profiles: []ACPProfileSettings{
				{ID: "enabled", Kind: "custom", Command: adapter, Enabled: true},
				{ID: "disabled", Kind: "custom", Command: adapter, Enabled: false},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			latest, err := ReadACPConfiguration(ctx, root)
			if err != nil {
				t.Fatal(err)
			}
			_, err = SaveACPSettings(ctx, root, latest.Revision, ACPSettings{Enabled: tc.enabled, DefaultProfile: tc.def, Profiles: tc.profiles})
			if !errors.Is(err, ErrACPSettingsInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
