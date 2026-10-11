package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/process"
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

// PrepareNextCodexMCPRegistry creates a source-only, Core-owned private
// listener from the already authenticated native app-server generation.
// Unlike live ScopeProvider, the listener never calls back into the Adapter
// during an MCP spawn (avoids thread/start/MCP init deadlock).
// This is deliberately NOT called in NewRuntime or any public tool.
func (r *Runtime) PrepareNextCodexMCPRegistry(ctx context.Context, profile config.ACPProfile, keyHex, server, digest string) (string, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return "", errNextCodexRegistryUnavailable
	}
	// Qualification does not start an Adapter: validate exact Next scope first.
	if _, err := r.nextCodexMCPRegistrarScopeProvider(profile, keyHex); err != nil {
		return "", err
	}
	if !profile.Enabled || profile.ID != "codex" || profile.Kind != "codex" {
		return "", errNextCodexRegistryUnavailable
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return "", errNextCodexRegistryUnavailable
	}
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.closing || r.nextCodexRegistry != nil {
		return "", errNextCodexRegistryUnavailable
	}
	births, err := r.acp.BootstrapNextCodexAppServerBirths(ctx, key)
	if err != nil {
		return "", errNextCodexRegistryUnavailable
	}
	pin, err := launchregistry.NewGenerationPin(births.Adapter, births.AppServer)
	if err != nil {
		return "", errNextCodexRegistryUnavailable
	}
	svc, err := launchregistry.StartPrivateCoreListener(r.cfg.AgentDockHome, pin.Scope, server, digest)
	if err != nil {
		pin.Revoke()
		return "", errNextCodexRegistryUnavailable
	}
	r.nextCodexPin = pin
	r.nextCodexRegistry = svc
	r.nextCodexRegisteredID = server
	r.nextCodexRegisteredDigest = digest
	return svc.SocketPath(), nil
}

// NextCodexMCPRegistrationSnapshot is read-only. No PID signaling, server
// shutdown or task ownership interpretation is permitted through this API.
func (r *Runtime) NextCodexMCPRegistrationSnapshot() []launchregistry.Observation {
	if r == nil {
		return nil
	}
	r.lifecycleMu.RLock()
	defer r.lifecycleMu.RUnlock()
	if r.closing || r.nextCodexRegistry == nil {
		return nil
	}
	return r.nextCodexRegistry.Snapshot()
}

// ApprovedNextCodexMCPConfig is supplied by trusted Next Core after the P5/M8
// permission gate has approved a single logical stdio MCP server. No user
// Prompt or ACP Adapter may construct or change this configuration.
type ApprovedNextCodexMCPConfig struct {
	ServerID          string
	WrapperExecutable string
	BackendExecutable string
	BackendArgs       []string
	ConfigDigest      string
}

// NextCodexMCPConfigDigest hashes a fully specified immutable server launch
// with domain separation; intentionally no process IDs or runtime secrets.
func NextCodexMCPConfigDigest(id, executable string, args []string) string {
	raw, _ := json.Marshal(struct {
		Domain, Server, Executable string
		Args                       []string
	}{"agentdock-next-mcp-allowlist-v1", id, executable, args})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func ownerExecutable(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Mode()&0111 == 0 || st.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return true
}

// AttachNextCodexInstrumentedMCP installs an allowlisted stdio wrapper only
// AFTER a signed, pre-Session Core-owned registry was created. No global
// ~/.codex config, installed CLI, Browser, Memory or shared process is touched.
// The native server may be reused across Sessions: release never kills it.
func (r *Runtime) AttachNextCodexInstrumentedMCP(ctx context.Context, c ApprovedNextCodexMCPConfig) error {
	if r == nil || ctx == nil || ctx.Err() != nil || r.acp == nil || !ownerExecutable(c.WrapperExecutable) || !ownerExecutable(c.BackendExecutable) || c.ConfigDigest != NextCodexMCPConfigDigest(c.ServerID, c.BackendExecutable, c.BackendArgs) {
		return errNextCodexRegistryUnavailable
	}
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.closing || r.nextCodexRegistry == nil || r.nextCodexPin == nil || r.nextCodexRegisteredID != c.ServerID || r.nextCodexRegisteredDigest != c.ConfigDigest {
		return errNextCodexRegistryUnavailable
	}
	if _, err := r.nextCodexPin.Scope(ctx); err != nil {
		return errNextCodexRegistryUnavailable
	}
	coreBirth, err := process.ObserveBirthIdentity(ctx, os.Getpid())
	if err != nil {
		return errNextCodexRegistryUnavailable
	}
	coreJSON, err := json.Marshal(coreBirth)
	if err != nil {
		return errNextCodexRegistryUnavailable
	}
	argv := []string{"--socket", r.nextCodexRegistry.SocketPath(), "--server", c.ServerID, "--config-digest", c.ConfigDigest, "--", c.BackendExecutable}
	argv = append(argv, c.BackendArgs...)
	server := acpruntime.SessionMCPServer{
		Name: c.ServerID, Command: c.WrapperExecutable, Args: argv,
		Env: []acpruntime.SessionMCPHeader{{Name: "AGENTDOCK_NEXT_MCP_LAUNCH_CORE_IDENTITY", Value: string(coreJSON)}},
	}
	if err := r.acp.InstallNextCodexInstrumentedSessionMCP(server); err != nil {
		return errNextCodexRegistryUnavailable
	}
	return nil
}

// NextCodexMCPReconciliation provides only provenance/lease observations for
// operator audit. Never parse Codex self-report into a permission to kill.
// The service intentionally always refuses to authorize native PID cleanup;
// verified graceful release belongs to ACP session close/app-server recycle.
func (r *Runtime) NextCodexMCPReconciliation(ctx context.Context) []launchregistry.Reconciliation {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return nil
	}
	r.lifecycleMu.RLock()
	defer r.lifecycleMu.RUnlock()
	if r.closing || r.nextCodexRegistry == nil || r.acp == nil {
		return nil
	}
	borrowers := r.acp.NextCodexMCPBorrowers()
	observations := r.nextCodexRegistry.Snapshot()
	results := make([]launchregistry.Reconciliation, 0, len(observations))
	for _, ob := range observations {
		results = append(results, launchregistry.Reconcile(ctx, ob, borrowers))
	}
	return results
}
