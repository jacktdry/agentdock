//go:build darwin

package desktopruntime

import (
	"context"

	"github.com/uvwt/agentdock/internal/desktopcontrol"
)

// ReadVerifiedNextNativeCodexReadiness queries the signed, selected Next Core
// over its PID/UID-verified native IPC channel; no bearer or TCP request.
func ReadVerifiedNextNativeCodexReadiness(ctx context.Context, root string) (NativeCodexReadiness, error) {
	empty := NativeCodexReadiness{}
	if ctx == nil || ctx.Err() != nil {
		return empty, ErrNextIdentityUnavailable
	}
	selection, err := SelectNextConnectionRuntime(ctx, root)
	if err != nil || selection.Running == nil || !*selection.Running {
		return empty, ErrNextIdentityUnavailable
	}
	pid, instance, binary := selection.PortRuntime.PID, selection.PortRuntime.ProcessInstance, selection.PortRuntime.Binary
	if pid <= 0 || instance == "" || binary == "" || verifyNextCoreProcess(pid, instance, binary) != nil {
		return empty, ErrNextIdentityUnavailable
	}
	var result NativeCodexReadiness
	if err := desktopcontrol.CallVerifiedPID(ctx, root, pid, "next_codex.readiness", nil, &result); err != nil {
		return empty, ErrNextIdentityUnavailable
	}
	if verifyNextCoreProcess(pid, instance, binary) != nil || !result.OK || !ValidNativeCodexState(result.State) {
		return empty, ErrNextIdentityUnavailable
	}
	return result, nil
}
