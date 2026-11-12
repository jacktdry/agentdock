//go:build !darwin

package desktopruntime

import (
	"context"
	"github.com/uvwt/agentdock/internal/browserdesktop"
)

// No equivalent verified Next Core peer authority is available on this platform.
func ReadVerifiedNextBrowserSnapshot(context.Context, string) (browserdesktop.Snapshot, error) {
	return browserdesktop.Snapshot{}, ErrNextIdentityUnavailable
}
