package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/config"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

// Selection tests only construct source-only bridges. They intentionally never
// call Acquire in the authorized workspace, so no npx/Chrome is started.
func TestRuntimeACPBrowserNextSharedWorkspaceOptIn(t *testing.T) {
	previous, had := os.LookupEnv(nextSharedBrowserWorkspaceEnv)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(nextSharedBrowserWorkspaceEnv, previous)
		} else {
			_ = os.Unsetenv(nextSharedBrowserWorkspaceEnv)
		}
	})
	_ = os.Unsetenv(nextSharedBrowserWorkspaceEnv)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	root, err := browserpolicy.CanonicalWorkspaceRoot(tmp)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		AgentDockHome:       filepath.Join(home, ".agentdock-next"),
		AgentDockDefaultDir: root,
		ACPEnabled:          true, BrowserEnabled: true,
	}
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := toolbrowser.NewRoutePlanner(nil, catalog, nil)
	if err != nil {
		t.Fatal(err)
	}

	// No opt-in in a Next Core must preserve the original exclusive behavior.
	baseline, err := newRuntimeACPBrowserBridge(cfg, planner)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Diagnostics().SharedBudget != nil {
		t.Fatal("default ACPBridge unexpectedly shared")
	}
	if err := baseline.Close(); err != nil {
		t.Fatal(err)
	}

	t.Setenv(nextSharedBrowserWorkspaceEnv, root)
	if runtime.GOOS != "darwin" {
		if _, err := newRuntimeACPBrowserBridge(cfg, planner); err == nil {
			t.Fatal("non-macOS host enabled Next shared browser")
		}
		return
	}
	bridge, err := newRuntimeACPBrowserBridge(cfg, planner)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := bridge.Close(); err != nil {
			t.Error(err)
		}
	}()
	snapshot := bridge.Diagnostics().SharedBudget
	if snapshot == nil || snapshot.CapacitySlots != 1 || snapshot.AvailableSlots != 1 || snapshot.OccupiedSlots != 0 || snapshot.ClaimedOwners != 1 {
		t.Fatalf("unexpected Next shared runtime budget: %+v", snapshot)
	}
	t1, err := bridge.RegisterSession("codex", "codex", root)
	if err != nil {
		t.Fatal(err)
	}
	t2, err := bridge.RegisterSession("agy", "antigravity", root)
	if err != nil || t1 == t2 {
		t.Fatalf("Next owners not distinct, err=%v", err)
	}

	// Registering another ACP workspace is harmless for its coding session,
	// but Acquire MUST fail before launching any managed/external worker.
	other, err := browserpolicy.CanonicalWorkspaceRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := bridge.RegisterSession("other", "codex", other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.Acquire(context.Background(), foreign, "about:blank"); err == nil {
		t.Fatal("foreign workspace acquired shared Chrome or fell back")
	}
	if state := bridge.Diagnostics().SharedBudget; state == nil || state.OccupiedSlots != 0 {
		t.Fatalf("foreign workspace spawned worker: %+v", state)
	}
	if err := bridge.ReleaseSession(context.Background(), "other"); err != nil {
		t.Fatal(err)
	}
	if err := bridge.ReleaseSession(context.Background(), "codex"); err != nil {
		t.Fatal(err)
	}
	if err := bridge.ReleaseSession(context.Background(), "agy"); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeACPBrowserNextSharedOptInDenials(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := browserpolicy.CanonicalWorkspaceRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := config.Config{
		AgentDockHome:       filepath.Join(home, ".agentdock-next"),
		AgentDockDefaultDir: root,
		ACPEnabled:          true, BrowserEnabled: true,
	}
	catalog, _ := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	planner, _ := toolbrowser.NewRoutePlanner(nil, catalog, nil)
	for _, tc := range []struct {
		name, variant, workspace string
		mutate                   func(*config.Config)
	}{
		{"stable-variant", "stable", root, nil},
		{"empty-variant", "", root, nil},
		{"wrong-home", "next", root, func(cfg *config.Config) { cfg.AgentDockHome = filepath.Join(home, ".agentdock") }},
		{"home-subpath", "next", root, func(cfg *config.Config) { cfg.AgentDockHome += "/nested" }},
		{"no-acp", "next", root, func(cfg *config.Config) { cfg.ACPEnabled = false }},
		{"no-browser", "next", root, func(cfg *config.Config) { cfg.BrowserEnabled = false }},
		{"stdio", "next", root, func(cfg *config.Config) { cfg.Stdio = true }},
		{"empty-workspace", "next", "", nil},
		{"relative-workspace", "next", ".", nil},
		{"dirty-workspace", "next", root + "/..", nil},
		{"nonexistent-workspace", "next", filepath.Join(root, "missing"), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGENTDOCK_DESKTOP_VARIANT", tc.variant)
			t.Setenv(nextSharedBrowserWorkspaceEnv, tc.workspace)
			cfg := base
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}
			if b, err := newRuntimeACPBrowserBridge(cfg, planner); err == nil {
				_ = b.Close()
				t.Fatal("invalid explicit opt-in silently accepted")
			}
		})
	}
	if runtime.GOOS == "darwin" {
		// Canonical symlink aliases never silently target a different path.
		alias := filepath.Join(t.TempDir(), "alias")
		if err := os.Symlink(root, alias); err != nil {
			t.Fatal(err)
		}
		t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
		t.Setenv(nextSharedBrowserWorkspaceEnv, alias)
		if b, err := newRuntimeACPBrowserBridge(base, planner); err == nil {
			_ = b.Close()
			t.Fatal("symlink workspace alias accepted")
		}
	}
	// An explicit but invalid opt-in must not degrade into an exclusive pool.
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "stable")
	t.Setenv(nextSharedBrowserWorkspaceEnv, root)
	if _, err := newRuntimeACPBrowserBridge(base, planner); err == nil || !strings.Contains(err.Error(), "Next shared ACP browser") {
		t.Fatalf("invalid selection incorrectly fell back: %v", err)
	}
}
