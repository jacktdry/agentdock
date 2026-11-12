//go:build !darwin

package desktopruntime

import "context"

// Windows Manifest has no variant authority; Linux has no selected Next adapter.
func platformSelectNextConnectionRuntime(context.Context, string) (NextConnectionRuntime, error) {
	return NextConnectionRuntime{}, ErrNextIdentityUnavailable
}

func platformValidateNextSettingsIdentity(context.Context, string) error {
	return ErrNextIdentityUnavailable
}
