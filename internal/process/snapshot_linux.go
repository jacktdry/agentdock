//go:build linux

package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// procfs RSS is an OS estimate, not an exact physical-memory measurement.
// Unreadable entries invalidate the collection rather than undercounting a tree.
func observeProcesses(ctx context.Context) ([]observedProcess, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var rows []observedProcess
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 || !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		} // exited during collection
		if err != nil {
			return nil, fmt.Errorf("read proc PID %d: %w", pid, err)
		}
		row, err := parseProcStat(string(data), uint64(os.Getpagesize()))
		if err != nil {
			return nil, fmt.Errorf("parse proc PID %d: %w", pid, err)
		}
		if row.pid != pid {
			return nil, fmt.Errorf("proc PID mismatch")
		}
		rows = append(rows, row)
	}
	return rows, nil
}
