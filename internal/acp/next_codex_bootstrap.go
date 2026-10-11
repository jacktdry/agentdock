package acp

import (
	"context"
	"debug/macho"
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

var errNextCodexBootstrapUnavailable = errors.New("Next Codex app-server bootstrap unavailable")

// BootstrapNextCodexAppServerBirths is a deliberate, restricted preparatory
// operation. It establishes the native app-server BEFORE the first Session is
// created, then authenticates its lineage through the Adapter challenge.
// Registration must be prepared before any MCP-bearing session/new. Ordinary
// ACP profiles and existing Sessions are not eligible for this operation.
func (m *Manager) BootstrapNextCodexAppServerBirths(ctx context.Context, key []byte) (NextCodexAppServerBirths, error) {
	deny := NextCodexAppServerBirths{}
	if m == nil || ctx == nil || ctx.Err() != nil || len(key) != 32 || m.opts.Agent.Environment["AGENTDOCK_NEXT_CODEX_MCP_ISOLATION"] != "source-only" || m.opts.Agent.Environment["AGENTDOCK_NEXT_MCP_OWNER_DIAGNOSTIC_KEY"] == "" {
		return deny, errNextCodexBootstrapUnavailable
	}
	// A Node.js/shell Codex launcher introduces an additional process layer,
	// so app-server→wrapper OS-parent provenance would be misleading.
	// The Next Core B4 contract requires a canonical direct native Mach-O
	// executable, not the npm JavaScript launcher or a symlink alias.
	if runtime.GOOS == "darwin" {
		bin := m.opts.Agent.Environment["CODEX_PATH"]
		if !filepath.IsAbs(bin) || filepath.Clean(bin) != bin {
			return deny, errNextCodexBootstrapUnavailable
		}
		st, err := os.Lstat(bin)
		if err != nil || !st.Mode().IsRegular() || st.Mode()&0111 == 0 || st.Mode()&os.ModeSymlink != 0 {
			return deny, errNextCodexBootstrapUnavailable
		}
		mach, err := macho.Open(bin)
		if err != nil {
			return deny, errNextCodexBootstrapUnavailable
		}
		_ = mach.Close()
	}
	m.mu.RLock()
	unsafe := m.closed || len(m.loaded) != 0 || len(m.runs) != 0
	m.mu.RUnlock()
	if unsafe {
		return deny, errNextCodexBootstrapUnavailable
	}
	// This explicitly starts only the configured ACP Adapter; it does not load
	// any persisted Session or attempt to create a Codex native Thread.
	if _, err := m.ensureProcess(ctx); err != nil {
		return deny, errNextCodexBootstrapUnavailable
	}
	m.mu.RLock()
	unsafe = m.closed || len(m.loaded) != 0 || len(m.runs) != 0
	m.mu.RUnlock()
	if unsafe {
		return deny, errNextCodexBootstrapUnavailable
	}
	births, err := m.NextCodexAppServerBirths(ctx, key)
	if err != nil {
		return deny, errNextCodexBootstrapUnavailable
	}
	return births, nil
}
