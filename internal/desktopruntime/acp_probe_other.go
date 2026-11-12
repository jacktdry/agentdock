//go:build !darwin && !linux && !windows

package desktopruntime

func platformResolveACPProbe(string, string, ACPProfileSettings) (string, []string, bool) {
	return "", nil, false
}
