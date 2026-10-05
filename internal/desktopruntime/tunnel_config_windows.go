//go:build windows

package desktopruntime

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
)

func platformConfigureTunnel(ctx context.Context, request TunnelConfigureRequest) error {
	runtime, err := loadTunnelRuntime(request.RuntimeRoot)
	if err != nil {
		return err
	}
	if request.Mode != "none" && request.Mode != "quick" && request.Mode != "named" {
		return errors.New("invalid_tunnel_mode")
	}
	if request.Mode != "named" && (request.ServerURL != "" || request.TokenFile != "") {
		return errors.New("named_fields_in_other_mode")
	}
	origin, token := "", ""
	if request.Mode == "named" {
		// Current Windows installation default. Custom ports require an external route
		// update and cannot be synchronized by the remotely-managed token runner.
		if runtime.settings.Port != 8765 {
			return ErrNamedManualRouteRequired
		}
		candidate := strings.TrimSpace(request.ServerURL)
		if candidate == "" {
			candidate, err = readTrimmedText(runtime.files.namedServerURL)
			if err != nil {
				return err
			}
		}
		origin, err = normalizeHTTPSOrigin(candidate)
		if err != nil {
			return errors.New("invalid_named_origin")
		}
		token, err = readSecretFile(request.TokenFile)
		if err != nil {
			return errors.New("tunnel_token_unreadable")
		}
		if token == "" {
			token, err = readProtectedText(runtime.files.token, tunnelTokenEntropy)
			if err != nil {
				return errors.New("tunnel_token_unreadable")
			}
		}
		if token == "" || len(token) > 16*1024 || strings.ContainsAny(token, "\r\n\x00") {
			return errors.New("tunnel_token_invalid")
		}
	}
	snapshots, err := captureTunnelFiles(runtime.files.manifest, runtime.files.mode, runtime.files.serverURL, runtime.files.namedServerURL, runtime.files.token,
		filepath.Join(runtime.root, "auth-token.dpapi"), filepath.Join(runtime.root, "oauth-password.dpapi"), filepath.Join(runtime.root, "oauth-token-secret.dpapi"), filepath.Join(runtime.root, credentialOwnerSIDFile))
	if err != nil {
		return err
	}
	coreRunning, err := basicWindowsCoreRunning(ctx, runtime)
	if err != nil {
		return err
	}
	tunnelRunning, err := processRunningAtPath(runtime.manifest.CloudflaredBinary)
	if err != nil {
		return err
	}
	runtime.preserveStoppedCore = true
	oldMode := runtime.mode
	return runTunnelTransactionLocked(ctx, runtime.root, snapshots, func(ctx context.Context) error {
		if _, err := AdvanceTunnelGenerationLocked(runtime.root); err != nil {
			return err
		}
		if tunnelRunning {
			if err := stopTunnel(ctx, runtime); err != nil {
				return err
			}
		}
		if err := ensureDesktopCredentials(runtime.root); err != nil {
			return errors.New("desktop_credentials_unavailable")
		}
		if err := preserveNamedServerURL(runtime); err != nil {
			return err
		}
		if err := clearActivePublicURL(runtime.files); err != nil {
			return err
		}
		if request.Mode == "named" {
			if err := writeProtectedText(runtime.files.token, token, tunnelTokenEntropy); err != nil {
				return errors.New("tunnel_token_write_failed")
			}
			if err := writeRuntimeText(runtime.files.namedServerURL, origin); err != nil {
				return err
			}
			if err := writeRuntimeText(runtime.files.serverURL, origin); err != nil {
				return err
			}
		}
		if err := writeRuntimeText(runtime.files.mode, request.Mode); err != nil {
			return err
		}
		if err := runtime.updateManifest(request.Mode, origin); err != nil {
			return err
		}
		if coreRunning {
			if err := platformServiceAction(ctx, runtime.root, "restart"); err != nil {
				return err
			}
		}
		if tunnelRunning && request.Mode != "none" {
			runtime.mode = request.Mode
			return startTunnelLocal(ctx, runtime)
		}
		return nil
	}, func(ctx context.Context) error {
		if _, err := AdvanceTunnelGenerationLocked(runtime.root); err != nil {
			return err
		}
		if oldMode == "quick" {
			if err := clearActivePublicURL(runtime.files); err != nil {
				return err
			}
			if err := runtime.updateManifest("quick", ""); err != nil {
				return err
			}
		}
		if coreRunning {
			if err := platformServiceAction(ctx, runtime.root, "restart"); err != nil {
				return err
			}
		}
		current, err := loadTunnelRuntime(runtime.root)
		if err != nil {
			return err
		}
		current.preserveStoppedCore = true
		if tunnelRunning {
			return startTunnelLocal(ctx, current)
		}
		return stopTunnel(ctx, current)
	})
}
