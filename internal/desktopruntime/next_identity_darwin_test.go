//go:build darwin

package desktopruntime

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func nextHelperFixture(t *testing.T, root string) (string, string) {
	t.Helper()
	helpers := filepath.Join(root, "AgentDock Next.app/Contents/Helpers")
	if err := os.MkdirAll(helpers, 0700); err != nil {
		t.Fatal(err)
	}
	helperData, err := os.ReadFile("/usr/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"agentdock", "cloudflared"} {
		if err := os.WriteFile(filepath.Join(helpers, name), helperData, 0700); err != nil {
			t.Fatal(err)
		}
		identifier := "dev.dropabit.agentdock.next.core"
		if name == "cloudflared" {
			identifier = "dev.dropabit.agentdock.next.cloudflared"
		}
		if output, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", "--identifier", identifier, filepath.Join(helpers, name)).CombinedOutput(); err != nil {
			t.Fatal(string(output), err)
		}
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(helpers), "Info.plist"), []byte(`<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>dev.dropabit.agentdock.next</string><key>AgentDockVariant</key><string>next</string></dict></plist>`), 0600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(helpers, "agentdock"), filepath.Join(helpers, "cloudflared")
}

func TestNextBundledStartupIgnoresExternalHelperRedirection(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	root, _ := filepath.EvalSymlinks(t.TempDir())
	core, cloud := nextHelperFixture(t, root)
	// Boot requires no external manifest.
	manifest, _, err := loadUnixRuntimeForExecutable(root, core)
	if err != nil || manifest.AgentDockBinary != core || manifest.CloudflaredBinary != cloud {
		t.Fatal(manifest, err)
	}
	if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), []byte(`{"agentdock_binary":"/arbitrary/core","cloudflared_binary":"/arbitrary/cloud"}`), 0600); err != nil {
		t.Fatal(err)
	}
	manifest, _, err = loadUnixRuntimeForExecutable(root, core)
	if err != nil || manifest.AgentDockBinary != core || manifest.CloudflaredBinary != cloud {
		t.Fatal("external redirect", manifest, err)
	}
}

func TestNextBundledConnectionNeedsNoExternalManifest(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	root, _ := filepath.EvalSymlinks(t.TempDir())
	core, cloud := nextHelperFixture(t, root)
	manifest, err := explicitNextDarwinRuntimeForExecutable(root, core)
	if err != nil || manifest.AgentDockBinary != core || manifest.CloudflaredBinary != cloud {
		t.Fatal(manifest, err)
	}
}

func TestNextDevIdentityRejectsHelperAliasesAndUnrelatedBinaries(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	root, _ := filepath.EvalSymlinks(t.TempDir())
	core, cloud := nextHelperFixture(t, root)
	manifest := unixRuntimeManifest{SchemaVersion: 1, ServiceManager: "smappservice", ServiceName: "dev.dropabit.agentdock.next.core", TunnelServiceName: "dev.dropabit.agentdock.next.tunnel", AgentDockBinary: core, CloudflaredBinary: cloud, EnvironmentFile: filepath.Join(root, "agentdock.env"), TunnelEnvironment: filepath.Join(root, "cloudflared.env")}
	write := func() {
		data, _ := json.Marshal(manifest)
		if err := os.WriteFile(filepath.Join(root, "desktop-runtime.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if _, err := explicitNextDarwinRuntime(root); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(root, "arbitrary-helper")
	if err := os.WriteFile(unrelated, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cloud); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(unrelated, cloud); err != nil {
		t.Fatal(err)
	}
	if _, err := explicitNextDarwinRuntime(root); err == nil {
		t.Fatal("symlink helper accepted")
	}
	manifest.CloudflaredBinary = unrelated
	write()
	if _, err := explicitNextDarwinRuntime(root); err == nil {
		t.Fatal("arbitrary helper accepted")
	}
}

func TestNextDevIdentityRejectsUnsignedHelperAtExpectedPath(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	core, cloud := nextHelperFixture(t, root)
	manifest := unixRuntimeManifest{AgentDockBinary: core, CloudflaredBinary: cloud}
	if err := validateNextDarwinHelpers(manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cloud, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateNextDarwinHelpers(manifest); err == nil {
		t.Fatal("arbitrary helper at expected path accepted")
	}
}
