//go:build darwin

package desktopruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMacOSVariantIsolation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_LAUNCHCTL_BIN", "/usr/bin/false")
	for _, tc := range []struct{ marker, name, label string }{
		{"", "AgentDock", "com.uvwt.agentdock"},
		{"stable", "AgentDock", "com.uvwt.agentdock"},
		{"next", "AgentDock Next", "dev.dropabit.agentdock.next"},
	} {
		t.Run(tc.marker, func(t *testing.T) {
			t.Setenv("AGENTDOCK_DESKTOP_VARIANT", tc.marker)
			want := filepath.Join(home, "Library", "Application Support", tc.name)
			if got := DefaultRuntimeRoot(); got != want {
				t.Fatalf("root %q, want %q", got, want)
			}
			logs, err := macOSLogDir()
			if err != nil || logs != filepath.Join(home, "Library", "Logs", tc.name) {
				t.Fatalf("logs %q: %v", logs, err)
			}
			if tc.marker == "next" {
				manifest, _, err := loadUnixRuntimeForExecutable(want, filepath.Join(home, "AgentDock Next.app/Contents/Helpers/agentdock"))
				if err != nil || manifest.ServiceName != tc.label+".core" || manifest.TunnelServiceName != tc.label+".tunnel" || manifest.ServiceManager != "smappservice" {
					t.Fatalf("Next manifest: %+v %v", manifest, err)
				}
				if got := healthURL(map[string]string{}); got != "http://127.0.0.1:8767/healthz" {
					t.Fatal(got)
				}
			}
		})
	}
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "unknown")
	if DefaultRuntimeRoot() != "" {
		t.Fatal("unknown variant fell back to stable")
	}
	if _, err := macOSLogDir(); err == nil {
		t.Fatal("unknown log identity accepted")
	}
	if _, _, err := loadUnixRuntime(home); err == nil {
		t.Fatal("unknown service identity accepted")
	}
	if err := platformPrepareLaunchEnvironment("core"); err == nil {
		t.Fatal("unknown launch identity accepted")
	}
}

func TestNextLaunchDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv("AGENTDOCK_HOME", "")
	t.Setenv("AGENTDOCK_DEFAULT_DIR", "")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	if err := platformPrepareLaunchEnvironment("core"); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(home, "AgentDock Next")
	got, _ := os.Getwd()
	// macOS /var and /tmp can be symlink aliases.
	resolved, _ := filepath.EvalSymlinks(work)
	if got != resolved {
		t.Fatalf("cwd %q, want %q", got, resolved)
	}
	if os.Getenv("AGENTDOCK_HOME") != filepath.Join(home, ".agentdock-next") || os.Getenv("AGENTDOCK_DEFAULT_DIR") != work {
		t.Fatal("Next environment not isolated")
	}
	for _, path := range []string{"AgentDock", "Library/Logs/AgentDock", ".agentdock"} {
		if _, err := os.Stat(filepath.Join(home, path)); !os.IsNotExist(err) {
			t.Fatalf("stable fixture touched: %s", path)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "Library/Logs/AgentDock Next")); err != nil {
		t.Fatal(err)
	}
}
