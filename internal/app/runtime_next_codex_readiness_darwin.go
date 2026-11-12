//go:build darwin

package app

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// NextCodexMCPReadiness is a Core-owned, redacted and read-only operator
// preflight. It is served only over the existing local native control socket;
// it never bootstraps an Adapter, unlocks a journal or grants authorization.
func (r *Runtime) NextCodexMCPReadiness() Result {
	state := r.nextCodexMCPReadinessState()
	if state == "operator_setup_required" {
		// codesign/plutil must not hold lifecycleMu: Close and private B6
		// registration need the writer lock. Never cache a successful preflight.
		_, err := selectNextCodexSignedLauncher()
		state = r.nextCodexMCPReadinessState()
		if state == "operator_setup_required" && err != nil {
			state = "launcher_unavailable"
		}
	}
	return Result{"ok": true, "state": state}
}

func (r *Runtime) nextCodexMCPReadinessState() string {
	state := "disabled"
	if r == nil || !nextCodexExclusiveAuthorityEnabled(r.cfg) {
		return state
	}
	r.lifecycleMu.RLock()
	defer r.lifecycleMu.RUnlock()
	switch {
	case r.closing:
		state = "core_closing"
	case r.nextPermissionOwner == nil || r.acp == nil || r.permissions == nil:
		state = "owner_unavailable"
	case r.nextCodexRegistry != nil:
		state = "registration_present"
	case r.nextCodexStartupLock != nil || r.nextCodexPin != nil:
		state = "startup_unresolved"
	default:
		path := filepath.Join(r.cfg.AgentDockHome, nextCodexStartupLockName)
		info, err := os.Lstat(path)
		var stat *syscall.Stat_t
		if err == nil {
			stat, _ = info.Sys().(*syscall.Stat_t)
		}
		switch {
		case errors.Is(err, os.ErrNotExist):
			state = "operator_setup_required"
		case err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || stat == nil || stat.Uid != uint32(os.Getuid()) || stat.Nlink != 1:
			state = "state_untrusted"
		case info.Size() != 0:
			// A nonempty journal is not proof that old native children exited.
			state = "recovery_required"
		default:
			state = "operator_setup_required"
		}
	}
	return state
}
