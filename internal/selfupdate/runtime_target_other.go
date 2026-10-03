//go:build !darwin && !windows

package selfupdate

func coreBinaryFallbackForRuntime(string) (string, bool) {
	return "", false
}

func detectDesktopUpdateTargetForRuntime(string, string) string {
	return ""
}
