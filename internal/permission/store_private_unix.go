//go:build !windows

package permission

import (
	"fmt"
	"os"
)

func verifyPrivateStatePath(_ string, info os.FileInfo, directory bool) error {
	want := os.FileMode(0o600)
	if directory {
		want = 0o700
	}
	if info.Mode().Perm() != want {
		return fmt.Errorf("permission state mode %o, want %o", info.Mode().Perm(), want)
	}
	return nil
}
