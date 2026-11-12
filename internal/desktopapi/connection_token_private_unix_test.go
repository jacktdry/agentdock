//go:build !windows

package desktopapi

import "os"

func testConnectionTokenPrivate(_ string, info os.FileInfo) bool {
	return info.Mode().Perm() == 0o600
}
