//go:build !darwin && !linux && !windows

package desktopruntime

import (
	"context"
	"errors"
)

func platformReadACPSettings(context.Context, string) (ACPSettings, error) {
	return ACPSettings{}, errors.New("ACP settings unavailable on this platform")
}

func platformLoadACPConfiguration(context.Context, string) (acpConfigurationDocument, error) {
	return acpConfigurationDocument{}, errors.New("ACP settings unavailable on this platform")
}
