//go:build windows

package desktopruntime

import (
	"errors"
	"github.com/uvwt/agentdock/internal/fs/securepath"
	"os"
)

func validateTunnelStateRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("tunnel_state_root_unsafe")
	}
	return securepath.EnsurePrivate(root)
}
