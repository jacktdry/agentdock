//go:build !darwin && !windows

package desktopruntime

import "context"

func platformPortOwner(context.Context, PortObservationRequest) (PortState, string) {
	return PortUnknown, "platform_ownership_unsupported"
}

func platformPortProcess(int) (string, string, error) {
	return "", "", ErrPortPreflight
}
