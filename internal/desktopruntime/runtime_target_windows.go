//go:build windows

package desktopruntime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// CoreBinaryForRuntime resolves the active managed core from the selected
// Windows runtime root rather than from the calling desktop executable.
func CoreBinaryForRuntime(runtimeRoot string) (string, error) {
	manifest, root, err := loadDesktopManifest(runtimeRoot)
	if err != nil {
		return "", err
	}
	binary := strings.TrimSpace(ActiveCoreBinary(root, manifest))
	if binary == "" || !filepath.IsAbs(binary) {
		return "", errors.New("Windows runtime manifest 缺少有效核心路径")
	}
	info, err := os.Stat(binary)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("Windows 核心路径不是普通文件")
	}
	return filepath.Clean(binary), nil
}
