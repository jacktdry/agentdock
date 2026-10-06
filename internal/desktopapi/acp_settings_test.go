package desktopapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
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
	if len(got.Capabilities) != 5 || got.Capabilities[0].Name != "settings" || got.Capabilities[1].Name != "probeProfile" ||
		got.Capabilities[2].Name != "checkProfileUpdate" || got.Capabilities[3].Name != "saveSettings" ||
		got.Capabilities[4].Name != "updateProfileAdapter" || !got.Capabilities[4].RequiresConfirmation {
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

func testACPUpdateService(t *testing.T, latestTag string) (*ACPService, string, func()) {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	command := filepath.Join(home, ".agentdock-next", "bin", "antigravity-acp")
	if err := os.MkdirAll(filepath.Dir(command), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(command, []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 1.2.0-agentdock.6; exit 0; fi\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	candidate := []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 1.2.0-agentdock.7; exit 0; fi\nexit 0\n")
	sum := sha256.Sum256(candidate)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			assets := []map[string]any{}
			for _, name := range []string{
				"agy-acp-darwin-arm64", "agy-acp-darwin-x64",
				"agy-acp-linux-arm64", "agy-acp-linux-x64",
				"agy-acp-windows-arm64.exe", "agy-acp-windows-x64.exe",
			} {
				assets = append(assets, map[string]any{
					"name": name, "browser_download_url": server.URL + "/asset", "digest": digest, "size": len(candidate),
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": latestTag, "assets": assets})
		case "/asset":
			_, _ = w.Write(candidate)
		default:
			http.NotFound(w, r)
		}
	}))
	parsed, err := url.Parse(server.URL)
	if err != nil {
		server.Close()
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
		updateClient: server.Client(),
		updateSource: desktopruntime.ACPUpdateSource{
			ReleaseAPI: server.URL + "/latest", AllowedAssetHosts: []string{parsed.Hostname()},
		},
	}
	return service, command, server.Close
}

func TestACPCheckProfileUpdateRequiresTrustedForkRelease(t *testing.T) {
	service, _, cleanup := testACPUpdateService(t, "v1.2.0-agentdock.7")
	defer cleanup()
	got := service.CheckProfileUpdate(context.Background(), "antigravity")
	if got.Error != nil || got.Profile == nil {
		t.Fatalf("check = %#v", got)
	}
	if !got.Profile.CanUpdate || got.Profile.LatestVersion != "1.2.0-agentdock.7" || got.Profile.VersionState != "update_available" {
		t.Fatalf("profile = %#v", got.Profile)
	}
}

func TestACPUpdateProfileAdapterRejectsStaleExpectedVersion(t *testing.T) {
	service, command, cleanup := testACPUpdateService(t, "v1.2.0-agentdock.7")
	defer cleanup()
	got := service.UpdateProfileAdapter(context.Background(), "antigravity", "1.2.0-agentdock.8")
	if got.Error == nil || got.Error.Code != "acp_profile_update_conflict" || got.Completed {
		t.Fatalf("update = %#v", got)
	}
	output, err := exec.Command(command, "--version").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "1.2.0-agentdock.6\n" {
		t.Fatalf("version after stale update = %q", output)
	}
}

func TestACPUpdateProfileAdapterPromotesVerifiedNextOwnedBinary(t *testing.T) {
	service, command, cleanup := testACPUpdateService(t, "v1.2.0-agentdock.7")
	defer cleanup()
	got := service.UpdateProfileAdapter(context.Background(), "antigravity", "1.2.0-agentdock.7")
	if got.Error != nil || !got.Completed || got.RestartRequired || got.RuntimeImpact != "existing_sessions_unchanged" || got.Profile == nil {
		t.Fatalf("update = %#v", got)
	}
	if got.Profile.InstalledVersion != "1.2.0-agentdock.7" || got.Profile.VersionState != "current" || got.Profile.CanUpdate {
		t.Fatalf("profile = %#v", got.Profile)
	}
	output, err := exec.Command(command, "--version").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "1.2.0-agentdock.7\n" {
		t.Fatalf("version after update = %q", output)
	}
}
