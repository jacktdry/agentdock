//go:build !darwin && !linux && !windows

package desktopruntime

func platformReadCoreConnection(string) (CoreConnection, error) {
	return CoreConnection{}, ErrCoreConnectionUnavailable
}
