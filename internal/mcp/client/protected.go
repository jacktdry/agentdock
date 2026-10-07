package client

import (
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ProtectedServer is a Core bridge projection, never a raw ServerConfig or
// provider response. Unknown tool counts are distinct from a discovered zero.
type ProtectedServer struct {
	Name            string               `json:"name"`
	DisplayName     string               `json:"display_name"`
	Description     string               `json:"description"`
	SourceType      string               `json:"source_type"`
	PluginName      string               `json:"plugin_name,omitempty"`
	Transport       string               `json:"transport"`
	ProtocolVersion string               `json:"protocol_version,omitempty"`
	URL             string               `json:"url,omitempty"`
	URLProtected    bool                 `json:"url_protected,omitempty"`
	Command         string               `json:"command,omitempty"`
	Cwd             string               `json:"cwd,omitempty"`
	Args            []string             `json:"args,omitempty"`
	ArgsProtected   bool                 `json:"args_protected"`
	Enabled         bool                 `json:"enabled"`
	TimeoutMS       int                  `json:"timeout_ms"`
	HeaderEnv       map[string]string    `json:"header_env,omitempty"`
	EnvFromEnv      map[string]string    `json:"env_from_env,omitempty"`
	Generation      string               `json:"generation"`
	Observation     ProtectedObservation `json:"observation"`
}

type ProtectedObservation struct {
	Connection    string `json:"connection"`
	Status        string `json:"status"`
	AuthStatus    string `json:"auth_status"`
	ToolCount     *int   `json:"tool_count"`
	ObservedAt    string `json:"observed_at,omitempty"`
	LastErrorCode string `json:"last_error_code,omitempty"`
	Stale         bool   `json:"stale"`
}

type ProtectedSnapshot struct {
	RegistryRevision string            `json:"registry_revision"`
	Authoritative    bool              `json:"authoritative"`
	Servers          []ProtectedServer `json:"servers"`
}

// Credential-like values and ambiguous argument syntax are preserve-only.
// This deliberately prefers hiding benign opaque strings over exposing grants.
var credentialShape = regexp.MustCompile(`(?i)(token|secret|password|passwd|credential|api[_-]?key|access[_-]?key|authorization|bearer|private[_-]?key|(^|[^a-z])(key|auth|cookie|session|jwt|oauth)([^a-z]|$)|[a-z0-9_+/=-]{24,})`)
var safeArgument = regexp.MustCompile(`^[A-Za-z0-9_./:@+-]{1,128}$`)

func protectedText(value string) string {
	if credentialShape.MatchString(value) || strings.ContainsAny(value, "\r\n\x00") {
		return "[protected]"
	}
	return value
}

func protectedArgs(args []string) bool {
	for _, arg := range args {
		if !safeArgument.MatchString(arg) || credentialShape.MatchString(arg) || strings.Contains(arg, "://") || arg == "-H" || arg == "-e" || arg == "-p" || arg == "-c" {
			return true
		}
	}
	return false
}

// SafeErrorCode accepts only Core-owned codes, never provider-generated text.
func SafeErrorCode(errCode string) string {
	switch errCode {
	case "":
		return ""
	case "MCP_REGISTRY_CONFLICT", "MCP_SERVER_GENERATION_CONFLICT", "MCP_ENV_CONFLICT",
		"MCP_OWNED_BY_PLUGIN", "MCP_NAME_IMMUTABLE", "MCP_CONFIG_INVALID",
		"MCP_SERVER_EXISTS", "MCP_SERVER_NOT_FOUND", "MCP_SERVER_DISABLED", "MCP_SERVER_COLLISION",
		"MCP_MANAGER_CLOSED", "MCP_REGISTRY_READ_FAILED", "MCP_REGISTRY_WRITE_FAILED",
		"MCP_CLIENT_CLOSE_FAILED", "MCP_ENV_ERROR", "MCP_AUTH_REQUIRED", "MCP_AUTH_UNSUPPORTED",
		"MCP_AUTH_CLEAR_FAILED", "MCP_AUTH_CANCELLED", "MCP_AUTH_FAILED", "MCP_AUTH_CALLBACK_REQUIRED",
		"MCP_AUTH_CALLBACK_INVALID", "MCP_AUTH_CALLBACK_UNAVAILABLE", "MCP_AUTH_DENIED", "MCP_AUTH_EXPIRED",
		"MCP_CREDENTIAL_REQUIRED", "MCP_CONNECTION_FAILED", "MCP_START_FAILED",
		"MCP_TRANSPORT_ERROR", "MCP_TRANSPORT_REJECTED", "MCP_TRANSPORT_UNSUPPORTED",
		"MCP_PROTOCOL_ERROR", "MCP_INVALID_RESPONSE", "MCP_SCHEMA_INVALID", "MCP_TIMEOUT":
		return errCode
	default:
		return "MCP_ERROR"
	}
}

func projectConfig(cfg ServerConfig) ProtectedServer {
	p := ProtectedServer{
		Name: cfg.Name, DisplayName: protectedText(cfg.DisplayName), Description: protectedText(cfg.Description),
		SourceType: cfg.SourceType, PluginName: cfg.PluginName, Transport: cfg.Transport,
		ProtocolVersion: cfg.ProtocolVersion, Enabled: cfg.Enabled, TimeoutMS: cfg.TimeoutMS,
		HeaderEnv: cloneStringMap(cfg.HeaderEnv), EnvFromEnv: cloneStringMap(cfg.EnvFromEnv), Generation: cfg.Generation,
		Command: protectedText(cfg.Command), Cwd: protectedText(cfg.Cwd), ArgsProtected: protectedArgs(cfg.Args),
	}
	if !p.ArgsProtected {
		p.Args = append([]string(nil), cfg.Args...)
	}
	if cfg.URL != "" {
		u, err := url.Parse(cfg.URL)
		if err != nil || u.Host == "" || protectedText(u.Host) != u.Host || (u.Scheme != "https" && u.Scheme != "http") {
			p.URLProtected = true
		} else {
			p.URLProtected = u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != ""
			u.User, u.RawQuery, u.Fragment, u.RawFragment = nil, "", "", ""
			u.ForceQuery = false
			if protectedText(u.Path) != u.Path {
				u.Path, u.RawPath = "", ""
				p.URLProtected = true
			}
			p.URL = u.String()
		}
	}
	return p
}

// ValidateDesktopConfig enforces the Shared write subset. Legacy callers keep
// their existing validation. Protected URLs/args may be preserved by omission.
func ValidateDesktopConfig(cfg ServerConfig) error {
	cfg = normalizeServerConfig(cfg)
	// An omitted endpoint is validated by the registry primitive after create
	// validation or after an update has preserved the old protected endpoint.
	if cfg.Transport == TransportStreamableHTTP && cfg.URL != "" {
		u, err := url.Parse(cfg.URL)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.ForceQuery ||
			(u.Scheme != "https" && u.Scheme != "http") {
			return newError("MCP_CONFIG_INVALID", "Desktop endpoint must be a safe absolute HTTP(S) URL", false, nil, nil)
		}
		ip := net.ParseIP(u.Hostname())
		if u.Scheme == "http" && u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return newError("MCP_CONFIG_INVALID", "Desktop remote endpoint requires HTTPS", false, nil, nil)
		}
	}
	if protectedArgs(cfg.Args) {
		return newError("MCP_CONFIG_INVALID", "Use environment bindings for protected arguments", false, nil, nil)
	}
	return nil
}

// DesktopSnapshot reads the authoritative registry and CURRENT memory only.
// It must never syncRegistry, discover tools, refresh tokens, or close clients.
func (m *Manager) DesktopSnapshot() (ProtectedSnapshot, error) {
	r, err := m.Registry()
	if err != nil {
		return ProtectedSnapshot{}, err
	}
	p := ProtectedSnapshot{RegistryRevision: r.Revision, Authoritative: true, Servers: make([]ProtectedServer, 0, len(r.Servers))}
	for _, cfg := range r.Servers {
		p.Servers = append(p.Servers, m.ProtectedServer(cfg))
	}
	sort.Slice(p.Servers, func(i, j int) bool { return p.Servers[i].Name < p.Servers[j].Name })
	return p, nil
}

func (m *Manager) DesktopInspect(name string) (string, ProtectedServer, error) {
	r, err := m.Registry()
	if err != nil {
		return "", ProtectedServer{}, err
	}
	cfg, ok := r.Servers[strings.TrimSpace(name)]
	if !ok {
		return "", ProtectedServer{}, newError("MCP_SERVER_NOT_FOUND", "MCP server not found", false, nil, nil)
	}
	return r.Revision, m.ProtectedServer(cfg), nil
}

// ProtectedServer also projects durable mutation post-state after cleanup failure.
func (m *Manager) ProtectedServer(cfg ServerConfig) ProtectedServer {
	p := projectConfig(cfg)
	p.Observation = ProtectedObservation{Connection: "unknown", Status: "unknown", AuthStatus: "unknown", Stale: true}
	m.mu.RLock()
	cached, ok := m.servers[cfg.Name]
	state := m.states[cfg.Name]
	if !ok || state == nil || cached.Generation != cfg.Generation {
		m.mu.RUnlock()
		return p
	}
	state.mu.Lock()
	m.mu.RUnlock()
	defer state.mu.Unlock()
	if state.generation != "" && state.generation != cfg.Generation {
		return p
	}
	o := &p.Observation
	o.Stale, o.Connection, o.Status = false, "not_connected", "idle"
	if !cfg.Enabled {
		o.Status = "disabled"
	}
	if state.client != nil {
		o.Connection, o.Status = "connected", "ready"
	}
	if state.lastError != "" {
		o.Status = "error"
	}
	o.LastErrorCode = SafeErrorCode(state.lastErrorCode)
	o.AuthStatus = "not_applicable"
	if cfg.Transport == TransportStreamableHTTP {
		// Snapshot observation is memory-only. Reading local grant metadata is
		// reserved for the separate passive authorization-status action.
		switch state.oauthStatus {
		case "unauthorized", "auth_required", "authorizing", "authorized":
			o.AuthStatus = state.oauthStatus
		default:
			o.AuthStatus = "unknown"
		}
		if o.AuthStatus == "auth_required" || o.AuthStatus == "authorizing" {
			o.Status = o.AuthStatus
		}
	}
	if !state.refreshedAt.IsZero() {
		count := len(state.tools)
		o.ToolCount, o.ObservedAt = &count, state.refreshedAt.UTC().Format(time.RFC3339Nano)
	}
	return p
}

type ProtectedCallback struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type AuthorizationStatus struct {
	Status          string              `json:"status"`
	CallbackOptions []ProtectedCallback `json:"callback_options,omitempty"`
}

// AuthorizationURL is the explicit, ephemeral OAuth handoff exception. Saved
// grants, client secrets and private callback objects never enter this result.
type ProtectedAuthorization struct {
	AuthorizationURL string              `json:"authorization_url,omitempty"`
	CallbackID       string              `json:"callback_id,omitempty"`
	ExpiresAt        string              `json:"expires_at,omitempty"`
	CallbackOptions  []ProtectedCallback `json:"callback_options,omitempty"`
}

func (m *Manager) protectedCallbacks() []ProtectedCallback {
	var out []ProtectedCallback
	for _, option := range m.oauth.CallbackOptions() {
		out = append(out, ProtectedCallback{ID: option.ID, Label: protectedText(option.Label)})
	}
	return out
}

// DesktopAuthorizationStatus reads local grants/flow metadata without token
// refresh. Private callback redirect URLs never enter the bridge projection.
func (m *Manager) DesktopAuthorizationStatus(name string) (AuthorizationStatus, error) {
	r, err := m.Registry()
	if err != nil {
		return AuthorizationStatus{}, err
	}
	cfg, ok := r.Servers[strings.TrimSpace(name)]
	if !ok {
		return AuthorizationStatus{}, newError("MCP_SERVER_NOT_FOUND", "MCP server not found", false, nil, nil)
	}
	status := AuthorizationStatus{Status: "not_applicable"}
	if cfg.Transport == TransportStreamableHTTP {
		status.Status = m.oauth.Status(cfg.StorageKey, cfg.URL)
		status.CallbackOptions = m.protectedCallbacks()
	}
	return status, nil
}
