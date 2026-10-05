package desktopapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

// Dependencies are backend-only, installed before use. ReadConfig should return
// one coherent safe configuration, including Tunnel's generation when available.
// Port observations can tag their results with ConnectionConfigRevision.
type ConnectionDependencies struct {
	ReadConfig        func(context.Context, string) (desktopruntime.ConnectionConfig, error)
	ReadPasswordState func(context.Context, string) desktopruntime.OAuthPasswordState
	ReadPassword      func(context.Context, string) (string, desktopruntime.OAuthPasswordState)
	Transport         http.RoundTripper
}

func defaultConnectionDependencies() ConnectionDependencies {
	return ConnectionDependencies{
		ReadConfig:        desktopruntime.ReadConnectionConfig,
		ReadPasswordState: desktopruntime.ReadOAuthPasswordState,
		ReadPassword:      desktopruntime.ReadOAuthPassword,
		// No environment proxy, cookie jar, auth middleware or client certificate.
		Transport: newPublicEndpointTransport(),
	}
}

func NewConnectionServiceWithDependencies(root string, deps ConnectionDependencies) *ConnectionService {
	s := NewConnectionService(root)
	if deps.ReadConfig != nil {
		s.foundation.ReadConfig = deps.ReadConfig
	}
	if deps.ReadPassword != nil {
		s.foundation.ReadPassword = deps.ReadPassword
	}
	if deps.ReadPasswordState != nil {
		s.foundation.ReadPasswordState = deps.ReadPasswordState
	}
	if deps.Transport != nil {
		s.foundation.Transport = deps.Transport
	}
	return s
}

type ConnectionSnapshot struct {
	LocalMCPURL        string                            `json:"localMCPURL,omitempty"`
	PublicMCPURL       string                            `json:"publicMCPURL,omitempty"`
	Port               int                               `json:"port"`
	Mode               string                            `json:"mode"`
	OAuthEnabled       bool                              `json:"oauthEnabled"`
	OAuthPasswordState desktopruntime.OAuthPasswordState `json:"oauthPasswordState"`
	ConfigRevision     string                            `json:"configRevision"`
	TunnelGeneration   string                            `json:"tunnelGeneration,omitempty"`
}

type ConnectionSnapshotResult struct {
	Snapshot ConnectionSnapshot `json:"snapshot"`
	Error    *APIError          `json:"error,omitempty"`
}

// This type deliberately cannot be embedded in the ordinary snapshot.
type OAuthPasswordRevealResult struct {
	Password         string                            `json:"password,omitempty"`
	State            desktopruntime.OAuthPasswordState `json:"state"`
	ConfigRevision   string                            `json:"configRevision,omitempty"`
	TunnelGeneration string                            `json:"tunnelGeneration,omitempty"`
	Error            *APIError                         `json:"error,omitempty"`
}

func localMCPURL(endpoint string, port int) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" ||
		u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" ||
		(u.Path != "" && u.Path != "/") || u.Port() != strconv.Itoa(port) {
		return ""
	}
	switch u.Hostname() {
	case "127.0.0.1", "::1", "localhost":
	default:
		return ""
	}
	return "http://" + u.Host + "/mcp"
}

func safeConnectionConfig(c desktopruntime.ConnectionConfig) (desktopruntime.ConnectionConfig, bool) {
	if c.Port < 1 || c.Port > 65535 || localMCPURL(c.CoreEndpoint, c.Port) == "" {
		return desktopruntime.ConnectionConfig{}, false
	}
	switch c.Mode {
	case "none", "quick", "named":
	default:
		return desktopruntime.ConnectionConfig{}, false
	}
	c.CoreEndpoint = localMCPURL(c.CoreEndpoint, c.Port)
	c.PublicOrigin = publicOrigin(c.PublicOrigin)
	if c.Mode == "none" {
		c.PublicOrigin = ""
	}
	return c, true
}

// Hash only the typed safe projection, never env bytes, tokens or passwords.
func ConnectionConfigRevision(c desktopruntime.ConnectionConfig) string {
	safe, ok := safeConnectionConfig(c)
	if !ok {
		return ""
	}
	data, _ := json.Marshal(safe)
	sum := sha256.Sum256(append([]byte("connection-v1\x00"), data...))
	return hex.EncodeToString(sum[:])
}

func connectionReadError() *APIError {
	return NewError("connection_config_unavailable", "Connection configuration unavailable", ErrorCategoryUnavailable, true, nil)
}

func (s *ConnectionService) config(ctx context.Context) (desktopruntime.ConnectionConfig, string, *APIError) {
	if s.rootError != nil || ctx.Err() != nil {
		return desktopruntime.ConnectionConfig{}, "", connectionReadError()
	}
	c, err := s.foundation.ReadConfig(ctx, s.runtimeRoot)
	revision := ConnectionConfigRevision(c)
	if err != nil || revision == "" {
		return desktopruntime.ConnectionConfig{}, "", connectionReadError()
	}
	return c, revision, nil
}

func (s *ConnectionService) unchanged(ctx context.Context, revision string) bool {
	_, current, err := s.config(ctx)
	return err == nil && current == revision
}

func safePasswordState(state desktopruntime.OAuthPasswordState) desktopruntime.OAuthPasswordState {
	switch state {
	case desktopruntime.OAuthPasswordStored, desktopruntime.OAuthPasswordMissing,
		desktopruntime.OAuthPasswordUnreadable, desktopruntime.OAuthPasswordUnavailable:
		return state
	default:
		return desktopruntime.OAuthPasswordUnavailable
	}
}

func (s *ConnectionService) Snapshot(ctx context.Context) ConnectionSnapshotResult {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, revision, err := s.config(ctx)
	if err != nil {
		return ConnectionSnapshotResult{Error: err}
	}
	state := s.foundation.ReadPasswordState(ctx, s.runtimeRoot)
	if !s.unchanged(ctx, revision) {
		return ConnectionSnapshotResult{Error: connectionReadError()}
	}
	public := publicOrigin(c.PublicOrigin)
	if c.Mode == "none" {
		public = ""
	}
	if public != "" {
		public += "/mcp"
	}
	return ConnectionSnapshotResult{Snapshot: ConnectionSnapshot{
		LocalMCPURL: localMCPURL(c.CoreEndpoint, c.Port), PublicMCPURL: public,
		Port: c.Port, Mode: c.Mode, OAuthEnabled: c.OAuthEnabled,
		OAuthPasswordState: safePasswordState(state), ConfigRevision: revision, TunnelGeneration: c.TunnelGeneration,
	}}
}

func (s *ConnectionService) RevealOAuthPassword(ctx context.Context) OAuthPasswordRevealResult {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, revision, err := s.config(ctx)
	if err != nil {
		return OAuthPasswordRevealResult{State: desktopruntime.OAuthPasswordUnavailable, Error: err}
	}
	password, state := s.foundation.ReadPassword(ctx, s.runtimeRoot)
	result := OAuthPasswordRevealResult{State: safePasswordState(state), ConfigRevision: revision, TunnelGeneration: c.TunnelGeneration}
	if !s.unchanged(ctx, revision) {
		result.State = desktopruntime.OAuthPasswordUnavailable
		result.Error = NewError("connection_config_stale", "Connection configuration changed", ErrorCategoryUnavailable, true, nil)
		return result
	}
	if result.State == desktopruntime.OAuthPasswordStored && password != "" {
		result.Password = password
	}
	return result
}
