//go:build darwin || linux

package desktopruntime

import "context"

func platformReadACPSettings(_ context.Context, runtimeRoot string) (ACPSettings, error) {
	_, _, values, err := loadCoreEnvironment(runtimeRoot)
	if err != nil {
		return ACPSettings{}, err
	}
	return acpSettingsFromEnvironment(values)
}
