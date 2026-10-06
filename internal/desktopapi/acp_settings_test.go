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
	"runtime"
	"strings"
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

func TestACPSettingsSnapshotHidesSensitiveArguments(t *testing.T) {
	service := &ACPService{
		runtimeRoot: "/runtime",
		readConfiguration: func(context.Context, string) (desktopruntime.ACPConfiguration, error) {
			return desktopruntime.ACPConfiguration{
				Revision: "rev-sensitive",
				ACPSettings: desktopruntime.ACPSettings{
					Profiles: []desktopruntime.ACPProfileSettings{{
						ID: "custom-one", Kind: "custom", Command: "/opt/custom-acp",
						Args: []string{"--api-key=PRIVATE_VALUE", "--mode", "safe"}, Enabled: false,
					}},
				},
			}, nil
		},
	}
	got := service.Settings(context.Background())
	if got.Error != nil || len(got.Profiles) != 1 {
		t.Fatalf("settings = %#v", got)
	}
	profile := got.Profiles[0]
	if !profile.ProtectedArgs || len(profile.ConfiguredArgs) != 0 {
		t.Fatalf("protected args were exposed: %#v", profile)
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "PRIVATE_VALUE") {
		t.Fatalf("serialized profile leaked protected argument: %s", raw)
	}
}

func TestSensitiveACPArgumentVariantsAreProtected(t *testing.T) {
	sensitive := [][]string{
		{"--api_key", "PRIVATE"},
		{"--apiKey=PRIVATE"},
		{"API_KEY=PRIVATE"},
		{"--github-token", "PRIVATE"},
		{"Authorization: Basic PRIVATE"},
		{"--header", "Authorization: Bearer PRIVATE"},
		{"--client_secret=PRIVATE"},
	}
	for _, args := range sensitive {
		if !containsSensitiveACPArguments(args) {
			t.Errorf("args %#v were not protected", args)
		}
	}
	for _, args := range [][]string{
		{"--max-tokens", "4096"},
		{"--tokenizer", "cl100k"},
		{"--sort-key", "name"},
		{"--mode", "safe"},
	} {
		if containsSensitiveACPArguments(args) {
			t.Errorf("args %#v were falsely protected", args)
		}
	}
}

func TestACPProbeProfileDoesNotExposeProtectedDetectedArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("custom executable fixture is Unix-only")
	}
	root := t.TempDir()
	command := filepath.Join(root, "custom-acp")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	service := &ACPService{
		runtimeRoot: root,
		readConfiguration: func(context.Context, string) (desktopruntime.ACPConfiguration, error) {
			return desktopruntime.ACPConfiguration{
				Revision: "rev-sensitive-probe",
				ACPSettings: desktopruntime.ACPSettings{
					Profiles: []desktopruntime.ACPProfileSettings{{
						ID: "custom-one", Kind: "custom", Command: command,
						Args: []string{"--api-key=PRIVATE_VALUE", "--mode", "safe"}, Enabled: false,
					}},
				},
			}, nil
		},
	}
	got := service.ProbeProfile(context.Background(), "custom-one")
	if got.Error != nil || got.Profile == nil {
		t.Fatalf("probe = %#v", got)
	}
	if !got.Profile.ProtectedArgs || len(got.Profile.ConfiguredArgs) != 0 || len(got.Profile.DetectedArgs) != 0 {
		t.Fatalf("protected probe args were exposed: %#v", got.Profile)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "PRIVATE_VALUE") {
		t.Fatalf("probe response leaked protected argument: %s", raw)
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
	if !got.Profile.CanUpdate || got.Profile.LatestVersion != "1.2.0-agentdock.7" || got.Profile.VersionState != "update_available" || got.Profile.UpdatePlan == "" {
		t.Fatalf("profile = %#v", got.Profile)
	}
}

func TestACPCheckProfileUpdateDoesNotExposeProtectedArguments(t *testing.T) {
	service, _, cleanup := testACPUpdateService(t, "v1.2.0-agentdock.7")
	defer cleanup()
	baseRead := service.readConfiguration
	service.readConfiguration = func(ctx context.Context, root string) (desktopruntime.ACPConfiguration, error) {
		config, err := baseRead(ctx, root)
		if err != nil {
			return config, err
		}
		config.Profiles[0].Args = []string{"--api-key=PRIVATE_VALUE", "--mode", "safe"}
		return config, nil
	}
	got := service.CheckProfileUpdate(context.Background(), "antigravity")
	if got.Error != nil || got.Profile == nil {
		t.Fatalf("check = %#v", got)
	}
	if !got.Profile.ProtectedArgs || len(got.Profile.ConfiguredArgs) != 0 || len(got.Profile.DetectedArgs) != 0 || got.Profile.UpdatePlan == "" {
		t.Fatalf("protected update metadata exposed: %#v", got.Profile)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "PRIVATE_VALUE") {
		t.Fatalf("update check leaked protected argument: %s", raw)
	}
}

func TestACPUpdateProfileAdapterRejectsStalePlan(t *testing.T) {
	service, command, cleanup := testACPUpdateService(t, "v1.2.0-agentdock.7")
	defer cleanup()
	got := service.UpdateProfileAdapter(context.Background(), "antigravity", "stale-plan")
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

func TestACPUpdateProfileAdapterReconcilesOutcomeUnknown(t *testing.T) {
	service, _, cleanup := testACPUpdateService(t, "v1.2.0-agentdock.7")
	defer cleanup()
	service.applyUpdate = func(context.Context, *http.Client, desktopruntime.ACPUpdateSource, string, desktopruntime.ACPAdapterUpdate) error {
		return desktopruntime.ErrACPUpdateOutcomeUnknown
	}
	checked := service.CheckProfileUpdate(context.Background(), "antigravity")
	if checked.Error != nil || checked.Profile == nil || checked.Profile.UpdatePlan == "" {
		t.Fatalf("check = %#v", checked)
	}
	got := service.UpdateProfileAdapter(context.Background(), "antigravity", checked.Profile.UpdatePlan)
	if got.Error == nil || got.Error.Code != "acp_profile_update_outcome_unknown" || got.Completed || got.Profile == nil {
		t.Fatalf("update = %#v", got)
	}
	if got.RuntimeImpact != "adapter_update_outcome_unknown" || got.Profile.UpdatePlan != "" || got.Profile.CanUpdate ||
		got.Profile.BlockedReason != "update_outcome_unknown" || got.Profile.VersionState != "unavailable" {
		t.Fatalf("reconciled profile = %#v", got.Profile)
	}
}

func TestACPUpdateProfileAdapterPromotesVerifiedNextOwnedBinary(t *testing.T) {
	service, command, cleanup := testACPUpdateService(t, "v1.2.0-agentdock.7")
	defer cleanup()
	checked := service.CheckProfileUpdate(context.Background(), "antigravity")
	if checked.Error != nil || checked.Profile == nil || checked.Profile.UpdatePlan == "" {
		t.Fatalf("check = %#v", checked)
	}
	got := service.UpdateProfileAdapter(context.Background(), "antigravity", checked.Profile.UpdatePlan)
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
