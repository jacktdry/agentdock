//go:build windows

package desktopapi

import (
	"errors"
	"os"

	"github.com/uvwt/agentdock/internal/fs/securepath"
)

// Windows permission bits do not attest ACL ownership. Secure the directory
// before the token is created so no wider inherited ACE can see the token.
func secureConnectionTokenRoot(root string, info os.FileInfo) error {
	if !info.IsDir() {
		return errors.New("unsafe token root")
	}
	return securepath.EnsurePrivate(root)
}

func secureConnectionTokenFile(file *os.File) error {
	if err := securepath.EnsurePrivate(file.Name()); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("unsafe token file")
	}
	return nil
}
