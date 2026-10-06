//go:build !darwin && !linux

package desktopruntime

import "errors"

func platformTrustedAntigravityTarget(string, string) bool {
	return false
}

func platformPromoteACPUpdate(string, string, []byte) error {
	return errors.New("ACP adapter managed updates are unavailable on this platform")
}
