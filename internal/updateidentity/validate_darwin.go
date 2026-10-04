//go:build darwin

package updateidentity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// SafePath rejects links (including intermediate links) before mutation. The
// standard macOS /var and /tmp aliases are allowed; no user-created link is.
func SafePath(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("non-canonical path %q", path)
	}
	for p := path; p != "/"; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if p == "/var" || p == "/tmp" {
				resolved, err := filepath.EvalSymlinks(p)
				if err == nil && resolved == "/private"+p {
					continue
				}
			}
			return fmt.Errorf("update path contains symlink: %s", p)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if ok && p == "/Applications" && stat.Uid == 0 && stat.Gid == 80 && info.IsDir() && info.Mode().Perm()&0o002 == 0 {
			continue
		}
		if ok && info.Mode().IsRegular() && stat.Nlink != 1 {
			return fmt.Errorf("update file has ambiguous hardlink ownership: %s", p)
		}
		if !ok || (stat.Uid != uint32(os.Getuid()) && stat.Uid != 0) || (info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0) {
			return fmt.Errorf("update path ownership is unsafe: %s", p)
		}
	}
	return nil
}

func Plist(ctx context.Context, path, key string) (string, error) {
	if err := SafePath(path); err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("invalid plist %s", path)
	}
	data, err := exec.CommandContext(ctx, "/usr/bin/plutil", "-extract", key, "raw", "-o", "-", path).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("missing plist key %s", key)
	}
	return strings.TrimSpace(string(data)), nil
}

func (id Identity) ValidateBundle(ctx context.Context, app string) error {
	if err := SafePath(app); err != nil {
		return err
	}
	plist := filepath.Join(app, "Contents", "Info.plist")
	variant, variantErr := Plist(ctx, plist, "AgentDockVariant")
	if variantErr != nil && id.Variant == "next" {
		return variantErr
	}
	bundle, err := Plist(ctx, plist, "CFBundleIdentifier")
	if err != nil {
		return err
	}
	name, _ := Plist(ctx, plist, "CFBundleName")
	display, _ := Plist(ctx, plist, "CFBundleDisplayName")
	if err := id.ValidateMetadata(variant, bundle, name, display); err != nil {
		return err
	}
	if id.Variant != "next" {
		return nil
	}
	executable, err := Plist(ctx, plist, "CFBundleExecutable")
	if err != nil || executable != "AgentDock" {
		return fmt.Errorf("Next executable identity mismatch")
	}
	if err := SafePath(filepath.Join(app, "Contents", "MacOS", "AgentDock")); err != nil {
		return err
	}
	agentDir := filepath.Join(app, "Contents", "Library", "LaunchAgents")
	if err := SafePath(agentDir); err != nil {
		return err
	}
	entries, err := os.ReadDir(agentDir)
	if err != nil {
		return err
	}
	if len(entries) != 3 {
		return fmt.Errorf("unexpected Next service inventory")
	}
	for _, item := range []struct {
		service, program string
		args             []string
	}{
		{"core", "agentdock", []string{"agentdock", "service", "launch-core"}},
		{"tunnel", "agentdock", []string{"agentdock", "tunnel", "launch"}},
		{"menu-login", "AgentDockLoginHelper", []string{"AgentDockLoginHelper"}},
	} {
		path := filepath.Join(app, "Contents", "Library", "LaunchAgents", id.Label(item.service)+".plist")
		if err := SafePath(path); err != nil {
			return err
		}
		raw, err := exec.CommandContext(ctx, "/usr/bin/plutil", "-convert", "json", "-o", "-", path).Output()
		var service map[string]json.RawMessage
		if err != nil || json.Unmarshal(raw, &service) != nil {
			return fmt.Errorf("invalid Next service plist")
		}
		var environment map[string]string
		if env, ok := service["EnvironmentVariables"]; ok {
			if json.Unmarshal(env, &environment) != nil {
				return fmt.Errorf("invalid Next service environment")
			}
		}
		if item.service == "menu-login" {
			if len(environment) != 0 {
				return fmt.Errorf("unexpected login service environment")
			}
		} else if len(environment) != 1 || environment["AGENTDOCK_DESKTOP_VARIANT"] != "next" {
			return fmt.Errorf("unexpected Next service environment")
		}

		expected := map[string]string{"Label": id.Label(item.service), "BundleProgram": "Contents/Helpers/" + item.program}
		for i, arg := range item.args {
			expected[fmt.Sprintf("ProgramArguments.%d", i)] = arg
		}
		if item.service != "menu-login" {
			expected["EnvironmentVariables.AGENTDOCK_DESKTOP_VARIANT"] = "next"
		}
		for key, want := range expected {
			value, err := Plist(ctx, path, key)
			if err != nil || value != want {
				return fmt.Errorf("Next service %s has invalid %s", item.service, key)
			}
		}
		if _, err := Plist(ctx, path, fmt.Sprintf("ProgramArguments.%d", len(item.args))); err == nil {
			return fmt.Errorf("unexpected Next service arguments")
		}
		// Runtime and state come from the signed identity contract, never plist overrides.
		for _, key := range []string{"Program", "WorkingDirectory", "EnvironmentVariables.AGENTDOCK_HOME", "EnvironmentVariables.AGENTDOCK_PORT", "EnvironmentVariables.AGENTDOCK_RUNTIME_ROOT"} {
			if _, err := Plist(ctx, path, key); err == nil {
				return fmt.Errorf("unexpected Next service override %s", key)
			}
		}
	}
	return nil
}

// ValidateSignatures checks every helper's signing identifier, not just the
// outer seal. No helper is executed until this check succeeds.
func (id Identity) ValidateSignatures(ctx context.Context, app string) error {
	for _, item := range []struct{ path, identifier string }{
		{app, id.BundleID},
		{filepath.Join(app, "Contents", "Helpers", "agentdock"), id.Label("core")},
		{filepath.Join(app, "Contents", "Helpers", "agentdock-arbiter"), id.Label("arbiter")},
		{filepath.Join(app, "Contents", "Helpers", "cloudflared"), id.Label("cloudflared")},
		{filepath.Join(app, "Contents", "Helpers", "AgentDockLoginHelper"), id.Label("login-helper")},
	} {
		if err := SafePath(item.path); err != nil {
			return err
		}
		if output, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", item.path).CombinedOutput(); err != nil {
			return fmt.Errorf("invalid signature: %s: %w", output, err)
		}
		output, err := exec.CommandContext(ctx, "/usr/bin/codesign", "-dv", "--verbose=4", item.path).CombinedOutput()
		if err != nil || !strings.Contains("\n"+string(output), "\nIdentifier="+item.identifier+"\n") {
			return fmt.Errorf("signing identity mismatch: %s", item.path)
		}
	}
	return nil
}
