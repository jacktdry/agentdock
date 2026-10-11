package app

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/mcp/launchregistry"
)

const nextCodexMCPRegistrationEnv = "AGENTDOCK_NEXT_CODEX_MCP_REGISTRATION"

var errNextCodexRegistryUnavailable = errors.New("Next Codex MCP registry evidence unavailable")

// nextCodexMCPRegistrarScopeProvider connects the private Core-bound listener
// to the existing HMAC-authenticated Codex ACP native diagnostic. It never
// starts a new Adapter, new Session, or a listener. Its output is read-only;
// there is no native per-thread/transport owner or cleanup authority.
// No production caller may enable the listener before P5/M8 allowlist/config
// fingerprint and exact adapter environment origin are bound and UAT verified.
func (r *Runtime) nextCodexMCPRegistrarScopeProvider(profile config.ACPProfile, keyHex string) (launchregistry.ScopeProvider, error) {
	if r == nil || r.acp == nil || runtime.GOOS != "darwin" || r.cfg.Stdio || !r.cfg.ACPEnabled || !profile.Enabled || profile.Kind != "codex" || profile.ID != "codex" ||
		os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" || os.Getenv(nextCodexMCPRegistrationEnv) != "source-only" {
		return nil, errNextCodexRegistryUnavailable
	}
	home, err := os.UserHomeDir()
	if err != nil || r.cfg.AgentDockHome != filepath.Join(home, ".agentdock-next") {
		return nil, errNextCodexRegistryUnavailable
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return nil, errNextCodexRegistryUnavailable
	}
	return func(ctx context.Context) (launchregistry.TrustedScope, error) {
		if ctx == nil || ctx.Err() != nil {
			return launchregistry.TrustedScope{}, errNextCodexRegistryUnavailable
		}
		native, err := r.acp.NextCodexAppServerBirths(ctx, key)
		if err != nil {
			return launchregistry.TrustedScope{}, errNextCodexRegistryUnavailable
		}
		return launchregistry.TrustedScope{Adapter: native.Adapter, AppServer: native.AppServer}, nil
	}, nil
}
