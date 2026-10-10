package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/config"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

// AGENTDOCK_NEXT_SHARED_BROWSER_WORKSPACE is a temporary Next-only, explicit,
// single-workspace source opt-in. The absent variable preserves the existing
// exclusive per-lease ACP browser behavior. No GUI/config default enables it.
//
// Desktop variant and state root are safeguards against accidental activation
// in stable AgentDock; these environment checks are NOT a signed Core identity,
// consent attestor, or protection against malicious same-user OS processes.
const nextSharedBrowserWorkspaceEnv = "AGENTDOCK_NEXT_SHARED_BROWSER_WORKSPACE"

func newRuntimeACPBrowserBridge(cfg config.Config, planner *toolbrowser.RoutePlanner) (*toolbrowser.ACPBridge, error) {
	registry := toolbrowser.NewWorkerRegistry(toolbrowser.WorkerDependencies{})
	workspace, selected := os.LookupEnv(nextSharedBrowserWorkspaceEnv)
	if !selected {
		return toolbrowser.NewACPBridge(planner, registry)
	}
	root, err := nextSharedBrowserWorkspace(cfg, workspace)
	if err != nil {
		// Do not silently switch to exclusive: an invalid explicit opt-in
		// must not start unintended Chrome processes or attach a user profile.
		return nil, fmt.Errorf("Next shared ACP browser admission unavailable: %w", err)
	}
	return toolbrowser.NewNextSingleWorkspaceSharedACPBridge(planner, registry, root)
}

func nextSharedBrowserWorkspace(cfg config.Config, selected string) (string, error) {
	if runtime.GOOS != "darwin" || os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" || !cfg.ACPEnabled || !cfg.BrowserEnabled || cfg.Stdio {
		return "", fmt.Errorf("Next-only ACP Browser opt-in requires the macOS Next Core")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" || cfg.AgentDockHome != filepath.Join(home, ".agentdock-next") {
		return "", fmt.Errorf("Next-only ACP Browser state root is not qualified")
	}
	if selected == "" || !filepath.IsAbs(selected) || filepath.Clean(selected) != selected {
		return "", fmt.Errorf("explicit canonical browser workspace is required")
	}
	root, err := browserpolicy.CanonicalWorkspaceRoot(selected)
	if err != nil || selected != root {
		return "", fmt.Errorf("explicit canonical browser workspace is not qualified")
	}
	return root, nil
}
