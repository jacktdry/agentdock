//go:build darwin || linux

package desktopruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CoreBinaryForRuntime resolves the managed core from the selected runtime root
// without consulting the calling desktop shell's executable identity.
func CoreBinaryForRuntime(runtimeRoot string) (string, error) {
	root, err := filepath.Abs(strings.TrimSpace(runtimeRoot))
	if err != nil || root == "" {
		return "", errors.New("runtime-root 无效")
	}
	data, err := os.ReadFile(filepath.Join(root, "desktop-runtime.json"))
	if err != nil {
		return "", fmt.Errorf("读取桌面运行清单失败: %w", err)
	}
	var manifest unixRuntimeManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("解析桌面运行清单失败: %w", err)
	}
	binary := strings.TrimSpace(manifest.AgentDockBinary)
	if binary == "" || !filepath.IsAbs(binary) {
		return "", errors.New("桌面运行清单缺少有效核心路径")
	}
	info, err := os.Stat(binary)
	if err != nil {
		return "", fmt.Errorf("读取核心二进制失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("核心路径不是普通文件")
	}
	return filepath.Clean(binary), nil
}
