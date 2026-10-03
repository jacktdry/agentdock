//go:build windows

package selfupdate

import (
	"path/filepath"
	"strings"
)

func coreBinaryFallbackForRuntime(string) (string, bool) {
	return "", false
}

func detectDesktopUpdateTargetForRuntime(runtimeRoot, _ string) string {
	root := strings.TrimSpace(runtimeRoot)
	if root == "" {
		return ""
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	return filepath.Clean(absolute)
}
