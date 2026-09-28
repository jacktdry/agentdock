package desktopruntime

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// ownedTailscalePID reads only AgentDock's recorded child. An unreadable process
// is not assumed stale: access errors must not make us lose ownership evidence.
func ownedTailscalePID(pidPath, binaryPath string, inspect func(uint32) (string, error)) (uint32, error) {
	data, err := os.ReadFile(pidPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	text := strings.TrimSpace(string(data))
	parsed, err := strconv.ParseUint(text, 10, 32)
	if err != nil || parsed == 0 {
		return 0, removeRecordedTailscalePID(pidPath, text)
	}
	pid := uint32(parsed)
	actual, err := inspect(pid)
	if errors.Is(err, os.ErrNotExist) || (err == nil && !samePath(actual, binaryPath)) {
		return 0, removeRecordedTailscalePID(pidPath, text)
	}
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func stopOwnedTailscalePID(pidPath, binaryPath string, inspect func(uint32) (string, error), terminate func(uint32, string) error) error {
	pid, err := ownedTailscalePID(pidPath, binaryPath, inspect)
	if err != nil || pid == 0 {
		return err
	}
	err = terminate(pid, binaryPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return removeRecordedTailscalePID(pidPath, strconv.FormatUint(uint64(pid), 10))
}

func removeRecordedTailscalePID(path, expected string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(data)) != expected {
		return nil // A new supervisor has replaced the record.
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
