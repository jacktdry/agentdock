package main

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type nativeMenuLabels struct {
	showWindow   string
	open         string
	quit         string
	pluginFolder string
	pluginZip    string
	choose       string
	zipArchives  string
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
		return nativeMenuLabels{showWindow: "顯示視窗", open: "開啟", quit: "結束介面", pluginFolder: "選取 Plugin 資料夾", pluginZip: "選取 Plugin ZIP 檔案", choose: "選取", zipArchives: "ZIP 壓縮檔"}
	case "zh-Hans":
		return nativeMenuLabels{showWindow: "显示窗口", open: "打开", quit: "退出界面", pluginFolder: "选择 Plugin 文件夹", pluginZip: "选择 Plugin ZIP 文件", choose: "选择", zipArchives: "ZIP 压缩包"}
	default:
		return nativeMenuLabels{showWindow: "Show Window", open: "Open", quit: "Quit UI", pluginFolder: "Choose Plugin folder", pluginZip: "Choose Plugin ZIP file", choose: "Choose", zipArchives: "ZIP archives"}
	}
}
