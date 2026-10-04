//go:build darwin

package selfupdate

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

func coreBinaryFallbackForRuntime(runtimeRoot string) (string, bool) {
	root, err := filepath.Abs(strings.TrimSpace(runtimeRoot))
	if err != nil {
		return "", false
	}
	expected, err := filepath.Abs(desktopruntime.DefaultRuntimeRoot())
	if err != nil || filepath.Clean(root) != filepath.Clean(expected) {
		return "", false
	}

	// The native-App fallback is only for the legacy/native layout that never
	// wrote desktop-runtime.json. If a manifest exists, any resolution failure
	// must remain visible rather than silently switching update targets.
	_, manifestErr := os.Lstat(filepath.Join(root, "desktop-runtime.json"))
	if manifestErr == nil || !errors.Is(manifestErr, os.ErrNotExist) {
		return "", false
	}

	appPath := detectStandardDesktopUpdateTarget()
	if appPath == "" || validateMacOSFallbackTarget(appPath) != nil {
		return "", false
	}
	return filepath.Join(appPath, "Contents", "Helpers", "agentdock"), true
}

var standardDesktopUpdateCandidates = func() []string {
	id, err := currentMacOSUpdateIdentity()
	if err != nil {
		return nil
	}
	candidates := []string{filepath.Join("/Applications", id.AppName())}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		candidates = append(candidates, filepath.Join(home, "Applications", id.AppName()))
	}
	return candidates
}

func detectStandardDesktopUpdateTarget() string {
	for _, candidate := range uniqueStrings(standardDesktopUpdateCandidates()) {
		candidate = filepath.Clean(candidate)
		if validateMacOSFallbackTarget(candidate) == nil {
			return candidate
		}
	}
	return ""
}

func validateMacOSFallbackTarget(appPath string) error {
	if err := validateMacOSDesktopTarget(appPath); err != nil {
		return err
	}
	core := filepath.Join(appPath, "Contents", "Helpers", "agentdock")
	if !executableRegularFile(core) {
		return errors.New("macOS App 缺少有效 Core helper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", "--verbose=2", appPath).CombinedOutput(); err != nil {
		return errors.New(strings.TrimSpace(string(output)))
	}
	if output, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--strict", "--verbose=2", core).CombinedOutput(); err != nil {
		return errors.New(strings.TrimSpace(string(output)))
	}
	return nil
}

func detectDesktopUpdateTargetForRuntime(_ string, executable string) string {
	path := filepath.Clean(strings.TrimSpace(executable))
	for dir := filepath.Dir(path); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if filepath.Ext(dir) != ".app" {
			continue
		}
		if validateMacOSDesktopTarget(dir) == nil && desktopUpdateOwnsExecutable(dir, path) {
			return dir
		}
		return ""
	}
	return ""
}
