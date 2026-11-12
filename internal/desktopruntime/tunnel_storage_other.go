//go:build !darwin && !linux && !windows

package desktopruntime

import "errors"

func validateTunnelStateRoot(string) error { return errors.New("tunnel_state_storage_unavailable") }
