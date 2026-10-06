//go:build windows

package desktopruntime

import (
	"path/filepath"
	"strings"
)

func platformResolveACPProbe(runtimeRoot, preset string, profile ACPProfileSettings) (string, []string, bool) {
	if preset == "antigravity" && strings.TrimSpace(profile.Command) == "" {
		for _, candidate := range []string{
			filepath.Join(runtimeRoot, "bin", "antigravity-acp.exe"),
			filepath.Join(runtimeRoot, "bin", "antigravity-acp.com"),
		} {
			if adapter, ok := resolveConfiguredACPAdapter(candidate, profile.Args); ok {
				return adapter.Command, adapter.Args, true
			}
		}
		return "", nil, false
	}
	adapter, err := resolveDesktopACPAdapter(profile.Kind, runtimeRoot, profile.Command, profile.Args)
	if err != nil {
		return "", nil, false
	}
	return adapter.Command, adapter.Args, true
}
