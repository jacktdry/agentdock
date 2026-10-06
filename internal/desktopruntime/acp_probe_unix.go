//go:build darwin || linux

package desktopruntime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func platformResolveACPProbe(runtimeRoot, preset string, profile ACPProfileSettings) (string, []string, bool) {
	if command, ok := unixACPExecutable(profile.Command); ok {
		return command, append([]string(nil), profile.Args...), true
	}

	if preset == "antigravity" && NextManagedRoot(runtimeRoot) {
		home, _ := os.UserHomeDir()
		for _, candidate := range []string{
			filepath.Join(home, ".agentdock-next", "bin", "antigravity-acp"),
			filepath.Join(runtimeRoot, "bin", "antigravity-acp"),
		} {
			if command, ok := unixACPExecutable(candidate); ok {
				return command, append([]string(nil), profile.Args...), true
			}
		}
	}

	if profile.Kind == "custom" {
		return "", nil, false
	}

	names := map[string][]string{
		"codex":  {"codex-acp"},
		"claude": {"claude-agent-acp"},
		"grok":   {"grok"},
	}[strings.ToLower(strings.TrimSpace(profile.Kind))]
	if len(names) == 0 {
		return "", nil, false
	}

	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(runtimeRoot, "bin"),
		filepath.Join(home, ".local", "bin"),
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/bin",
	}
	for _, name := range names {
		if command, err := exec.LookPath(name); err == nil {
			if command, ok := unixACPExecutable(command); ok {
				return command, append([]string(nil), profile.Args...), true
			}
		}
		for _, dir := range dirs {
			if command, ok := unixACPExecutable(filepath.Join(dir, name)); ok {
				return command, append([]string(nil), profile.Args...), true
			}
		}
	}
	return "", nil, false
}

func unixACPExecutable(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", false
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return filepath.Clean(absolute), true
}
