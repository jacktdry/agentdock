//go:build darwin

package desktopruntime

import (
	"context"
	"errors"

	"github.com/uvwt/agentdock/internal/browserdesktop"
	"github.com/uvwt/agentdock/internal/desktopcontrol"
)

// ReadVerifiedNextBrowserSnapshot reads only the trusted Next launchd Core via
// a Unix socket whose peer PID and UID are verified before data is exchanged.
// No Core bearer token is read or sent to a TCP listener.
func ReadVerifiedNextBrowserSnapshot(ctx context.Context, root string) (browserdesktop.Snapshot, error) {
	empty := browserdesktop.Snapshot{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	selection, err := SelectNextConnectionRuntime(ctx, root)
	if err != nil || selection.Running == nil || !*selection.Running {
		return empty, ErrNextIdentityUnavailable
	}
	pid := selection.PortRuntime.PID
	instance := selection.PortRuntime.ProcessInstance
	binary := selection.PortRuntime.Binary
	if pid <= 0 || instance == "" || binary == "" {
		return empty, ErrNextIdentityUnavailable
	}
	if err := verifyNextCoreProcess(pid, instance, binary); err != nil {
		return empty, ErrNextIdentityUnavailable
	}
	var response struct {
		OK       bool                    `json:"ok"`
		Snapshot browserdesktop.Snapshot `json:"snapshot"`
	}
	if err := desktopcontrol.CallVerifiedPID(ctx, root, pid, "browser.snapshot", nil, &response); err != nil {
		return empty, ErrNextIdentityUnavailable
	}
	if err := verifyNextCoreProcess(pid, instance, binary); err != nil {
		return empty, ErrNextIdentityUnavailable
	}
	if !response.OK {
		return empty, errors.New("browser snapshot unavailable")
	}
	return response.Snapshot, nil
}
