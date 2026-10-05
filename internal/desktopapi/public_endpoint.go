package desktopapi

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"slices"
	"time"
)

type PublicEndpointResult struct {
	State            string    `json:"state"`
	ReasonCode       string    `json:"reasonCode,omitempty"`
	ConfigRevision   string    `json:"configRevision,omitempty"`
	TunnelGeneration string    `json:"tunnelGeneration,omitempty"`
	CheckedAt        time.Time `json:"checkedAt"`
	Error            *APIError `json:"error,omitempty"`
}

type endpointMetadata struct {
	Resource              string   `json:"resource"`
	AuthorizationServers  []string `json:"authorization_servers"`
	BearerMethods         []string `json:"bearer_methods_supported"`
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RegistrationEndpoint  string   `json:"registration_endpoint"`
	ResponseTypes         []string `json:"response_types_supported"`
	GrantTypes            []string `json:"grant_types_supported"`
	CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
	TokenAuthMethods      []string `json:"token_endpoint_auth_methods_supported"`
	ResourceIndicators    bool     `json:"resource_indicators_supported"`
}

// No URL argument: only the current configured public origin is probed.
// Reachability is discovery compatibility, never proof of Next ownership.
func (s *ConnectionService) TestPublicEndpoint(ctx context.Context) PublicEndpointResult {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	c, revision, err := s.config(ctx)
	result := PublicEndpointResult{State: "not_configured", ConfigRevision: revision,
		TunnelGeneration: c.TunnelGeneration, CheckedAt: time.Now().UTC(), Error: err}
	if err != nil {
		result.State = "unreachable"
		result.ReasonCode = "config_unavailable"
		return result
	}
	origin := publicOrigin(c.PublicOrigin)
	if origin != "" && c.Mode != "none" {
		client := &http.Client{Transport: s.foundation.Transport, Timeout: 8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resource, state, reason := readEndpointMetadata(ctx, client, origin+"/.well-known/oauth-protected-resource/mcp")
		result.State, result.ReasonCode = state, reason
		if state == "reachable" {
			if resource.Resource != origin+"/mcp" || len(resource.AuthorizationServers) != 1 ||
				resource.AuthorizationServers[0] != origin || !slices.Contains(resource.BearerMethods, "header") {
				result.State, result.ReasonCode = "unexpected_endpoint", "resource_metadata_mismatch"
			} else {
				auth, state, reason := readEndpointMetadata(ctx, client, origin+"/.well-known/oauth-authorization-server")
				result.State, result.ReasonCode = state, reason
				if state == "reachable" && (auth.Issuer != origin || auth.AuthorizationEndpoint != origin+"/oauth/authorize" ||
					auth.TokenEndpoint != origin+"/oauth/token" || auth.RegistrationEndpoint != origin+"/register" ||
					!slices.Contains(auth.ResponseTypes, "code") || !slices.Contains(auth.GrantTypes, "authorization_code") ||
					!slices.Contains(auth.CodeChallengeMethods, "S256") || !slices.Contains(auth.TokenAuthMethods, "none") || !auth.ResourceIndicators) {
					result.State, result.ReasonCode = "unexpected_endpoint", "oauth_metadata_mismatch"
				}
			}
		}
	}
	// Recheck the configuration independently of a canceled network request so
	// timeout alone does not falsely imply a configuration change.
	recheck, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer stop()
	if !s.unchanged(recheck, revision) {
		result.State, result.ReasonCode = "stale", "config_changed_or_unavailable"
	}
	result.CheckedAt = time.Now().UTC()
	return result
}

func readEndpointMetadata(ctx context.Context, client *http.Client, target string) (endpointMetadata, string, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return endpointMetadata{}, "unexpected_endpoint", "invalid_origin"
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return endpointMetadata{}, "unreachable", "request_failed"
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return endpointMetadata{}, "unexpected_endpoint", "redirect_rejected"
	}
	if resp.StatusCode != http.StatusOK {
		return endpointMetadata{}, "unreachable", "http_status"
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return endpointMetadata{}, "unexpected_endpoint", "invalid_metadata"
	}
	const maxBody = 64 << 10
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return endpointMetadata{}, "unreachable", "response_failed"
	}
	var metadata endpointMetadata
	if len(data) > maxBody || json.Unmarshal(data, &metadata) != nil {
		return endpointMetadata{}, "unexpected_endpoint", "invalid_metadata"
	}
	return metadata, "reachable", ""
}
