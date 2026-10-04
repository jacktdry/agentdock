//go:build darwin || linux

package desktopruntime

import (
	"context"
	"errors"
)

func platformReadACPSettings(_ context.Context, runtimeRoot string) (ACPSettings, error) {
	_, _, values, err := readBasicEnvironment(runtimeRoot)
	if err != nil {
		return ACPSettings{}, errors.New("protected ACP environment could not be read")
	}
	return acpSettingsFromEnvironment(values)
}
