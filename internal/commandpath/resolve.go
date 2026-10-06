package commandpath

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// ValidateExecutable accepts absolute paths, including symlinks to regular executables.
// Errors deliberately omit configured values.
func ValidateExecutable(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("CODEX_PATH must be absolute")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("CODEX_PATH must resolve to a regular executable file")
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", errors.New("CODEX_PATH must resolve to a regular executable file")
	}
	return resolved, nil
}
