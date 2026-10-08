//go:build !darwin

package desktopruntime

import "context"

func ReadVerifiedNextNexusRuntimeStatus(context.Context, string) (ServiceStatus, error) {
	return ServiceStatus{}, ErrNextIdentityUnavailable
}
