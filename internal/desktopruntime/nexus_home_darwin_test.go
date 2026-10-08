//go:build darwin

package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func nexusHomeFixture(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv("AGENTDOCK_HOME", "")
	home, _ := filepath.EvalSymlinks(t.TempDir())
	root := filepath.Join(home, "Library/Application Support/AgentDock Next")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	core, cloud := nextHelperFixture(t, home)
	m := unixRuntimeManifest{SchemaVersion: 1, ServiceManager: "smappservice", ServiceName: "dev.dropabit.agentdock.next.core", TunnelServiceName: "dev.dropabit.agentdock.next.tunnel", AgentDockBinary: core, CloudflaredBinary: cloud, EnvironmentFile: filepath.Join(root, "agentdock.env"), TunnelEnvironment: filepath.Join(root, "cloudflared.env")}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return home, root
}

func TestNexusHomeAuthority(t *testing.T) {
	home, root := nexusHomeFixture(t)
	resolve := func(root string) (string, error) {
		return resolveNextNexusHome(context.Background(), root, home, explicitNextDarwinRuntime)
	}
	state, err := resolve(root)
	if err != nil || state != filepath.Join(home, ".agentdock-next") || state == root {
		t.Fatal(state, err)
	}
	if _, err := os.Stat(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("resolver created state")
	}
	if _, err := readNextNexusIdentity(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, bad := range []string{filepath.Join(home, "Library/Application Support/AgentDock"), filepath.Join(home, ".agentdock"), state, ""} {
		if _, err := resolve(bad); !errors.Is(err, ErrNextIdentityUnavailable) {
			t.Fatal("foreign root", bad, err)
		}
	}
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "stable")
	if _, err := resolve(root); err == nil {
		t.Fatal("stable variant")
	}
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv("AGENTDOCK_HOME", filepath.Join(home, ".agentdock"))
	if got, err := resolve(root); err != nil || got != state {
		t.Fatal("ambient stable AGENTDOCK_HOME overrode selected Next identity", got, err)
	}
	t.Setenv("AGENTDOCK_HOME", "")
	if err := os.WriteFile(filepath.Join(root, "agentdock.env"), []byte("AGENTDOCK_DESKTOP_VARIANT=next\nAGENTDOCK_HOME="+filepath.Join(home, ".agentdock")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(root); err == nil {
		t.Fatal("env state mismatch")
	}
	if err := os.WriteFile(filepath.Join(root, "agentdock.env"), []byte("AGENTDOCK_DESKTOP_VARIANT=next\nAGENTDOCK_HOME="+state+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(root); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(home, "runtime-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(alias); err == nil {
		t.Fatal("runtime symlink")
	}
	if err := os.Symlink(filepath.Join(home, ".agentdock"), state); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(root); err == nil {
		t.Fatal("dangling state symlink")
	}
}

func TestNexusIdentityReadOnlyAndNoFollow(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	state := filepath.Join(home, ".agentdock-next")
	nexus := filepath.Join(state, "nexus")
	if err := os.MkdirAll(nexus, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nexus, "device.json")
	data := []byte(`{"device_token":"fixture-secret"}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	for i := 0; i < 2; i++ {
		got, err := readNextNexusIdentity(state)
		if err != nil || string(got) != string(data) {
			t.Fatal(string(got), err)
		}
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
		t.Fatal("read mutated identity")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readNextNexusIdentity(state); err == nil {
		t.Fatal("public token file")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, ".agentdock/nexus/device.json"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := readNextNexusIdentity(state); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatal("file symlink", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(nexus); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(home, nexus); err != nil {
		t.Fatal(err)
	}
	if _, err := readNextNexusIdentity(state); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatal("directory symlink", err)
	}
}
