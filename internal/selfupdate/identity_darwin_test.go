//go:build darwin

package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/updateidentity"
	"github.com/uvwt/agentdock/internal/updateplatform"
)

func writeNextApp(t *testing.T, root, version string) string {
	t.Helper()
	marker := os.Getenv("AGENTDOCK_DESKTOP_VARIANT")
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "stable")
	stablePath := writeSignedMacOSApp(t, root, version)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", marker)
	id, _ := updateidentity.Resolve("next")
	app := filepath.Join(root, id.AppName())
	if err := os.Rename(stablePath, app); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(app, "Contents", "Library", "LaunchAgents")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTDOCK_MACOS_APP_VARIANT", "next")
	t.Setenv("VERSION", version)
	t.Setenv("MIN_VERSION", "12.0")
	runTestCommand(t, "/bin/zsh", "-c", `source ../../packaging/macos/app-identity.sh; write_app_metadata "$1"`, "fixture", filepath.Join(app, "Contents"))
	for helper, suffix := range map[string]string{"agentdock": "core", "agentdock-arbiter": "arbiter", "cloudflared": "cloudflared", "AgentDockLoginHelper": "login-helper"} {
		runTestCommand(t, "/usr/bin/codesign", "--force", "--sign", "-", "--identifier", id.Label(suffix), filepath.Join(app, "Contents", "Helpers", helper))
	}
	runTestCommand(t, "/usr/bin/codesign", "--force", "--sign", "-", "--identifier", id.BundleID, app)
	return app
}

func TestNextSignedBundleAndIdentityMismatch(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	app := writeNextApp(t, t.TempDir(), "0.8.7")
	ctx := context.Background()
	t.Run("missing caller marker", func(t *testing.T) {
		t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "")
		if err := validateUpdateCaller(filepath.Join(app, "Contents", "Helpers", "agentdock")); err == nil {
			t.Fatal("unmarked Next caller accepted")
		}
	})
	if err := validateMacOSDesktopRuntime(ctx, app, "0.8.7"); err != nil {
		t.Fatal(err)
	}
	id, _ := updateidentity.Resolve("next")
	archive := filepath.Join(t.TempDir(), id.Artifact)
	runTestCommand(t, "/usr/bin/ditto", "-c", "-k", "--keepParent", app, archive)
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	extracted, err := extractDesktopUpdateArchive(ctx, data, t.TempDir(), "0.8.7")
	if err != nil || filepath.Base(extracted) != id.AppName() {
		t.Fatalf("Next extraction: %s %v", extracted, err)
	}
	for _, test := range []struct{ name, path, key, value string }{
		{"missing variant", "Contents/Info.plist", "AgentDockVariant", ""},
		{"stable identifier", "Contents/Info.plist", "CFBundleIdentifier", "com.uvwt.agentdock"},
		{"stable service", "Contents/Library/LaunchAgents/" + id.Label("core") + ".plist", "Label", "com.uvwt.agentdock.core"},
		{"stable service environment", "Contents/Library/LaunchAgents/" + id.Label("tunnel") + ".plist", "EnvironmentVariables.AGENTDOCK_DESKTOP_VARIANT", "stable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(app, test.path)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.WriteFile(path, original, 0644) })
			runTestCommand(t, "/usr/bin/plutil", "-replace", test.key, "-string", test.value, path)
			if err := id.ValidateBundle(ctx, app); err == nil {
				t.Fatal("accepted mismatched bundle")
			}
		})
	}
	if err := updateplatform.ValidateNextSigningContinuity(ctx, app, app); err == nil {
		t.Fatal("Next accepted stable ad-hoc continuity exception")
	}
	runTestCommand(t, "/usr/bin/codesign", "--force", "--sign", "-", "--identifier", "com.uvwt.agentdock.arbiter", filepath.Join(app, "Contents", "Helpers", "agentdock-arbiter"))
	runTestCommand(t, "/usr/bin/codesign", "--force", "--sign", "-", "--identifier", id.BundleID, app)
	if err := id.ValidateSignatures(ctx, app); err == nil {
		t.Fatal("accepted stable helper signing identity")
	}
}

func TestNextArchiveRejectsStablePayload(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	f, _ := w.Create("AgentDock.app/Contents/Info.plist")
	_, _ = f.Write([]byte("stable sentinel"))
	_ = w.Close()
	if _, err := extractDesktopUpdateArchive(context.Background(), data.Bytes(), t.TempDir(), "0.8.7"); err == nil {
		t.Fatal("stable archive accepted")
	}
}

func TestNextReleaseRequiresNextArtifactWithoutFallback(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	home := t.TempDir()
	t.Setenv("HOME", home)
	id, _ := updateidentity.Resolve("next")
	for _, artifact := range []string{macOSDesktopArchiveName, id.Artifact} {
		t.Run(artifact, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(release{TagName: "v0.8.8", Assets: []releaseAsset{{Name: artifact, URL: "https://fixture.invalid/app"}, {Name: artifact + ".sha256", URL: "https://fixture.invalid/checksum"}}})
			}))
			defer server.Close()
			result, err := inspectUpdate(context.Background(), options{GOOS: "darwin", DesktopOnly: true, DesktopTargetPath: filepath.Join(home, "Applications", id.AppName()), CurrentVersion: "0.8.7", DesktopCurrentVersion: "0.8.7", ReleaseAPI: server.URL, HTTPClient: server.Client()})
			if artifact == id.Artifact {
				if err != nil || result.DesktopArchiveAsset.Name != id.Artifact {
					t.Fatalf("Next asset: %v", err)
				}
			} else if err == nil {
				t.Fatal("fell back to stable artifact")
			}
		})
	}
}

func TestNextMissingTargetAndUnknownCallerFailClosed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stable := writeSignedMacOSApp(t, filepath.Join(home, "Applications"), "0.8.7")
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv("AGENTDOCK_DESKTOP_APP_PATH", stable)
	if target := detectDesktopUpdateTarget(); target != "" {
		t.Fatalf("selected stable %s", target)
	}
	useStandardDesktopCandidates(t, stable)
	if target := detectStandardDesktopUpdateTarget(); target != "" {
		t.Fatalf("selected stable %s", target)
	}
	if _, _, err := applyManagedDesktopOnlyUpdate(context.Background(), applyRequest{DesktopOnly: true, DesktopTargetPath: stable}); err == nil {
		t.Fatal("stable mutation accepted")
	}
	if _, err := prepareDesktopUpdate(context.Background(), stable, stable, "0.8.7"); err == nil {
		t.Fatal("Next legacy new/backup path enabled")
	}
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "unknown")
	if _, err := macOSDesktopUpdateDirectory(); err == nil {
		t.Fatal("unknown identity created state")
	}
	if err := validateUpdateCaller(filepath.Join(stable, "Contents", "Helpers", "agentdock")); err == nil {
		t.Fatal("unknown caller accepted")
	}
}

func TestNextStateAndHelperEnvironmentAreIsolated(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_HOME", filepath.Join(home, ".agentdock"))
	root, err := macOSDesktopUpdateDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if root != filepath.Join(home, "Library", "Application Support", "AgentDock Next") {
		t.Fatal(root)
	}
	env, err := updateHelperEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(env, "\n"), "AGENTDOCK_HOME="+filepath.Join(home, ".agentdock-next")+"\n") {
		t.Fatal("helper home not scoped")
	}
	path := filepath.Join(root, "update-services.json")
	for _, marker := range []string{"", "stable", "next"} {
		data, _ := json.Marshal(macOSUpdateServiceState{SchemaVersion: 1, Variant: marker})
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		_, err := readMacOSUpdateServiceState(path)
		if (err == nil) != (marker == "next") {
			t.Fatalf("marker=%q err=%v", marker, err)
		}
	}
	if _, err := platformBackupPath(filepath.Join(home, ".local", "bin", "agentdock")); err == nil {
		t.Fatal("standalone backup allowed")
	}
	if candidates := healthCandidates(""); len(candidates) != 1 || candidates[0] != "http://127.0.0.1:8767/healthz" {
		t.Fatal(candidates)
	}
}
