//go:build !darwin && !linux && !windows

package desktopruntime

import "context"

func platformBasicAutostartAvailable() error { return ErrBasicSettingsUnavailable }
func platformReadBasicSettings(context.Context, string) (BasicSettings, error) {
	return BasicSettings{}, ErrBasicSettingsUnavailable
}
func platformUpdateBasicSettings(context.Context, string, BasicSettings) error {
	return ErrBasicSettingsUnavailable
}
