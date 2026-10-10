package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/config"
	processcontrol "github.com/uvwt/agentdock/internal/process"
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

const sharedBrowserRestartMarker = ".shared-browser-restart-pending.v1"

func newRuntimeACPBrowserBridge(cfg config.Config, planner *toolbrowser.RoutePlanner) (*toolbrowser.ACPBridge, *sharedBrowserRestartGuard, error) {
	registry := toolbrowser.NewWorkerRegistry(toolbrowser.WorkerDependencies{})
	workspace, selected := os.LookupEnv(nextSharedBrowserWorkspaceEnv)
	if !selected {
		bridge, err := toolbrowser.NewACPBridge(planner, registry)
		return bridge, nil, err
	}
	root, err := nextSharedBrowserWorkspace(cfg, workspace)
	if err != nil {
		// Do not silently switch to exclusive: an invalid explicit opt-in
		// must not start unintended Chrome processes or attach a user profile.
		return nil, nil, fmt.Errorf("Next shared ACP browser admission unavailable: %w", err)
	}
	guard, err := acquireSharedBrowserRestartGuard(cfg.AgentDockHome)
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		inspection := inspectSharedBrowserOrphanEvidence(ctx, cfg.AgentDockHome, processcontrol.Observe)
		// Classification is advisory only; all statuses refuse a new worker.
		// Do not include PID, command line, workspace, or filesystem paths.
		return nil, nil, fmt.Errorf("Next shared ACP browser restart blocked (%s): %w", inspection.Status, err)
	}
	// The explicit Next-only registry persists a direct-child birth witness
	// immediately after cmd.Start, before MCP handshake, and requires an exact
	// matching started record before ready can publish a browser lease.
	// No stable/exclusive registry receives these hooks.
	guard.requireStartedEvidence = true
	registry = toolbrowser.NewWorkerRegistry(toolbrowser.WorkerDependencies{
		StartedWitness: guard.recordStartedWorker,
		ReadyWitness:   guard.recordManagedWorker,
	})
	bridge, err := toolbrowser.NewNextSingleWorkspaceSharedACPBridge(planner, registry, root)
	if err != nil {
		// This constructor never dispatches a worker. It is safe to clear a
		// marker only at this before-start boundary.
		if cleanupErr := guard.releaseAfterVerifiedClose(); cleanupErr != nil {
			return nil, nil, fmt.Errorf("Next shared ACP browser construction and rollback unavailable: %w", cleanupErr)
		}
		return nil, nil, err
	}
	return bridge, guard, nil
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
