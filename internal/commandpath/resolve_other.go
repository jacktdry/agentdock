//go:build !darwin

package commandpath

import "os/exec"

// Preserve native Linux/Windows executable lookup semantics.
func LookPath(name string) (string, error) { return exec.LookPath(name) }
