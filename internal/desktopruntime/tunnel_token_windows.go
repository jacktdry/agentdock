//go:build windows

package desktopruntime

import (
	"errors"
	"os"
	"strings"
)

func tunnelTokenStateWindows(runtime tunnelRuntime) string {
	token, err := readProtectedText(runtime.files.token, tunnelTokenEntropy)
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	if err != nil || strings.TrimSpace(token) == "" {
		return "unreadable"
	}
	return "stored"
}

// InvalidateQuickTunnelLocked requires the outer Desktop mutation lock.
func InvalidateQuickTunnelLocked(root string) error {
	runtime, err := loadTunnelRuntime(root)
	if err != nil {
		return err
	}
	if _, err := AdvanceTunnelGenerationLocked(root); err != nil {
		return err
	}
	if err := clearActivePublicURL(runtime.files); err != nil {
		return err
	}
	return runtime.updateManifest("quick", "")
}
