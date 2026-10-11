package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	"github.com/uvwt/agentdock/internal/mcp/launchregistry"
	"github.com/uvwt/agentdock/internal/permission"
	"github.com/uvwt/agentdock/internal/process"
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

// prepareNextCodexMCPRegistry is Core-private: no frontend/bootstrap principal
// route exists. A caller checksum is only input integrity, never authorization.
// Admission precedes both the cross-Core reservation and native Adapter startup.
func (r *Runtime) prepareNextCodexMCPRegistry(ctx context.Context, profile config.ACPProfile, keyHex string, c nextCodexMCPConfig) (socket string, retErr error) {
	c.BackendArgs = slices.Clone(c.BackendArgs)
	principal, ok := requestmeta.AuthPrincipalFromContext(ctx)
	if r == nil || ctx == nil || ctx.Err() != nil || !ok || !validNextCodexMCPConfig(c) {
		return "", errNextCodexRegistryUnavailable
	}
	if _, err := r.nextCodexMCPRegistrarScopeProvider(profile, keyHex); err != nil {
		return "", err
	}
	// Bind the actual configured Adapter, not an arbitrary caller profile.
	configured := false
	for _, actual := range r.cfg.EffectiveACPProfiles() {
		if actual.ID == profile.ID {
			left, _ := json.Marshal(actual)
			right, _ := json.Marshal(profile)
			configured = string(left) == string(right)
		}
	}
	if !configured {
		return "", errNextCodexRegistryUnavailable
	}
	finish, revision, err := r.admitNextCodexMCP(ctx, c, "prepare")
	if err != nil {
		return "", err
	}
	defer func() { finish(retErr) }()
	key, _ := hex.DecodeString(keyHex)
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.closing || r.nextCodexStartupLock != nil || r.nextCodexRegistry != nil || ctx.Err() != nil || r.permissions.Policy().Revision != revision || r.permissions.VerifyDiskState() != nil {
		return "", errNextCodexRegistryUnavailable
	}
	lock, err := acquireNextCodexStartupLock(r.cfg.AgentDockHome)
	if err != nil {
		return "", errNextCodexRegistryUnavailable
	}
	// Retain even on uncertain bootstrap failure; never allow another Core to
	// start a replacement before this Runtime tears down its owned ACP service.
	r.nextCodexStartupLock = lock
	if ctx.Err() != nil || r.permissions.Policy().Revision != revision || r.permissions.VerifyDiskState() != nil || !validNextCodexMCPConfig(c) {
		return "", errNextCodexRegistryUnavailable
	}
	// Create the crash fence durably while owning the generation lock. If
	// this Core dies at any point after this line, another Core cannot
	// mistake flock release for proof that native MCP children are gone.
	if err := lock.markBootstrapAttempt(); err != nil {
		return "", errNextCodexRegistryUnavailable
	}
	births, err := r.acp.BootstrapNextCodexAppServerBirths(ctx, key)
	if err != nil || ctx.Err() != nil || r.permissions.Policy().Revision != revision || lock.validate() != nil {
		return "", errNextCodexRegistryUnavailable
	}
	pin, err := launchregistry.NewGenerationPin(births.Adapter, births.AppServer)
	if err != nil {
		return "", errNextCodexRegistryUnavailable
	}
	// Policy changes revoke observation authority without signaling any PID.
	scope := func(ctx context.Context) (launchregistry.TrustedScope, error) {
		if r.permissions.Policy().Revision != revision || r.permissions.VerifyDiskState() != nil || lock.validate() != nil {
			return launchregistry.TrustedScope{}, errNextCodexRegistryUnavailable
		}
		return pin.Scope(ctx)
	}
	svc, err := launchregistry.StartPrivateCoreListener(r.cfg.AgentDockHome, scope, c.ServerID, c.ConfigDigest)
	if err != nil {
		pin.Revoke()
		return "", errNextCodexRegistryUnavailable
	}
	if ctx.Err() != nil || r.permissions.Policy().Revision != revision || r.permissions.VerifyDiskState() != nil || lock.validate() != nil {
		pin.Revoke()
		_ = svc.Close()
		return "", errNextCodexRegistryUnavailable
	}
	r.nextCodexPin = pin
	r.nextCodexRegistry = svc
	r.nextCodexRegisteredID = c.ServerID
	r.nextCodexRegisteredDigest = c.ConfigDigest
	r.nextCodexPrincipal = principal
	r.nextCodexPolicyRevision = revision
	return svc.SocketPath(), nil
}

// Only the trusted context carries identity. The redacted M8 payload contains
// a digest; raw executable paths/arguments (possibly credentials) never enter
// approval records. No live Session or synthetic authenticated identity is used.
func (r *Runtime) admitNextCodexMCP(ctx context.Context, c nextCodexMCPConfig, action string) (permission.HostOperationFinish, uint64, error) {
	if _, ok := requestmeta.AuthPrincipalFromContext(ctx); !ok || (action != "prepare" && action != "attach") || r == nil || r.permissions == nil || r.execution == nil || r.admission == nil || r.admission.Store() != r.permissions || r.permissions.RuntimeEpoch() != r.execution.Epoch() {
		return nil, 0, errNextCodexRegistryUnavailable
	}
	revision := r.permissions.Policy().Revision
	payload := map[string]any{"config_sha256": c.ConfigDigest}
	fingerprint, err := hostOperationFingerprint("next_codex_mcp", action, payload)
	if err != nil {
		return nil, 0, errNextCodexRegistryUnavailable
	}
	finish, err := r.admitHostOperation(ctx, permission.HostOperation{Tool: "next_codex_mcp", Action: action, Source: "internal", PreparedFingerprint: fingerprint, Payload: payload})
	return finish, revision, err
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

// nextCodexMCPConfig is untrusted input until actual M8 admission succeeds.
type nextCodexMCPConfig struct {
	ServerID          string
	WrapperExecutable string
	BackendExecutable string
	BackendArgs       []string
	ConfigDigest      string
}

// nextCodexMCPConfigDigest hashes a fully specified immutable server launch
// with domain separation. Only the digest is exposed to the permission store;
// raw arguments may contain credentials and must never be logged.
func nextCodexMCPConfigDigest(id, wrapper, executable string, args []string) string {
	raw, _ := json.Marshal(struct {
		Domain, Server, Wrapper, Executable string
		Args                                []string
	}{"agentdock-next-mcp-allowlist-v2", id, wrapper, executable, args})
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

var nextCodexServerPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,80}$`)

func validNextCodexMCPConfig(c nextCodexMCPConfig) bool {
	// JSON canonicalization must not collapse invalid UTF-8 into replacement
	// characters; NUL is not a valid exec path/argument either.
	for _, value := range append([]string{c.WrapperExecutable, c.BackendExecutable}, c.BackendArgs...) {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return false
		}
	}
	return nextCodexServerPattern.MatchString(c.ServerID) && ownerExecutable(c.WrapperExecutable) && ownerExecutable(c.BackendExecutable) && c.ConfigDigest == nextCodexMCPConfigDigest(c.ServerID, c.WrapperExecutable, c.BackendExecutable, c.BackendArgs)
}

// attachNextCodexInstrumentedMCP requires a fresh M8 Allow, same principal,
// unchanged policy and immutable prepared configuration. No production hook.
func (r *Runtime) attachNextCodexInstrumentedMCP(ctx context.Context, c nextCodexMCPConfig) (retErr error) {
	c.BackendArgs = slices.Clone(c.BackendArgs)
	principal, ok := requestmeta.AuthPrincipalFromContext(ctx)
	if r == nil || ctx == nil || ctx.Err() != nil || !ok || r.acp == nil || !validNextCodexMCPConfig(c) {
		return errNextCodexRegistryUnavailable
	}
	finish, revision, err := r.admitNextCodexMCP(ctx, c, "attach")
	if err != nil {
		return err
	}
	defer func() { finish(retErr) }()
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.closing || ctx.Err() != nil || r.permissions.VerifyDiskState() != nil || r.nextCodexRegistry == nil || r.nextCodexPin == nil || r.nextCodexStartupLock == nil || r.nextCodexStartupLock.validate() != nil || r.nextCodexPrincipal != principal || r.nextCodexPolicyRevision != revision || r.permissions.Policy().Revision != revision || r.nextCodexRegisteredID != c.ServerID || r.nextCodexRegisteredDigest != c.ConfigDigest {
		return errNextCodexRegistryUnavailable
	}
	if !validNextCodexMCPConfig(c) {
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
	if ctx.Err() != nil || r.permissions.Policy().Revision != revision || r.nextCodexStartupLock.validate() != nil || !validNextCodexMCPConfig(c) {
		return errNextCodexRegistryUnavailable
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
