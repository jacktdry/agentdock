//go:build darwin

package selfupdate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

func useStandardDesktopCandidates(t *testing.T, candidates ...string) {
	t.Helper()
	previous := standardDesktopUpdateCandidates
	standardDesktopUpdateCandidates = func() []string { return candidates }
	t.Cleanup(func() { standardDesktopUpdateCandidates = previous })
}

func nativeFallbackRuntimeRoot(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return desktopruntime.DefaultRuntimeRoot()
}

func TestCoreBinaryFallbackForNativeMacOSAppWithoutManifest(t *testing.T) {
	runtimeRoot := nativeFallbackRuntimeRoot(t)
	appPath := writeSignedMacOSApp(t, t.TempDir(), "0.8.7")
	useStandardDesktopCandidates(t, appPath)

	got, ok := coreBinaryFallbackForRuntime(runtimeRoot)
	if !ok {
		t.Fatal("native App fallback was not resolved")
	}
	want := filepath.Join(appPath, "Contents", "Helpers", "agentdock")
	if got != want {
		t.Fatalf("fallback core = %q, want %q", got, want)
	}
	if _, ok := coreBinaryFallbackForRuntime(t.TempDir()); ok {
		t.Fatal("fallback accepted a non-native runtime root")
	}
}

func TestCoreBinaryFallbackDoesNotOverrideExistingManifestFailure(t *testing.T) {
	runtimeRoot := nativeFallbackRuntimeRoot(t)
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, "desktop-runtime.json"), []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	appPath := writeSignedMacOSApp(t, t.TempDir(), "0.8.7")
	useStandardDesktopCandidates(t, appPath)
	if _, ok := coreBinaryFallbackForRuntime(runtimeRoot); ok {
		t.Fatal("fallback overrode an existing runtime manifest")
	}
}

func TestCoreBinaryFallbackIgnoresArbitraryOverride(t *testing.T) {
	runtimeRoot := nativeFallbackRuntimeRoot(t)
	override := writeSignedMacOSApp(t, t.TempDir(), "0.8.7")
	t.Setenv("AGENTDOCK_DESKTOP_APP_PATH", override)
	useStandardDesktopCandidates(t)
	if _, ok := coreBinaryFallbackForRuntime(runtimeRoot); ok {
		t.Fatal("fallback trusted AGENTDOCK_DESKTOP_APP_PATH")
	}
}

func TestCoreBinaryFallbackRejectsSubstitutedHelper(t *testing.T) {
	runtimeRoot := nativeFallbackRuntimeRoot(t)
	appPath := writeSignedMacOSApp(t, t.TempDir(), "0.8.7")
	core := filepath.Join(appPath, "Contents", "Helpers", "agentdock")
	if err := os.WriteFile(core, []byte("#!/bin/sh\necho AgentDock v9.9.9\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	useStandardDesktopCandidates(t, appPath)
	if _, ok := coreBinaryFallbackForRuntime(runtimeRoot); ok {
		t.Fatal("fallback accepted a substituted Core helper")
	}
}
