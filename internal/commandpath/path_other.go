//go:build !darwin

package commandpath

func Path(currentPath, _ string) string { return currentPath }
