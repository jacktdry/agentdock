package browser

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"time"

	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	processcontrol "github.com/uvwt/agentdock/internal/process"
)

const ManagedEnginePackage = "chrome-devtools-mcp"
const ManagedEngineVersion = PreferredEngineVersion
const ManagedEngineServerName = "chrome_devtools"
const ManagedEnginePackageSpec = ManagedEnginePackage + "@" + ManagedEngineVersion

func ManagedWorkerConfig(cwd string) mcpclient.ServerConfig {
	return mcpclient.ServerConfig{Name: ManagedEngineServerName, Transport: mcpclient.TransportStdio, Command: "npx", Cwd: cwd, TimeoutMS: 30000, Args: []string{"--yes", ManagedEnginePackageSpec, "--isolated", "--headless", "--experimentalPageIdRouting", "--experimentalStructuredContent", "--no-usage-statistics", "--no-performance-crux"}}
}

// VersionProbe is injectable; production executes only the exact pinned package.
type VersionProbe func(context.Context, mcpclient.ServerConfig) (string, error)

func ProbeManagedVersion(ctx context.Context, cfg mcpclient.ServerConfig) (string, error) {
	return runOwnedVersionProbe(ctx, "npx", []string{"--yes", ManagedEnginePackageSpec, "--version"}, cfg.Cwd)
}

func runOwnedVersionProbe(ctx context.Context, command string, args []string, cwd string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = cwd
	// Command cancellation targets only its own process/controller. Wait has one
	// owner (the Wait call below); WaitDelay bounds descendant-held pipes after termination.
	processcontrol.Configure(cmd)
	cmd.WaitDelay = 2 * time.Second
	var mu sync.Mutex
	var controller *processcontrol.Controller
	cmd.Cancel = func() error {
		mu.Lock()
		defer mu.Unlock()
		if controller != nil {
			return controller.Terminate()
		}
		return cmd.Process.Kill()
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	mu.Lock()
	var attachErr error
	controller, attachErr = processcontrol.Attach(cmd)
	mu.Unlock()
	if attachErr != nil {
		_ = cmd.Process.Kill()
		return "", errors.Join(attachErr, cmd.Wait())
	}
	waitErr := cmd.Wait()
	var terminateErr error
	if waitErr != nil {
		terminateErr = controller.Terminate()
	}
	cleanupErr := errors.Join(terminateErr, controller.Close())
	if err := errors.Join(waitErr, cleanupErr); err != nil {
		return "", err
	}
	// Successful stdout is authoritative. npm update notices on stderr do not
	// contaminate version parsing; stderr is never treated as a version source.
	return strings.TrimSpace(stdout.String()), nil
}

func validateManagedVersion(version string) error {
	if strings.TrimSpace(version) != ManagedEngineVersion {
		return browserError(ErrEngineVersionIncompatible, "managed browser engine version incompatible", "preflight", &ErrorDetails{EngineVersion: strings.TrimSpace(version)}, nil)
	}
	return nil
}
