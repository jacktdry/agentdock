//go:build darwin || linux

package desktopruntime

import (
	"errors"
	"os"
	"syscall"
)

func validateTunnelStateRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return errors.New("tunnel_state_root_unsafe")
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Getuid()) {
		return errors.New("tunnel_state_root_unsafe")
	}
	return nil
}
