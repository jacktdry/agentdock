//go:build darwin || linux

package desktopruntime

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// runQuickTunnel 是 Quick Tunnel 公网地址回写的唯一权威（平台脚本的 legacy
// 刷新逻辑已删除）。回写必须：把新地址写进 AGENTDOCK_SERVER_URL、开启 OAuth、
// 重启 Core 并落 quick-tunnel-url.txt，同时不能轮换已有的稳定凭据。
func TestQuickTunnelURLPublicationWaitsForDesktopMutation(t *testing.T) {
	root := t.TempDir()
	envFile := filepath.Join(root, "agentdock.env")
	if err := os.WriteFile(envFile, []byte("AGENTDOCK_HOST=127.0.0.1\nAGENTDOCK_PORT=8765\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	serviceManager := "launchd"
	if runtime.GOOS == "darwin" {
		launchctl := filepath.Join(root, "fake-launchctl")
		if err := os.WriteFile(launchctl, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("AGENTDOCK_LAUNCHCTL_BIN", launchctl)
	} else {
		serviceManager = "systemd"
		systemctl := filepath.Join(root, "systemctl")
		if err := os.WriteFile(systemctl, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	manifest := unixRuntimeManifest{
		SchemaVersion:     1,
		ServiceManager:    serviceManager,
		ServiceName:       "com.uvwt.agentdock",
		TunnelServiceName: "com.uvwt.agentdock.cloudflared",
		AgentDockBinary:   filepath.Join(root, "agentdock"),
		EnvironmentFile:   envFile,
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	release, err := AcquireDesktopMutation(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- applyQuickTunnelURLUnix(context.Background(), manifest, root, root, "https://delayed.trycloudflare.com")
	}()

	time.Sleep(80 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(root, "quick-tunnel-url.txt")); !os.IsNotExist(err) {
		t.Fatalf("quick URL published while mutation lock held: %v", err)
	}
	before, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(before), "AGENTDOCK_SERVER_URL") {
		t.Fatal("core environment changed while mutation lock held")
	}

	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("quick URL publication did not resume after lock release")
	}
	after, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "AGENTDOCK_SERVER_URL='https://delayed.trycloudflare.com'") {
		t.Fatalf("core environment missing delayed URL: %s", after)
	}
}

func TestRunQuickTunnelWritesBackPublicURLAndRestartsCore(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"version":"test"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer healthServer.Close()
	healthPort := healthServer.URL[strings.LastIndex(healthServer.URL, ":")+1:]

	root := t.TempDir()
	runtimeRoot := filepath.Join(root, "runtime")
	envFile := filepath.Join(runtimeRoot, "agentdock.env")
	cloudflared := filepath.Join(root, "fake-cloudflared")
	serviceActionLog := filepath.Join(root, "service-actions.log")

	// 假 cloudflared：输出 quick URL 成功行后立刻退出，驱动 scanner 走完回写链路。
	fakeCloudflared := "#!/bin/sh\n" +
		`echo "2026-09-13T00:00:00Z INF +--------------------------------------------------------------------------------------------+"` + "\n" +
		`echo "2026-09-13T00:00:00Z INF |  Your quick Tunnel has been created! Visit it at https://fresh.trycloudflare.com  |"` + "\n"
	if err := os.WriteFile(cloudflared, []byte(fakeCloudflared), 0o755); err != nil {
		t.Fatal(err)
	}
	serviceManager := "launchd"
	if runtime.GOOS == "darwin" {
		// 假 launchctl：print 回報 Core 正在執行，kickstart 計數並成功。
		launchctl := filepath.Join(root, "fake-launchctl")
		fakeLaunchctl := "#!/bin/sh\n" +
			`case "$1" in` + "\n" +
			`  print) echo 'state = running'; exit 0 ;;` + "\n" +
			`  kickstart) echo restart >> "$SERVICE_ACTION_LOG"; exit 0 ;;` + "\n" +
			`esac` + "\n" +
			"exit 1\n"
		if err := os.WriteFile(launchctl, []byte(fakeLaunchctl), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("AGENTDOCK_LAUNCHCTL_BIN", launchctl)
	} else {
		// Linux CI 不一定运行 systemd/OpenRC。manifest 明确声明 systemd，PATH 中
		// 放一个只记录 restart 的假 systemctl，测试真实的平台分发而不依赖宿主 init。
		serviceManager = "systemd"
		systemctl := filepath.Join(root, "systemctl")
		fakeSystemctl := "#!/bin/sh\n" +
			`case "$1" in` + "\n" +
			`  is-active) exit 0 ;;` + "\n" +
			`  restart) echo restart >> "$SERVICE_ACTION_LOG"; exit 0 ;;` + "\n" +
			`esac` + "\n" + "exit 1\n"
		if err := os.WriteFile(systemctl, []byte(fakeSystemctl), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	t.Setenv("SERVICE_ACTION_LOG", serviceActionLog)

	env := "AGENTDOCK_HOST=127.0.0.1\n" +
		"AGENTDOCK_PORT=" + healthPort + "\n" +
		"AGENTDOCK_AUTH_TOKEN=stable-bearer-token\n" +
		"AGENTDOCK_OAUTH_ENABLED=true\n" +
		"AGENTDOCK_OAUTH_PASSWORD=stable-oauth-password\n" +
		"AGENTDOCK_OAUTH_TOKEN_SECRET=stable-oauth-secret\n"
	if err := os.MkdirAll(runtimeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := unixRuntimeManifest{
		SchemaVersion:     1,
		ServiceManager:    serviceManager,
		ServiceName:       "com.uvwt.agentdock",
		TunnelServiceName: "com.uvwt.agentdock.cloudflared",
		AgentDockBinary:   filepath.Join(root, "agentdock"),
		CloudflaredBinary: cloudflared,
		EnvironmentFile:   envFile,
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, "desktop-runtime.json"), manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runQuickTunnel(context.Background(), manifest, root, runtimeRoot, "http://127.0.0.1:18766", io.Discard); err != nil {
		t.Fatalf("runQuickTunnel: %v", err)
	}

	updated, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	for _, want := range []string{
		"AGENTDOCK_SERVER_URL='https://fresh.trycloudflare.com'",
		"AGENTDOCK_OAUTH_ENABLED='true'",
		"AGENTDOCK_AUTH_TOKEN='stable-bearer-token'",
		"AGENTDOCK_OAUTH_PASSWORD='stable-oauth-password'",
		"AGENTDOCK_OAUTH_TOKEN_SECRET='stable-oauth-secret'",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("quick tunnel write-back missing %q in env:\n%s", want, text)
		}
	}
	urlBytes, err := os.ReadFile(filepath.Join(root, "quick-tunnel-url.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(urlBytes)) != "https://fresh.trycloudflare.com" {
		t.Fatalf("quick-tunnel-url.txt=%q", string(urlBytes))
	}
	serviceActions, err := os.ReadFile(serviceActionLog)
	if err != nil {
		t.Fatal(err)
	}
	if restarts := strings.Count(string(serviceActions), "restart"); restarts != 1 {
		t.Fatalf("core restart actions=%d, want 1", restarts)
	}
}
