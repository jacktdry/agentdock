//go:build !darwin && !linux && !windows

package desktopruntime

import (
	"context"
	"errors"
)

func platformReadLocalCoreAccess(context.Context, string) (LocalCoreAccess, error) {
	return LocalCoreAccess{}, errors.New("local AgentDock Core access is unavailable on this platform")
}
