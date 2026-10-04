//go:build windows

package desktopruntime

import "context"

func platformReadACPSettings(_ context.Context, runtimeRoot string) (ACPSettings, error) {
	manifest, root, err := loadDesktopManifest(runtimeRoot)
	if err != nil {
		return ACPSettings{}, err
	}
	settings, err := loadControlPanelSettings(root, manifest.Port)
	if err != nil {
		return ACPSettings{}, err
	}
	return acpSettingsFromProfiles(settings.ACPEnabled, settings.ACPProfiles, settings.ACPDefaultProfile), nil
}
