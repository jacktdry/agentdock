//go:build windows

package permission

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// FlushFileBuffers on a Windows directory handle (os.File.Sync) returns
// ERROR_ACCESS_DENIED. Use a same-directory move with write-through instead;
// never use COPY_ALLOWED or a non-atomic copy/delete fallback. Windows cannot
// promise the same crash/power-loss semantics as a Unix directory fsync on
// every backing filesystem, so native Windows durability testing remains a gate.
func replacePermissionState(tempPath, statePath, stateDir string) error {
	if filepath.Clean(filepath.Dir(tempPath)) != filepath.Clean(stateDir) || filepath.Clean(filepath.Dir(statePath)) != filepath.Clean(stateDir) {
		return fmt.Errorf("permission state replacement must stay within one directory")
	}
	source, err := windows.UTF16PtrFromString(tempPath)
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(statePath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(source, target, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
