//go:build windows

package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func platformBasicAutostartAvailable() error { return nil }

func platformReadBasicSettings(ctx context.Context, root string) (BasicSettings, error) {
	manifest, root, err := loadDesktopManifest(root)
	if err != nil {
		return BasicSettings{}, err
	}
	settings, err := loadControlPanelSettings(root, manifest.Port)
	if err != nil {
		return BasicSettings{}, err
	}
	startup, err := coreAutostartEnabled(ctx, manifest)
	result := BasicSettings{Port: settings.Port, LogLevel: settings.LogLevel, CoreAutostart: startup}
	if err == nil {
		err = validateBasicSettings(result)
	}
	return result, err
}

func patchBasicSettingsJSON(data []byte, settings BasicSettings) ([]byte, error) {
	fields := map[string]json.RawMessage{}
	if len(data) != 0 {
		if err := json.Unmarshal(data, &fields); err != nil {
			return nil, err
		}
		if fields == nil {
			return nil, ErrBasicSettingsInvalid
		}
	}
	fields["port"], _ = json.Marshal(settings.Port)
	fields["log_level"], _ = json.Marshal(settings.LogLevel)
	return json.MarshalIndent(fields, "", "  ")
}

func platformUpdateBasicSettings(ctx context.Context, root string, requested BasicSettings) error {
	runtime, err := loadTunnelRuntime(root)
	if err != nil {
		return err
	}
	path := filepath.Join(runtime.root, "control-panel-settings.json")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	updated, err := patchBasicSettingsJSON(data, requested)
	if err != nil {
		return err
	}
	settings := runtime.settings
	settings.Port, settings.LogLevel = requested.Port, requested.LogLevel
	coreRunning, err := basicWindowsCoreRunning(ctx, runtime)
	if err != nil {
		return err
	}
	tunnelRunning, err := basicWindowsTunnelRunning(runtime)
	if err != nil {
		return err
	}
	prior := &controlPanelRunState{coreRunning, tunnelRunning}
	return applyControlPanelSettingsWithState(ctx, runtime, settings, append(updated, '\n'), prior, nativeControlPanelActions())
}

func basicWindowsTunnelRunning(runtime tunnelRuntime) (bool, error) {
	cloudflaredRunning, err := processRunningAtPath(runtime.manifest.CloudflaredBinary)
	if err != nil {
		return false, err
	}
	supervisorPID, err := activeTunnelSupervisorPIDForRuntime(runtime.root, runtime.manifest)
	if err != nil {
		return false, err
	}
	return managedTunnelRunning(cloudflaredRunning, supervisorPID), nil
}

func managedTunnelRunning(cloudflaredRunning bool, supervisorPID uint32) bool {
	return cloudflaredRunning || supervisorPID != 0
}

// The tunnel supervisor shares the Core executable but is not a running Core.
func basicWindowsCoreRunning(ctx context.Context, runtime tunnelRuntime) (bool, error) {
	binary := ActiveCoreBinary(runtime.root, runtime.manifest)
	excluded, err := ancestorProcessIDsAtPath(binary)
	if err != nil {
		return false, err
	}
	pid, err := activeTunnelSupervisorPIDForRuntime(runtime.root, runtime.manifest)
	if err != nil {
		return false, err
	}
	if pid != 0 {
		excluded[pid] = struct{}{}
	}
	pids, err := processIDsAtPathExcept(binary, excluded)
	if err != nil {
		return false, err
	}
	return len(pids) > 0 || testHealth(ctx, runtime.manifest.HealthURL()), nil
}
