//go:build darwin

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/uvwt/agentdock/internal/updateidentity"
)

func validateUpdateCaller(executable string) error {
	id, err := currentMacOSUpdateIdentity()
	if err != nil {
		return err
	}
	for dir := filepath.Dir(executable); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if filepath.Ext(dir) == ".app" {
			if err := id.ValidateBundle(context.Background(), dir); err != nil {
				return err
			}
			if id.Variant == "next" {
				if err := id.ValidateSignatures(context.Background(), dir); err != nil {
					return err
				}
			}
			if !desktopUpdateOwnsExecutable(dir, executable) {
				return errors.New("updater caller is not the bundle Core")
			}
			return nil
		}
	}
	if id.Variant == "next" {
		return errors.New("Next updater requires an explicit bundle Core")
	}
	return nil
}

func validateUpdateRequest(request applyRequest) error {
	id, err := currentMacOSUpdateIdentity()
	if err != nil {
		return err
	}
	if id.Variant != "next" {
		return nil
	}
	if !request.DesktopOnly {
		return errors.New("Next standalone Core replacement is disabled")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := id.ValidateDestination(request.DesktopTargetPath, home); err != nil {
		return err
	}
	if !desktopUpdateOwnsExecutable(request.DesktopTargetPath, request.CurrentPath) {
		return errors.New("Next Core ownership mismatch")
	}
	return updateidentity.SafePath(request.DesktopTargetPath)
}

func updateHelperEnvironment() ([]string, error) {
	id, err := currentMacOSUpdateIdentity()
	if err != nil {
		return nil, err
	}
	env := os.Environ()
	if id.Variant == "next" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		state := filepath.Join(home, ".agentdock-next")
		work := filepath.Join(home, "AgentDock Next")
		for _, path := range []string{state, work} {
			if err := updateidentity.SafePath(path); err != nil {
				return nil, err
			}
		}
		env = environmentWithOverride(env, "AGENTDOCK_HOME", state)
		env = environmentWithOverride(env, "AGENTDOCK_DEFAULT_DIR", work)
	}
	return env, nil
}
