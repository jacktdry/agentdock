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
	home := t.TempDir()
	t.Setenv("HOME", home)
	nextState := filepath.Join(home, ".agentdock-next")
	if err := os.Mkdir(nextState, 0700); err != nil {
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
	baseline, defaultGuard, err := newRuntimeACPBrowserBridge(cfg, planner)
	if err != nil {
		t.Fatal(err)
	}
	if defaultGuard != nil {
		t.Fatal("exclusive unexpectedly took a restart guard")
	}
	if baseline.Diagnostics().SharedBudget != nil {
		t.Fatal("default ACPBridge unexpectedly shared")
	}
	if err := baseline.Close(); err != nil {
		t.Fatal(err)
	}

	t.Setenv(nextSharedBrowserWorkspaceEnv, root)
	if runtime.GOOS != "darwin" {
		if _, _, err := newRuntimeACPBrowserBridge(cfg, planner); err == nil {
			t.Fatal("non-macOS host enabled Next shared browser")
		}
		return
	}
	bridge, restartGuard, err := newRuntimeACPBrowserBridge(cfg, planner)
	if err != nil {
		t.Fatal(err)
	}
	if restartGuard == nil {
		t.Fatal("Next shared bridge omitted durable restart guard")
	}
	defer func() {
		if err := (&Runtime{acpBrowser: bridge, acpSharedRestartGuard: restartGuard}).Close(); err != nil {
			t.Error(err)
		}
		if _, err := os.Lstat(filepath.Join(cfg.AgentDockHome, sharedBrowserRestartMarker)); !os.IsNotExist(err) {
			t.Errorf("clean bridge left startup marker: %v", err)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
			if b, g, err := newRuntimeACPBrowserBridge(cfg, planner); err == nil {
				_ = b.Close()
				if g != nil {
					_ = g.releaseAfterVerifiedClose()
				}
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
		if b, g, err := newRuntimeACPBrowserBridge(base, planner); err == nil {
			_ = b.Close()
			if g != nil {
				_ = g.releaseAfterVerifiedClose()
			}
			t.Fatal("symlink workspace alias accepted")
		}
	}
	// An explicit but invalid opt-in must not degrade into an exclusive pool.
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "stable")
	t.Setenv(nextSharedBrowserWorkspaceEnv, root)
	if _, _, err := newRuntimeACPBrowserBridge(base, planner); err == nil || !strings.Contains(err.Error(), "Next shared ACP browser") {
		t.Fatalf("invalid selection incorrectly fell back: %v", err)
	}
}

// Disabling a previously enabled shared mode must not use the otherwise
// exclusive startup path to bypass a persistent orphan/restart guard.
func TestRuntimeACPBrowserExclusiveRejectsSurvivingSharedGuard(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv(nextSharedBrowserWorkspaceEnv, "")
	// Unset rather than empty opt-in: an empty explicit opt-in is rejected.
	if err := os.Unsetenv(nextSharedBrowserWorkspaceEnv); err != nil {
		t.Fatal(err)
	}
	stateRoot := filepath.Join(home, ".agentdock-next")
	if err := os.Mkdir(stateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(stateRoot, sharedBrowserRestartMarker)
	if err := os.WriteFile(marker, []byte("do not remove orphan evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := toolbrowser.NewRoutePlanner(nil, catalog, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AgentDockHome: stateRoot, ACPEnabled: true, BrowserEnabled: true}
	if b, g, err := newRuntimeACPBrowserBridge(cfg, planner); err == nil || b != nil || g != nil {
		t.Fatalf("exclusive started despite surviving guard: bridge=%v guard=%v err=%v", b, g, err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "do not remove orphan evidence" {
		t.Fatal("denial modified the shared restart marker")
	}
}
