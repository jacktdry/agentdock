//go:build !darwin

package desktopruntime

import "context"

// No trusted Next desktop/runtime pairing is supported here yet.
func ReadVerifiedNextNativeCodexReadiness(context.Context, string) (NativeCodexReadiness, error) {
	return NativeCodexReadiness{}, ErrNextIdentityUnavailable
}
