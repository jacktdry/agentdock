//go:build !darwin && !linux

package desktopruntime

import (
	"context"
	"errors"
)

// Windows DPAPI can be read without creating secrets, but the current manifest
// has no Next identity contract. Fail closed until a Next-owned adapter exists.
func platformReadConnectionConfig(context.Context, string) (ConnectionConfig, error) {
	return ConnectionConfig{}, errors.New("Next connection adapter unavailable")
}
func platformReadOAuthPassword(string) (string, OAuthPasswordState) {
	return "", OAuthPasswordUnavailable
}

func platformReadOAuthPasswordState(string) OAuthPasswordState {
	return OAuthPasswordUnavailable
}
