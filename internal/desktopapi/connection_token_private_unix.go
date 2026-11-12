//go:build !windows

package desktopapi

import (
	"errors"
	"os"
)

func secureConnectionTokenRoot(_ string, info os.FileInfo) error {
	if !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return errors.New("unsafe token root")
	}
	return nil
}

func secureConnectionTokenFile(file *os.File) error {
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return errors.New("unsafe token file")
	}
	return nil
}
