//go:build darwin

package commandpath

import (
	"path/filepath"
	"strings"
)

// Path supplements launchd's PATH with common macOS CLI locations.
func Path(currentPath, home string) string {
	return searchPath(currentPath, home, []string{"/opt/homebrew/bin", "/usr/local/bin"})
}

func searchPath(currentPath, home string, common []string) string {
	entries := make([]string, 0, 8)
	if home != "" {
		entries = append(entries, filepath.Join(home, ".local", "bin"))
	}
	entries = append(entries, common...)
	entries = append(entries, filepath.SplitList(currentPath)...)

	seen := make(map[string]struct{}, len(entries))
	normalized := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == "" {
			continue
		}
		if _, exists := seen[entry]; exists {
			continue
		}
		seen[entry] = struct{}{}
		normalized = append(normalized, entry)
	}
	return strings.Join(normalized, ":")
}
