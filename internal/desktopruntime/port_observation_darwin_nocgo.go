//go:build darwin && !cgo

package desktopruntime

func platformPortProcess(int) (string, string, error) {
	return "", "", ErrPortPreflight
}
