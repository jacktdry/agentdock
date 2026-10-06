//go:build darwin || linux

package desktopruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeACPProbeExecutable(t *testing.T, path, output string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo '" + output + "'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProbeACPProfileReportsConfiguredAntigravityVersion(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	command := writeACPProbeExecutable(t, filepath.Join(home, ".agentdock-next", "bin", "antigravity-acp"), "1.2.0-agentdock.6")
	probe, err := ProbeACPProfile(context.Background(), root, "antigravity", ACPProfileSettings{
		ID: "antigravity", Kind: "custom", Command: command, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe.Availability != "available" || probe.Command != command || probe.InstalledVersion != "1.2.0-agentdock.6" || probe.VersionState != "not_checked" {
		t.Fatalf("probe = %#v", probe)
	}
}

func TestProbeACPProfileDiscoversCodexFromPATH(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	command := writeACPProbeExecutable(t, filepath.Join(bin, "codex-acp"), "@agentclientprotocol/codex-acp 2.1.1")
	t.Setenv("PATH", bin)
	probe, err := ProbeACPProfile(context.Background(), root, "codex", ACPProfileSettings{ID: "codex", Kind: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if probe.Availability != "available" || probe.Command != command || probe.InstalledVersion != "2.1.1" {
		t.Fatalf("probe = %#v", probe)
	}
}

func TestProbeACPProfileDoesNotRunVersionForGenericCustomAdapter(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "marker")
	command := filepath.Join(root, "custom-acp")
	script := "#!/bin/sh\necho touched > '" + marker + "'\n"
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	probe, err := ProbeACPProfile(context.Background(), root, "custom", ACPProfileSettings{ID: "custom-one", Kind: "custom", Command: command})
	if err != nil {
		t.Fatal(err)
	}
	if probe.Availability != "available" || probe.VersionState != "unsupported" || probe.InstalledVersion != "" {
		t.Fatalf("probe = %#v", probe)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("generic custom adapter was executed: %v", err)
	}
}

func TestProbeACPProfileAntigravityDoesNotFallbackToGlobalPATHForNext(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	globalBin := filepath.Join(t.TempDir(), "global")
	writeACPProbeExecutable(t, filepath.Join(globalBin, "antigravity-acp"), "1.2.0-agentdock.5")
	t.Setenv("HOME", home)
	t.Setenv("PATH", globalBin)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")

	nextCommand := writeACPProbeExecutable(t, filepath.Join(home, ".agentdock-next", "bin", "antigravity-acp"), "1.2.0-agentdock.6")
	probe, err := ProbeACPProfile(context.Background(), root, "antigravity", ACPProfileSettings{ID: "antigravity", Kind: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	if probe.Command != nextCommand || probe.InstalledVersion != "1.2.0-agentdock.6" {
		t.Fatalf("probe = %#v", probe)
	}
}

func TestProbeACPProfileDoesNotRunVersionForExternalAntigravity(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	command := writeACPProbeExecutable(t, filepath.Join(t.TempDir(), "antigravity-acp"), "9.9.9")
	probe, err := ProbeACPProfile(context.Background(), root, "antigravity", ACPProfileSettings{
		ID: "antigravity", Kind: "custom", Command: command,
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe.Availability != "available" || probe.InstalledVersion != "" || probe.VersionState != "unsupported" {
		t.Fatalf("probe = %#v", probe)
	}
}
