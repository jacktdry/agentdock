//go:build windows

package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	agentconfig "github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/fs/atomicfile"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

type fileSnapshot struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

func snapshotFile(path string) (fileSnapshot, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{path: path}, nil
	}
	if err != nil {
		return fileSnapshot{}, err
	}
	if !info.Mode().IsRegular() {
		return fileSnapshot{}, fmt.Errorf("配置文件不是普通文件: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	return fileSnapshot{path: path, data: data, mode: info.Mode().Perm(), exists: true}, nil
}

func restoreSnapshots(snapshots []fileSnapshot) error {
	var restoreErr error
	for _, snapshot := range snapshots {
		if snapshot.exists {
			mode := snapshot.mode
			if mode == 0 {
				mode = 0o600
			}
			if err := atomicfile.Write(snapshot.path, snapshot.data, mode); err != nil {
				restoreErr = errors.Join(restoreErr, err)
			}
		} else if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			restoreErr = errors.Join(restoreErr, err)
		}
	}
	return restoreErr
}

func platformUpdateConfig(ctx context.Context, request ConfigUpdateRequest) error {
	if request.BrowserEnabled && request.BrowserCDPURL == "" && !request.BrowserReuseExistingCDP {
		if _, err := toolbrowser.FindExecutable("", toolbrowser.BrowserAuto); err != nil {
			return fmt.Errorf("未检测到受支持的 Chrome、Chromium 或 Microsoft Edge，且未配置外部 CDP: %w", err)
		}
	}
	runtime, err := loadTunnelRuntime(request.RuntimeRoot)
	if err != nil {
		return err
	}
	acpProfiles := append([]agentconfig.ACPProfile(nil), request.ACPProfiles...)
	if request.ACPEnabled {
		for index := range acpProfiles {
			profile := &acpProfiles[index]
			if !profile.Enabled {
				continue
			}
			adapter, resolveErr := resolveDesktopACPAdapter(profile.Kind, runtime.root, profile.Command, profile.Args)
			if resolveErr != nil {
				return fmt.Errorf("解析 ACP Profile %s Adapter 失败: %w", profile.ID, resolveErr)
			}
			profile.Command = adapter.Command
			profile.Args = append([]string(nil), adapter.Args...)
		}
	}
	acpDefaultProfile := request.ACPDefaultProfile
	if acpDefaultProfile == "" {
		for _, profile := range acpProfiles {
			if profile.Enabled {
				acpDefaultProfile = profile.ID
				break
			}
		}
	}

	settings := controlPanelSettings{
		Port:                    request.Port,
		LogLevel:                request.LogLevel,
		OAuthAccessTokenTTL:     request.OAuthAccessTokenTTL,
		MCPAppsMode:             request.MCPAppsMode,
		BrowserEnabled:          request.BrowserEnabled,
		BrowserCDPURL:           request.BrowserCDPURL,
		BrowserReuseExistingCDP: request.BrowserReuseExistingCDP,
		ACPEnabled:              request.ACPEnabled,
		ACPProfiles:             acpProfiles,
		ACPDefaultProfile:       acpDefaultProfile,
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return applyControlPanelSettings(ctx, runtime, settings, data)
}

// Shared by native full configuration and the basic-settings patch.
func applyControlPanelSettings(ctx context.Context, runtime tunnelRuntime, settings controlPanelSettings, data []byte) error {
	return applyControlPanelSettingsWithState(ctx, runtime, settings, data, nil, nativeControlPanelActions())
}

type controlPanelRunState struct{ coreRunning, tunnelRunning bool }

func controlPanelState(mode string, prior *controlPanelRunState) controlPanelRunState {
	if prior != nil {
		return *prior
	}
	// Full ConfigUpdate retains its legacy always-start policy.
	return controlPanelRunState{true, mode != "none"}
}

type controlPanelActions struct {
	stopTunnel, startTunnel func(context.Context, tunnelRuntime) error
	coreAction              func(context.Context, string, string) error
}

func nativeControlPanelActions() controlPanelActions {
	return controlPanelActions{stopTunnel, startTunnel, platformServiceAction}
}

func applyControlPanelSettingsWithState(ctx context.Context, runtime tunnelRuntime, settings controlPanelSettings, data []byte, prior *controlPanelRunState, actions controlPanelActions) error {
	state := controlPanelState(runtime.mode, prior)
	runtime.preserveStoppedCore = prior != nil
	var err error
	settingsPath := filepath.Join(runtime.root, "control-panel-settings.json")
	snapshotPaths := []string{
		settingsPath,
		runtime.files.manifest,
		runtime.files.serverURL,
		runtime.files.quickURL,
	}
	snapshots := make([]fileSnapshot, 0, len(snapshotPaths))
	for _, path := range snapshotPaths {
		snapshot, snapshotErr := snapshotFile(path)
		if snapshotErr != nil {
			return fmt.Errorf("备份配置失败: %w", snapshotErr)
		}
		snapshots = append(snapshots, snapshot)
	}

	rollback := func(cause error) error {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), basicRecoveryTimeout)
		defer cancel()
		var restoreErr error
		if prior != nil && state.tunnelRunning {
			// Stop a partially started supervisor before restoring files it writes.
			restoreErr = actions.stopTunnel(recovery, runtime)
		}
		restoreErr = errors.Join(restoreErr, restoreSnapshots(snapshots))
		oldRuntime, loadErr := loadTunnelRuntime(runtime.root)
		if loadErr == nil {
			oldRuntime.preserveStoppedCore = prior != nil
			if state.coreRunning {
				restoreErr = errors.Join(restoreErr, actions.coreAction(recovery, oldRuntime.root, "restart"))
			}
			if state.tunnelRunning {
				restoreErr = errors.Join(restoreErr, actions.startTunnel(recovery, oldRuntime))
			}
		}
		restoreErr = errors.Join(restoreErr, loadErr)
		if restoreErr != nil {
			return fmt.Errorf("%w；同时恢复配置失败: %v", cause, restoreErr)
		}
		return cause
	}
	if prior == nil || state.tunnelRunning {
		if err := actions.stopTunnel(ctx, runtime); err != nil {
			if prior != nil {
				return rollback(err)
			}
			return err
		}
	}

	if err := atomicfile.Write(settingsPath, data, 0o600); err != nil {
		return rollback(err)
	}

	runtime.settings = settings
	publicURL := ""
	manifestMode := runtime.mode
	switch runtime.mode {
	case "quick":
		if err := clearActivePublicURL(runtime.files); err != nil {
			return rollback(err)
		}
		manifestMode = "none"
	case "named":
		publicURL, err = readTrimmedText(runtime.files.namedServerURL)
		if err != nil {
			return rollback(err)
		}
	}
	if err := runtime.updateManifest(manifestMode, publicURL); err != nil {
		return rollback(err)
	}
	if state.coreRunning {
		if err := actions.coreAction(ctx, runtime.root, "restart"); err != nil {
			return rollback(err)
		}
	}
	if state.tunnelRunning {
		if err := actions.startTunnel(ctx, runtime); err != nil {
			return rollback(err)
		}
	}
	return nil
}
