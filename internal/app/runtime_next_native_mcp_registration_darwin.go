//go:build darwin

package app

import (
	"context"

	"github.com/uvwt/agentdock/internal/config"
)

// CORE-PRIVATE, source-only. A future trusted operator supplies only serverID;
// executable/argv/digest always come from Core's sealed catalog. No IPC/HTTP/CLI
// route or identity/approval is created here. Existing M8 authority is required.
func (r *Runtime) prepareNextNativeMCPRegistry(ctx context.Context, profile config.ACPProfile, keyHex, serverID string) (string, error) {
	return r.prepareNextNativeMCPRegistryResolved(ctx, profile, keyHex, serverID, resolveNextNativeMCPServer)
}

func (r *Runtime) attachNextNativeMCP(ctx context.Context, serverID string) error {
	return r.attachNextNativeMCPResolved(ctx, serverID, resolveNextNativeMCPServer)
}

// The resolver argument is a local test seam for the existing signed temporary
// bundle fixture, never a Runtime field, environment override or external input.
func (r *Runtime) prepareNextNativeMCPRegistryResolved(ctx context.Context, profile config.ACPProfile, keyHex, serverID string, resolve func(string) (nextCodexMCPConfig, error)) (string, error) {
	if r == nil || ctx == nil || ctx.Err() != nil || resolve == nil || !nextNativeMCPServerID.MatchString(serverID) {
		return "", errNextNativeMCPUnavailable
	}
	c, err := resolve(serverID)
	if err != nil || c.ServerID != serverID || !validNextCodexMCPConfig(c) {
		return "", errNextNativeMCPUnavailable
	}
	return r.prepareNextCodexMCPRegistryChecked(ctx, profile, keyHex, c, func() error {
		return reverifyNextNativeMCPConfig(serverID, c, resolve)
	})
}

func (r *Runtime) attachNextNativeMCPResolved(ctx context.Context, serverID string, resolve func(string) (nextCodexMCPConfig, error)) error {
	if r == nil || ctx == nil || ctx.Err() != nil || resolve == nil || !nextNativeMCPServerID.MatchString(serverID) {
		return errNextNativeMCPUnavailable
	}
	c, err := resolve(serverID)
	if err != nil || c.ServerID != serverID || !validNextCodexMCPConfig(c) {
		return errNextNativeMCPUnavailable
	}
	r.lifecycleMu.RLock()
	pinned := !r.closing && r.nextNativeMCPPrepared != nil && sameNextNativeMCPConfig(*r.nextNativeMCPPrepared, c)
	r.lifecycleMu.RUnlock()
	if !pinned {
		return errNextNativeMCPUnavailable
	}
	return r.attachNextCodexInstrumentedMCPChecked(ctx, c, func() error {
		return reverifyNextNativeMCPConfig(serverID, c, resolve)
	})
}

func reverifyNextNativeMCPConfig(serverID string, pinned nextCodexMCPConfig, resolve func(string) (nextCodexMCPConfig, error)) error {
	fresh, err := resolve(serverID)
	if err != nil || !sameNextNativeMCPConfig(pinned, fresh) {
		return errNextNativeMCPUnavailable
	}
	// This is a path-based snapshot, not an atomic verify-to-exec guarantee.
	// Mutation after verification, while waiting for lifecycleMu or before the
	// native app-server eventually execs the wrapper/backend, remains a release
	// blocker. Neither verifier tools nor M8 admission may run under the lock.
	return nil
}
