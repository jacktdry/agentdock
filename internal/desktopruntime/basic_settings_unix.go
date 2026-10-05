//go:build darwin || linux

package desktopruntime

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/uvwt/agentdock/internal/envstore"
	"github.com/uvwt/agentdock/internal/fs/atomicfile"
)

func platformBasicAutostartAvailable() error {
	if runtime.GOOS == "darwin" {
		return ErrBasicSettingsUnavailable
	}
	return nil
}

func readBasicEnvironment(root string) (string, []byte, map[string]string, error) {
	manifest, _, err := loadUnixRuntime(root)
	if err != nil {
		return "", nil, nil, err
	}
	path := manifest.EnvironmentFile
	info, err := os.Lstat(path)
	if err != nil {
		return "", nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", nil, nil, errors.New("configuration must be a private regular file")
	}
	if runtime.GOOS == "darwin" {
		owner, ok := info.Sys().(*syscall.Stat_t)
		if !ok || owner.Uid != uint32(os.Getuid()) {
			return "", nil, nil, errors.New("configuration must belong to the current user")
		}
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return "", nil, nil, err
	}
	values, err := envstore.Parse(original)
	return path, original, values, err
}

func platformReadBasicSettings(ctx context.Context, root string) (BasicSettings, error) {
	_, _, values, err := readBasicEnvironment(root)
	if err != nil {
		return BasicSettings{}, err
	}
	port := unixDefaultPort()
	if raw := values["AGENTDOCK_PORT"]; raw != "" {
		port, err = strconv.Atoi(raw)
		if err != nil {
			return BasicSettings{}, ErrBasicSettingsInvalid
		}
	}
	level := values["AGENTDOCK_LOG_LEVEL"]
	if level == "" {
		level = "info"
	}
	settings := BasicSettings{Port: port, LogLevel: level}
	if err := validateBasicSettings(settings); err != nil {
		return BasicSettings{}, err
	}
	status, err := platformServiceStatus(ctx, root)
	settings.CoreAutostart = status.StartupEnabled
	return settings, err
}

func platformUpdateBasicSettings(ctx context.Context, root string, settings BasicSettings) error {
	path, original, values, err := readBasicEnvironment(root)
	if err != nil {
		return err
	}
	manifest, _, err := loadUnixRuntime(root)
	if err != nil {
		return err
	}
	running := basicUnixServiceRunning(ctx, manifest, false)
	oldOrigin := strings.TrimSuffix(healthURL(values), "/healthz")
	values["AGENTDOCK_PORT"] = strconv.Itoa(settings.Port)
	values["AGENTDOCK_LOG_LEVEL"] = settings.LogLevel
	changes := []basicEnvironmentChange{{path, original, envstore.Marshal(values)}}
	var restartTunnel func(context.Context) error
	newOrigin := strings.TrimSuffix(healthURL(values), "/healthz")
	if oldOrigin != newOrigin {
		change, err := basicQuickTunnelChange(manifest.TunnelEnvironment, newOrigin)
		if err != nil {
			return err
		}
		if change != nil {
			changes = append(changes, *change)
			if basicUnixServiceRunning(ctx, manifest, true) {
				restartTunnel = func(ctx context.Context) error { return tunnelServiceAction(ctx, manifest, "restart") }
			}
		}
	}
	var restartCore func(context.Context) error
	if running {
		restartCore = func(ctx context.Context) error { return platformServiceAction(ctx, root, "restart") }
	}
	if os.Getenv("AGENTDOCK_DESKTOP_VARIANT") == "next" && oldOrigin != newOrigin {
		snapshots, err := captureTunnelFiles(manifest.EnvironmentFile, manifest.TunnelEnvironment)
		if err != nil {
			return err
		}
		// Invalidate the old public address in the same bounded transaction.
		// The launcher callback reacquires Desktop mutation only after we return.
		for i := range changes {
			if changes[i].path == manifest.EnvironmentFile {
				updated, err := envstore.Parse(changes[i].updated)
				if err != nil {
					return err
				}
				if len(changes) > 1 {
					delete(updated, "AGENTDOCK_SERVER_URL")
					updated["AGENTDOCK_OAUTH_ENABLED"] = "false"
				}
				changes[i].updated = envstore.Marshal(updated)
			}
		}
		return runTunnelTransactionLocked(ctx, root, snapshots, func(ctx context.Context) error {
			if _, err := AdvanceTunnelGenerationLocked(root); err != nil {
				return err
			}
			if len(changes) > 1 {
				if err := removeQuickURL(root); err != nil {
					return err
				}
			}
			return replaceBasicEnvironments(ctx, changes, restartCore, restartTunnel)
		}, func(ctx context.Context) error {
			if _, err := AdvanceTunnelGenerationLocked(root); err != nil {
				return err
			}
			if len(changes) > 1 {
				if err := invalidateQuickTunnelUnixLocked(root); err != nil {
					return err
				}
			}
			for _, restart := range []func(context.Context) error{restartCore, restartTunnel} {
				if restart != nil {
					if err := restart(ctx); err != nil {
						return err
					}
				}
			}
			return nil
		})
	}
	return replaceBasicEnvironments(ctx, changes, restartCore, restartTunnel)
}

type basicEnvironmentChange struct {
	path              string
	original, updated []byte
}

func basicQuickTunnelChange(path, origin string) (*basicEnvironmentChange, error) {
	original, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	values, err := envstore.Parse(original)
	if err != nil {
		return nil, err
	}
	if tunnelMode(values) != "quick" {
		return nil, nil
	}
	values["AGENTDOCK_TUNNEL_TARGET"] = origin
	return &basicEnvironmentChange{path, original, envstore.Marshal(values)}, nil
}

// All environments are installed before any restart; recovery is independent of
// the caller and restores exact bytes before restarting previously active jobs.
func replaceBasicEnvironments(ctx context.Context, changes []basicEnvironmentChange, restartCore, restartTunnel func(context.Context) error) error {
	written := 0
	restarted := false
	rollback := func(cause error) error {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), basicRecoveryTimeout)
		defer cancel()
		var restoreErr error
		for _, change := range changes[:written] {
			restoreErr = errors.Join(restoreErr, atomicfile.Write(change.path, change.original, 0o600))
		}
		if restoreErr == nil && restarted {
			if restartCore != nil {
				restoreErr = errors.Join(restoreErr, restartCore(recovery))
			}
			if restartTunnel != nil {
				restoreErr = errors.Join(restoreErr, restartTunnel(recovery))
			}
		}
		return errors.Join(cause, restoreErr)
	}
	for _, change := range changes {
		if err := atomicfile.Write(change.path, change.updated, 0o600); err != nil {
			return rollback(err)
		}
		written++
	}
	for _, restart := range []func(context.Context) error{restartCore, restartTunnel} {
		if restart == nil {
			continue
		}
		restarted = true
		if err := restart(ctx); err != nil {
			return rollback(err)
		}
	}
	return nil
}
