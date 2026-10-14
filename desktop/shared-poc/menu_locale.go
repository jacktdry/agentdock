package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type nativeMenuLabels struct {
	showWindow    string
	open          string
	quit          string
	pluginFolder  string
	pluginZip     string
	choose        string
	zipArchives   string
	trayInitial   string
	trayChecking  string
	trayExpired   string
	trayRefresh   string
	trayCoreName  string
	trayHealthy   string
	trayUnhealthy string
	trayStopped   string
	trayUnknown   string
	trayObserved  string
}

// Native menu uses the OS language, independently of the webview preference.
func nativeMenuLanguage() string {
	if runtime.GOOS == "darwin" {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		// Bounded, read-only preferences access; no window or browser launch.
		if output, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
			if language := firstAppleLanguage(string(output)); language != "" {
				return normalizeNativeMenuLanguage(language)
			}
		}
	}
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
		if value := os.Getenv(env); value != "" {
			return normalizeNativeMenuLanguage(strings.Split(value, ":")[0])
		}
	}
	return "en"
}

func firstAppleLanguage(output string) string {
	index := strings.IndexByte(output, '"')
	if index < 0 {
		return ""
	}
	rest := output[index+1:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func normalizeNativeMenuLanguage(raw string) string {
	lang := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), "_", "-"))
	lang = strings.SplitN(lang, ".", 2)[0]
	switch {
	case strings.HasPrefix(lang, "zh-hant"), strings.HasPrefix(lang, "zh-tw"),
		strings.HasPrefix(lang, "zh-hk"), strings.HasPrefix(lang, "zh-mo"):
		return "zh-Hant"
	case strings.HasPrefix(lang, "zh"):
		return "zh-Hans"
	default:
		return "en"
	}
}

func labelsForNativeMenu(language string) nativeMenuLabels {
	switch language {
	case "zh-Hant":
		return nativeMenuLabels{showWindow: "顯示視窗", open: "開啟", quit: "結束介面", pluginFolder: "選取 Plugin 資料夾", pluginZip: "選取 Plugin ZIP 檔案", choose: "選取", zipArchives: "ZIP 壓縮檔", trayInitial: "核心服務：尚未檢查", trayChecking: "核心服務：檢查中…", trayExpired: "核心服務：資料已過期", trayRefresh: "檢查核心服務狀態", trayCoreName: "核心服務", trayHealthy: "運作正常", trayUnhealthy: "運行中／健康異常", trayStopped: "已停止", trayUnknown: "無法確認", trayObserved: "上次觀察"}
	case "zh-Hans":
		return nativeMenuLabels{showWindow: "显示窗口", open: "打开", quit: "退出界面", pluginFolder: "选择 Plugin 文件夹", pluginZip: "选择 Plugin ZIP 文件", choose: "选择", zipArchives: "ZIP 压缩包", trayInitial: "核心服务：尚未检查", trayChecking: "核心服务：检查中…", trayExpired: "核心服务：数据已过期", trayRefresh: "检查核心服务状态", trayCoreName: "核心服务", trayHealthy: "运行正常", trayUnhealthy: "运行中／健康异常", trayStopped: "已停止", trayUnknown: "无法确认", trayObserved: "上次观察"}
	default:
		return nativeMenuLabels{showWindow: "Show Window", open: "Open", quit: "Quit UI", pluginFolder: "Choose Plugin folder", pluginZip: "Choose Plugin ZIP file", choose: "Choose", zipArchives: "ZIP archives", trayInitial: "Core: not checked", trayChecking: "Core: checking…", trayExpired: "Core: observation expired", trayRefresh: "Check Core health", trayCoreName: "Core", trayHealthy: "healthy", trayUnhealthy: "running, unhealthy", trayStopped: "stopped", trayUnknown: "unknown", trayObserved: "observed"}
	}
}

// trayCoreLabel is a last-observed status, never a claim of a live connector.
func (l nativeMenuLabels) trayCoreLabel(reading coreTrayReading, observed time.Time) string {
	state := l.trayUnknown
	if !reading.err {
		switch {
		case !reading.running:
			state = l.trayStopped
		case reading.healthy:
			state = l.trayHealthy
		default:
			state = l.trayUnhealthy
		}
	}
	if l.trayCoreName == "Core" {
		return fmt.Sprintf("%s: %s (%s %s)", l.trayCoreName, state, l.trayObserved, observed.Format("15:04:05"))
	}
	return fmt.Sprintf("%s：%s（%s %s）", l.trayCoreName, state, l.trayObserved, observed.Format("15:04:05"))
}
