//go:build darwin || linux

package desktopruntime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNextConnectionAuthReadsOnly(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manifest := unixRuntimeManifest{SchemaVersion: 1, ServiceManager: "smappservice", ServiceName: "dev.dropabit.agentdock.next.core", TunnelServiceName: "dev.dropabit.agentdock.next.tunnel", AgentDockBinary: filepath.Join(root, "core"), CloudflaredBinary: filepath.Join(root, "cloudflared"), EnvironmentFile: filepath.Join(root, "agentdock.env"), TunnelEnvironment: filepath.Join(root, "cloudflared.env")}
	if runtime.GOOS == "darwin" {
		manifest.AgentDockBinary, manifest.CloudflaredBinary = nextHelperFixture(t, root)
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "agentdock.env")
	original := []byte("AGENTDOCK_DESKTOP_VARIANT=next\nAGENTDOCK_OAUTH_PASSWORD=fixture-password\nAGENTDOCK_AUTH_TOKEN=BEARER_SENTINEL\nAGENTDOCK_OAUTH_TOKEN_SECRET=SIGNING_SENTINEL\nAGENTDOCK_SERVER_URL=https://example.test\nAGENTDOCK_OAUTH_ENABLED=true\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cloudflared.env"), []byte("AGENTDOCK_TUNNEL_MODE=named\nTUNNEL_TOKEN=TUNNEL_SENTINEL\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := ReadConnectionConfig(context.Background(), root)
	if err != nil || config.Port != 8767 || config.CoreEndpoint != "http://127.0.0.1:8767" || config.Mode != "named" {
		t.Fatalf("%+v %v", config, err)
	}
	password, state := ReadOAuthPassword(context.Background(), root)
	if password != "fixture-password" || state != OAuthPasswordStored {
		t.Fatal("stored password unavailable")
	}
	if state := ReadOAuthPasswordState(context.Background(), root); state != OAuthPasswordStored {
		t.Fatal("state-only stored read failed")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("read changed environment")
	}
	entries, _ := os.ReadDir(root)
	expected := 3
	if runtime.GOOS == "darwin" {
		expected++
	}
	if len(entries) != expected {
		t.Fatal("read created runtime files")
	}
	if err := os.WriteFile(path, []byte("AGENTDOCK_DESKTOP_VARIANT=next\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if password, state := ReadOAuthPassword(context.Background(), root); password != "" || state != OAuthPasswordMissing {
		t.Fatal("missing password generated")
	}
	if state := ReadOAuthPasswordState(context.Background(), root); state != OAuthPasswordMissing {
		t.Fatal("state-only missing read failed")
	}
	if err := os.WriteFile(path, []byte("invalid env content PASSWORD_SENTINEL"), 0600); err != nil {
		t.Fatal(err)
	}
	if password, state := ReadOAuthPassword(context.Background(), root); password != "" || state != OAuthPasswordUnreadable {
		t.Fatal("parse failure not safe")
	}
}

func TestNextConnectionAuthRejectsUnownedStorage(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	root, _ := filepath.EvalSymlinks(t.TempDir())
	foreign := filepath.Join(root, "foreign.env")
	manifest := unixRuntimeManifest{SchemaVersion: 1, ServiceManager: "smappservice", ServiceName: "dev.dropabit.agentdock.next.core", TunnelServiceName: "dev.dropabit.agentdock.next.tunnel", AgentDockBinary: filepath.Join(root, "core"), CloudflaredBinary: filepath.Join(root, "cloudflared"), EnvironmentFile: filepath.Join(root, "agentdock.env"), TunnelEnvironment: filepath.Join(root, "cloudflared.env")}
	if runtime.GOOS == "darwin" {
		manifest.AgentDockBinary, manifest.CloudflaredBinary = nextHelperFixture(t, root)
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "agentdock.env")
	if err := os.WriteFile(foreign, []byte("AGENTDOCK_DESKTOP_VARIANT=next\nAGENTDOCK_OAUTH_PASSWORD=fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, path); err != nil {
		t.Fatal(err)
	}
	if password, state := ReadOAuthPassword(context.Background(), root); password != "" || state != OAuthPasswordUnreadable {
		t.Fatal("symlink credential accepted")
	}
	for _, marker := range []string{"", "stable", "unknown"} {
		t.Setenv("AGENTDOCK_DESKTOP_VARIANT", marker)
		if _, err := ReadConnectionConfig(context.Background(), root); err == nil {
			t.Fatal("non-Next identity accepted")
		}
		if password, state := ReadOAuthPassword(context.Background(), root); password != "" || state != OAuthPasswordUnavailable {
			t.Fatal("non-Next identity exposed password")
		}
	}
}
