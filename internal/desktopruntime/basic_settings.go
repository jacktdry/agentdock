package desktopruntime

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrBasicSettingsInvalid = errors.New("invalid basic settings")
var ErrBasicSettingsUnavailable = errors.New("basic settings operation requires the native desktop adapter")

// Recovery may need to restore an active Windows Quick Tunnel (up to 145s)
// plus a core restart. Keep the recovery budget above the longest supported
// composed desktop mutation rather than truncating rollback mid-flight.
const basicRecoveryTimeout = 5 * time.Minute

var basicSettingsMu sync.Mutex

// BasicSettings contains no advanced configuration or credentials.
type BasicSettings struct {
	Port          int    `json:"port"`
	LogLevel      string `json:"logLevel"`
	CoreAutostart bool   `json:"coreAutostart"`
}

func validateBasicSettings(settings BasicSettings) error {
	if settings.Port < 1 || settings.Port > 65535 {
		return ErrBasicSettingsInvalid
	}
	switch settings.LogLevel {
	case "debug", "info", "warn", "error":
		return nil
	}
	return ErrBasicSettingsInvalid
}

func ReadBasicSettings(ctx context.Context, root string) (BasicSettings, error) {
	basicSettingsMu.Lock()
	defer basicSettingsMu.Unlock()
	return platformReadBasicSettings(ctx, root)
}

// BasicAutostartMutable reports whether the shared settings surface may mutate
// core autostart on this platform. macOS keeps SMAppService registration native.
func BasicAutostartMutable() bool {
	return platformBasicAutostartAvailable() == nil
}

func UpdateBasicSettings(ctx context.Context, root string, requested BasicSettings) error {
	if err := validateBasicSettings(requested); err != nil {
		return err
	}
	basicSettingsMu.Lock()
	defer basicSettingsMu.Unlock()
	old, err := platformReadBasicSettings(ctx, root)
	if err != nil {
		return err
	}
	if old == requested {
		return nil
	}
	if old.CoreAutostart != requested.CoreAutostart {
		if err := platformBasicAutostartAvailable(); err != nil {
			return err
		}
	}
	// Use the existing service adapter; never implement OS registration here.
	if old.CoreAutostart != requested.CoreAutostart {
		if err := platformSetAutostart(ctx, root, "core", requested.CoreAutostart); err != nil {
			return err
		}
	}
	if old.Port != requested.Port || old.LogLevel != requested.LogLevel {
		if err := platformUpdateBasicSettings(ctx, root, requested); err != nil {
			if old.CoreAutostart != requested.CoreAutostart {
				recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), basicRecoveryTimeout)
				defer cancel()
				err = errors.Join(err, platformSetAutostart(recovery, root, "core", old.CoreAutostart))
			}
			return err
		}
	}
	return nil
}
