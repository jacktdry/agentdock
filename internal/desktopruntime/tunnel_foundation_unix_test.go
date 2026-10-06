//go:build darwin || linux

package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/uvwt/agentdock/internal/envstore"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func assertTunnelPrivateMode(t *testing.T, info os.FileInfo) {
	t.Helper()
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("marker mode %o", info.Mode().Perm())
	}
}

func tunnelStoppedFixture(t *testing.T) (string, unixRuntimeManifest, string) {
	t.Helper()
	root := t.TempDir()
	actions := filepath.Join(root, "actions")
	script := "#!/bin/sh\ncase \"$1\" in\nprint) echo 'state = waiting'; exit 0 ;;\nis-active) exit 3 ;;\nis-enabled) exit 1 ;;\nesac\necho \"$1\" >> \"$TUNNEL_TEST_ACTIONS\"\nexit 0\n"
	name := "launchctl"
	manager := "launchd"
	if runtime.GOOS == "linux" {
		name = "systemctl"
		manager = "systemd"
	}
	fake := filepath.Join(root, name)
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTDOCK_LAUNCHCTL_BIN", fake)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "")
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TUNNEL_TEST_ACTIONS", actions)
	manifest := unixRuntimeManifest{SchemaVersion: 1, ServiceManager: manager, ServiceName: "fixture.next.core", TunnelServiceName: "fixture.next.tunnel", AgentDockBinary: filepath.Join(root, "agentdock"), EnvironmentFile: filepath.Join(root, "agentdock.env"), TunnelEnvironment: filepath.Join(root, "cloudflared.env")}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeEnvironment(manifest.EnvironmentFile, map[string]string{"AGENTDOCK_HOST": "127.0.0.1", "AGENTDOCK_PORT": "8765", "AGENTDOCK_SERVER_URL": "https://old.trycloudflare.com", "AGENTDOCK_OAUTH_ENABLED": "true"}); err != nil {
		t.Fatal(err)
	}
	if err := writeEnvironment(manifest.TunnelEnvironment, map[string]string{"AGENTDOCK_TUNNEL_MODE": "quick", "AGENTDOCK_TUNNEL_TARGET": "http://127.0.0.1:8765"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "quick-tunnel-url.txt"), []byte("https://old.trycloudflare.com"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, manifest, actions
}

func TestTunnelConfigurePreservesStoppedServicesAndInvalidatesQuickURL(t *testing.T) {
	for _, mode := range []string{"none", "quick", "named"} {
		t.Run(mode, func(t *testing.T) {
			root, manifest, actions := tunnelStoppedFixture(t)
			request := TunnelConfigureRequest{RuntimeRoot: root, Mode: mode}
			if mode == "named" {
				request.ServerURL = "https://next.example"
				if err := os.WriteFile(filepath.Join(root, "cloudflare-tunnel-token"), []byte("fixture-token"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			release, err := AcquireDesktopMutation(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if err := platformConfigureTunnel(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(actions); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("configure changed stopped service lifecycle")
			}
			if _, err := os.Stat(filepath.Join(root, "quick-tunnel-url.txt")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("stale Quick ready URL retained")
			}
			core, err := envstore.ParseFile(manifest.EnvironmentFile)
			if err != nil {
				t.Fatal(err)
			}
			if mode != "named" && core["AGENTDOCK_SERVER_URL"] != "" {
				t.Fatal("stale OAuth origin retained")
			}
			if core["AGENTDOCK_OAUTH_ENABLED"] != tunnelBool(mode == "named") {
				t.Fatal("invalid intermediate OAuth state")
			}
			if mode == "quick" || mode == "named" {
				if len(core["AGENTDOCK_OAUTH_PASSWORD"]) != 24 || len(core["AGENTDOCK_OAUTH_TOKEN_SECRET"]) != 64 {
					t.Fatal("public tunnel configuration did not provision OAuth credentials")
				}
			}
			gen, err := TunnelGeneration(root)
			if err != nil || gen == "" {
				t.Fatal("configuration generation absent")
			}
		})
	}
}

func TestTunnelConfigurePreservesExistingOAuthCredentials(t *testing.T) {
	root, manifest, _ := tunnelStoppedFixture(t)
	core, err := envstore.ParseFile(manifest.EnvironmentFile)
	if err != nil {
		t.Fatal(err)
	}
	core["AGENTDOCK_OAUTH_PASSWORD"] = "existing-password"
	core["AGENTDOCK_OAUTH_TOKEN_SECRET"] = "existing-signing-secret"
	if err := writeEnvironment(manifest.EnvironmentFile, core); err != nil {
		t.Fatal(err)
	}
	if err := platformConfigureTunnel(context.Background(), TunnelConfigureRequest{RuntimeRoot: root, Mode: "quick"}); err != nil {
		t.Fatal(err)
	}
	after, err := envstore.ParseFile(manifest.EnvironmentFile)
	if err != nil {
		t.Fatal(err)
	}
	if after["AGENTDOCK_OAUTH_PASSWORD"] != "existing-password" || after["AGENTDOCK_OAUTH_TOKEN_SECRET"] != "existing-signing-secret" {
		t.Fatal("existing OAuth credentials were rotated")
	}
}

func TestTunnelConfigureValidatesBeforeSideEffects(t *testing.T) {
	for _, request := range []TunnelConfigureRequest{
		{Mode: "invalid"}, {Mode: "named", ServerURL: "http://invalid"}, {Mode: "quick", TokenFile: "unexpected"}, {Mode: "named", ServerURL: "https://next.example", TokenFile: "missing"},
	} {
		root, manifest, actions := tunnelStoppedFixture(t)
		request.RuntimeRoot = root
		before, _ := os.ReadFile(manifest.EnvironmentFile)
		if err := platformConfigureTunnel(context.Background(), request); err == nil {
			t.Fatal("invalid request accepted")
		}
		after, _ := os.ReadFile(manifest.EnvironmentFile)
		if string(before) != string(after) {
			t.Fatal("invalid request changed config")
		}
		if _, err := os.Stat(actions); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("invalid request invoked lifecycle")
		}
		if gen, _ := TunnelGeneration(root); gen != "" {
			t.Fatal("invalid request advanced generation")
		}
	}
}

func TestTunnelNamedCustomPortFailsBeforeEffects(t *testing.T) {
	root, manifest, actions := tunnelStoppedFixture(t)
	if err := writeEnvironment(manifest.EnvironmentFile, map[string]string{"AGENTDOCK_PORT": "18767"}); err != nil {
		t.Fatal(err)
	}
	err := platformConfigureTunnel(context.Background(), TunnelConfigureRequest{RuntimeRoot: root, Mode: "named", ServerURL: "https://next.example"})
	if !errors.Is(err, ErrNamedManualRouteRequired) {
		t.Fatalf("custom Named port: %v", err)
	}
	if _, err := os.Stat(actions); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("custom Named port touched service")
	}
}

func TestTunnelStaleQuickGenerationRejectedAfterWaitingForLock(t *testing.T) {
	root, manifest, _ := tunnelStoppedFixture(t)
	old, err := AdvanceTunnelGenerationLocked(root)
	if err != nil {
		t.Fatal(err)
	}
	release, err := AcquireDesktopMutation(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		done <- applyQuickTunnelURLUnixGeneration(ctx, manifest, root, root, "https://stale.trycloudflare.com", old, "http://127.0.0.1:8765")
	}()
	select {
	case err := <-done:
		t.Fatalf("callback did not wait for lock: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	if err := platformConfigureTunnel(ctx, TunnelConfigureRequest{RuntimeRoot: root, Mode: "none"}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		t.Fatalf("callback ran under outer lock: %v", err)
	default:
	}
	release()
	if err := <-done; !errors.Is(err, ErrStaleTunnelGeneration) {
		t.Fatalf("stale callback: %v", err)
	}
	core, _ := envstore.ParseFile(manifest.EnvironmentFile)
	if core["AGENTDOCK_SERVER_URL"] != "" {
		t.Fatal("superseded callback overwrote new mode")
	}
}

func TestTunnelCurrentGenerationCallbackResumesAndPreservesStoppedCore(t *testing.T) {
	root, manifest, actions := tunnelStoppedFixture(t)
	gen, err := AdvanceTunnelGenerationLocked(root)
	if err != nil {
		t.Fatal(err)
	}
	release, err := AcquireDesktopMutation(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- applyQuickTunnelURLUnixGeneration(ctx, manifest, root, root, "https://fresh.trycloudflare.com", gen, "http://127.0.0.1:8765")
	}()
	select {
	case err := <-done:
		t.Fatalf("callback did not wait: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "quick-tunnel-url.txt"))
	if err != nil || !strings.Contains(string(data), "fresh") {
		t.Fatal("current callback did not publish")
	}
	if _, err := os.Stat(actions); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("callback started stopped Core")
	}
}

func TestTunnelTokenStateNeverRevealsBytes(t *testing.T) {
	root, _, _ := tunnelStoppedFixture(t)
	path := filepath.Join(root, "cloudflare-tunnel-token")
	if got := tunnelTokenStateUnix(root); got != "missing" {
		t.Fatal(got)
	}
	cases := []struct {
		data  string
		mode  os.FileMode
		state string
	}{{"private-fixture-token", 0o600, "stored"}, {"private-fixture-token", 0o644, "unreadable"}, {"", 0o600, "unreadable"}, {strings.Repeat("x", 16*1024+3), 0o600, "unreadable"}, {"token\nsecond", 0o600, "unreadable"}}
	for _, tc := range cases {
		if err := os.WriteFile(path, []byte(tc.data), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, tc.mode); err != nil {
			t.Fatal(err)
		}
		status, err := platformTunnelStatus(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(status)
		if status.Observation.TokenState != tc.state || strings.Contains(string(data), "private-fixture-token") {
			t.Fatalf("unsafe token status: %s", data)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "agentdock.env"), path); err != nil {
		t.Fatal(err)
	}
	if got := tunnelTokenStateUnix(root); got != "unreadable" {
		t.Fatal("token symlink accepted")
	}
	if _, err := configuredTunnelToken(root, filepath.Join(root, "absent")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("replacement token silently fell back")
	}
}

func TestTunnelRecoveryStorageRejectsSymlinkRootAndMarker(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	called := false
	err := runTunnelTransactionLocked(context.Background(), alias, nil, func(context.Context) error { called = true; return nil }, func(context.Context) error { return nil })
	if err == nil || called {
		t.Fatal("symlink root accepted")
	}
	foreign := filepath.Join(t.TempDir(), "foreign")
	if err := os.WriteFile(foreign, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, filepath.Join(root, tunnelRecoveryFile)); err != nil {
		t.Fatal(err)
	}
	err = runTunnelTransactionLocked(context.Background(), root, nil, func(context.Context) error { called = true; return nil }, func(context.Context) error { return nil })
	if !errors.Is(err, ErrTunnelRecoveryRequired) || called {
		t.Fatal("symlink recovery marker accepted")
	}
	data, _ := os.ReadFile(foreign)
	if string(data) != "untouched" {
		t.Fatal("symlink marker target changed")
	}
}

func TestTunnelCLIWaitsForDesktopMutationAndCancellationHasNoEffects(t *testing.T) {
	root, manifest, actions := tunnelStoppedFixture(t)
	before, _ := os.ReadFile(manifest.EnvironmentFile)
	release, err := AcquireDesktopMutation(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
	defer cancel()
	if err := RunTunnelCommand(ctx, []string{"configure", "--runtime-root", root, "--mode", "quick"}, io.Discard, io.Discard); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("CLI bypassed Shared lock", err)
	}
	after, _ := os.ReadFile(manifest.EnvironmentFile)
	if string(before) != string(after) {
		t.Fatal("CLI changed locked config")
	}
	if _, err := os.Stat(actions); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("CLI changed service")
	}
	release()
	if err := RunTunnelCommand(context.Background(), []string{"configure", "--runtime-root", root, "--mode", "quick"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	core, _ := envstore.ParseFile(manifest.EnvironmentFile)
	if core["AGENTDOCK_SERVER_URL"] != "" || core["AGENTDOCK_OAUTH_ENABLED"] != "false" {
		t.Fatal("Quick intermediate OAuth invalid")
	}
}

func TestQuickInvalidationDisablesOAuthUntilCurrentCallback(t *testing.T) {
	root, manifest, _ := tunnelStoppedFixture(t)
	gen, err := AdvanceTunnelGenerationLocked(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := invalidateQuickTunnelUnixLocked(root); err != nil {
		t.Fatal(err)
	}
	core, _ := envstore.ParseFile(manifest.EnvironmentFile)
	if core["AGENTDOCK_SERVER_URL"] != "" || core["AGENTDOCK_OAUTH_ENABLED"] != "false" {
		t.Fatal("invalidated origin left OAuth enabled")
	}
	if err := applyQuickTunnelURLUnixGeneration(context.Background(), manifest, root, root, "https://fresh.trycloudflare.com", gen, "http://127.0.0.1:8765"); err != nil {
		t.Fatal(err)
	}
	core, _ = envstore.ParseFile(manifest.EnvironmentFile)
	if core["AGENTDOCK_SERVER_URL"] != "https://fresh.trycloudflare.com" || core["AGENTDOCK_OAUTH_ENABLED"] != "true" {
		t.Fatal("current generation did not publish OAuth atomically")
	}
}

func TestTunnelCLIRejectsNextBeforeLockOrEffects(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	root := t.TempDir()
	for _, action := range []string{"start", "stop", "restart", "regenerate", "configure", "autostart"} {
		args := []string{action, "--runtime-root", root}
		if action == "configure" {
			args = append(args, "--mode", "quick")
		}
		if action == "autostart" {
			args = append(args, "--enabled", "true")
		}
		if err := RunTunnelCommand(context.Background(), args, io.Discard, io.Discard); !errors.Is(err, ErrNextIdentityUnavailable) {
			t.Fatal("unowned Next accepted", action)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatal("rejected root received mutation files", action, entries, err)
		}
	}
}

func TestQuickPortChangeInvalidatesOriginAndDisablesOAuth(t *testing.T) {
	root, manifest, actions := tunnelStoppedFixture(t)
	if runtime.GOOS == "darwin" {
		// Select only signed fixture helpers; all service calls use fake launchctl.
		canonical, err := filepath.EvalSymlinks(root)
		if err != nil {
			t.Fatal(err)
		}
		root = canonical
		manifest.AgentDockBinary, manifest.CloudflaredBinary = nextHelperFixture(t, root)
		manifest.ServiceName = "dev.dropabit.agentdock.next.core"
		manifest.TunnelServiceName = "dev.dropabit.agentdock.next.tunnel"
		manifest.ServiceManager = "smappservice"
		manifest.EnvironmentFile = filepath.Join(root, "agentdock.env")
		manifest.TunnelEnvironment = filepath.Join(root, "cloudflared.env")
		data, _ := json.Marshal(manifest)
		if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	if err := platformUpdateBasicSettings(context.Background(), root, BasicSettings{Port: 19000, LogLevel: "info"}); err != nil {
		t.Fatal(err)
	}
	core, _ := envstore.ParseFile(manifest.EnvironmentFile)
	tunnel, _ := envstore.ParseFile(manifest.TunnelEnvironment)
	if core["AGENTDOCK_SERVER_URL"] != "" || core["AGENTDOCK_OAUTH_ENABLED"] != "false" || tunnel["AGENTDOCK_TUNNEL_TARGET"] != "http://127.0.0.1:19000" {
		t.Fatal("port change left invalid Quick OAuth state")
	}
	if _, err := os.Stat(actions); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("port save started stopped fixture")
	}
}
