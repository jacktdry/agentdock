//go:build darwin

package commandpath

import (
	"os"
	"os/exec"
	"path/filepath"
)

func LookPath(name string) (string, error) {
	home, _ := os.UserHomeDir()
	return lookPathIn(name, Path(os.Getenv("PATH"), home))
}

func lookPathIn(name, path string) (string, error) {
	for _, dir := range filepath.SplitList(path) {
		candidate := filepath.Join(dir, name)
		if _, err := exec.LookPath(candidate); err != nil {
			continue
		}
		if !filepath.IsAbs(candidate) {
			return "", exec.ErrDot
		}
		if resolved, err := ValidateExecutable(candidate); err == nil {
			return resolved, nil
		}
	}
	return "", exec.ErrNotFound
}
