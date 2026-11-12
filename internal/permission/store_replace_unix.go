//go:build !windows

package permission

import (
	"fmt"
	"os"
)

// The temp file has already been flushed. On Unix, syncing the containing
// directory after rename is required to persist the name replacement.
func replacePermissionState(tempPath, statePath, stateDir string) error {
	if err := os.Rename(tempPath, statePath); err != nil {
		return err
	}
	dir, err := os.Open(stateDir)
	if err != nil {
		return fmt.Errorf("open permissions directory for fsync: %w", err)
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return fmt.Errorf("fsync permissions directory: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close permissions directory: %w", closeErr)
	}
	return nil
}
