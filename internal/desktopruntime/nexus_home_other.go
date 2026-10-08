//go:build !darwin

package desktopruntime

import "context"

func ResolveNextAgentDockHome(context.Context, string) (string, error) {
	return "", ErrNextIdentityUnavailable
}

func ReadNextNexusIdentity(context.Context, string) ([]byte, error) {
	return nil, ErrNextIdentityUnavailable
}
