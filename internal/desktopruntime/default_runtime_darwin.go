//go:build darwin

package desktopruntime

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultRuntimeRoot 返回 macOS 桌面版固定的当前用户运行目录。
// LaunchAgent plist 位于 App Bundle 内，不能在构建时写入具体用户名，因此由 Core 自己解析。
func DefaultRuntimeRoot() string {
	name, err := macOSRuntimeName()
	if err != nil {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", name)
}

func macOSRuntimeName() (string, error) {
	switch os.Getenv("AGENTDOCK_DESKTOP_VARIANT") {
	case "", "stable":
		return "AgentDock", nil
	case "next":
		return "AgentDock Next", nil
	default:
		return "", fmt.Errorf("unknown desktop variant")
	}
}
