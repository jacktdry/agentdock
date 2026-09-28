package desktopruntime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Candidate order is explicit so discovery can be tested without a Windows installation.
func tailscaleWindowsCandidates(configured, pathResult, programFiles, programFilesX86, localAppData string) []string {
	candidates := []string{strings.TrimSpace(configured), pathResult}
	for _, root := range []string{programFiles, programFilesX86, localAppData} {
		if root != "" {
			candidates = append(candidates, filepath.Join(root, "Tailscale", "tailscale.exe"))
		}
	}
	return candidates
}

func firstRegularFile(candidates []string) (string, error) {
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		if !filepath.IsAbs(candidate) {
			absolute, err := filepath.Abs(candidate)
			if err != nil {
				continue
			}
			candidate = absolute
		}
		key := strings.ToLower(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", errors.New("未找到 tailscale.exe；请安装 Tailscale 或设置 AGENTDOCK_TAILSCALE_BIN")
}
