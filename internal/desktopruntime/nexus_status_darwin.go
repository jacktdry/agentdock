//go:build darwin

package desktopruntime

import (
	"context"
	"errors"

	"github.com/uvwt/agentdock/internal/desktopcontrol"
)

// ReadVerifiedNextNexusRuntimeStatus only trusts Nexus connection evidence from
// the selected Next launchd process. The Unix socket peer PID/UID is verified
// by desktopcontrol, and the process binary + creation identity are checked
// both before and after the request to fence PID reuse.
func ReadVerifiedNextNexusRuntimeStatus(ctx context.Context, runtimeRoot string) (ServiceStatus, error) {
	selection, err := SelectNextConnectionRuntime(ctx, runtimeRoot)
	if err != nil {
		return ServiceStatus{}, ErrNextIdentityUnavailable
	}
	if selection.Running == nil {
		return ServiceStatus{}, ErrNextIdentityUnavailable
	}
	if !*selection.Running {
		return ServiceStatus{Running: false, Healthy: false}, nil
	}
	pid := selection.PortRuntime.PID
	instance := selection.PortRuntime.ProcessInstance
	if pid <= 0 || instance == "" || selection.PortRuntime.Binary == "" {
		return ServiceStatus{}, ErrNextIdentityUnavailable
	}
	if err := verifyNextCoreProcess(pid, instance, selection.PortRuntime.Binary); err != nil {
		return ServiceStatus{}, err
	}
	var status ServiceStatus
	if err := desktopcontrol.CallVerifiedPID(ctx, runtimeRoot, pid, "service.status", controlActionParams{RuntimeRoot: runtimeRoot}, &status); err != nil {
		return ServiceStatus{}, ErrNextIdentityUnavailable
	}
	if err := verifyNextCoreProcess(pid, instance, selection.PortRuntime.Binary); err != nil {
		return ServiceStatus{}, err
	}
	return status, nil
}

func verifyNextCoreProcess(pid int, expectedInstance, expectedBinary string) error {
	binary, instance, err := platformPortProcess(pid)
	if err != nil || instance == "" || instance != expectedInstance || !portBinaryMatches(binary, expectedBinary) {
		return errors.New("next core process identity changed")
	}
	return nil
}
