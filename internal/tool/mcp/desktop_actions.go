package mcp

import (
	"context"
	"errors"
	"strings"

	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

var desktopActions = []string{
	"desktop_snapshot", "desktop_inspect", "desktop_create", "desktop_update", "desktop_remove",
	"desktop_set_enabled", "desktop_env_snapshot", "desktop_env_set", "desktop_env_unset", "desktop_env_purge",
	"desktop_reconnect", "desktop_auth_status", "desktop_authorize", "desktop_auth_clear",
}

// Desktop errors are a protected allowlist projection. Never forward Cause,
// provider Message, Details, URL, stderr or request value to the renderer.
func protectedMCPError(err error) *ToolError {
	code := "MCP_ERROR"
	var typed *mcpclient.Error
	if errors.As(err, &typed) {
		code = mcpclient.SafeErrorCode(typed.Code)
	}
	category := "external"
	if strings.Contains(code, "CONFLICT") {
		category = "conflict"
	}
	if strings.Contains(code, "INVALID") || strings.Contains(code, "OWNED") || strings.Contains(code, "IMMUTABLE") {
		category = "validation"
	}
	return toolErrorDetails(code, "Desktop MCP operation could not complete", category, nil)
}

func desktopConfig(r ManageRequest) mcpclient.ServerConfig {
	return mcpclient.ServerConfig{
		Name: r.Name, Description: r.Description, Transport: strings.ToLower(strings.TrimSpace(r.Transport)), ProtocolVersion: r.ProtocolVersion,
		URL: r.URL, Command: r.Command, Args: r.Args, Cwd: r.CWD, HeaderEnv: cloneStringMap(r.HeaderEnv),
		EnvFromEnv: cloneStringMap(r.EnvFromEnv), Enabled: boolValue(r.Enabled, false), TimeoutMS: intValue(r.TimeoutMS, 30000),
	}
}

func (s *Service) desktopManage(ctx context.Context, action string, r ManageRequest) (Result, error) {
	var err error
	result := Result{"action": action}
	switch action {
	case "desktop_snapshot":
		var snapshot mcpclient.ProtectedSnapshot
		snapshot, err = s.mcpClients.DesktopSnapshot()
		result["registry_revision"], result["authoritative"], result["servers"], result["count"] = snapshot.RegistryRevision, snapshot.Authoritative, snapshot.Servers, len(snapshot.Servers)
	case "desktop_inspect":
		var revision string
		var server mcpclient.ProtectedServer
		revision, server, err = s.mcpClients.DesktopInspect(r.Name)
		result["registry_revision"], result["server"] = revision, server
	case "desktop_create", "desktop_update", "desktop_remove", "desktop_set_enabled":
		var mutation mcpclient.MutationResult
		cfg := desktopConfig(r)
		switch action {
		case "desktop_create":
			err = mcpclient.ValidateDesktopConfig(cfg)
			if err == nil {
				mutation, err = s.mcpClients.AddChecked(cfg, r.ExpectedRegistryRevision)
			}
		case "desktop_update":
			// Omitted protected args/endpoint preserve authoritative persisted data.
			// Update rechecks both tokens atomically before persisting this replacement.
			var registry mcpclient.RegistrySnapshot
			registry, err = s.mcpClients.Registry()
			if err == nil {
				old, exists := registry.Servers[strings.TrimSpace(r.Name)]
				if exists && old.SourceType == "plugin" {
					err = &mcpclient.Error{Code: "MCP_OWNED_BY_PLUGIN"}
				} else {
					if r.Enabled == nil {
						cfg.Enabled = old.Enabled
					}
					validationCfg := cfg
					if cfg.Transport == old.Transport {
						if r.URL == "" {
							cfg.URL = old.URL
							validationCfg.URL = ""
						}
						if r.Args == nil {
							cfg.Args = old.Args
							validationCfg.Args = nil
						}
					}
					err = mcpclient.ValidateDesktopConfig(validationCfg)
					if err == nil {
						mutation, err = s.mcpClients.Update(r.Name, cfg, r.ExpectedRegistryRevision, r.ExpectedGeneration)
					}
				}
			}
		case "desktop_remove":
			mutation, err = s.mcpClients.RemoveChecked(r.Name, r.ExpectedRegistryRevision, r.ExpectedGeneration)
		case "desktop_set_enabled":
			if r.Enabled == nil {
				err = &mcpclient.Error{Code: "MCP_CONFIG_INVALID"}
			} else {
				mutation, err = s.mcpClients.SetEnabledChecked(r.Name, *r.Enabled, r.ExpectedRegistryRevision, r.ExpectedGeneration)
			}
		}
		if mutation.Persisted {
			result["persisted"], result["runtime_applied"], result["completed"] = true, mutation.RuntimeApplied, err == nil
			result["recovery_required"] = err != nil
			result["registry_revision"] = mutation.Registry.Revision
			if mutation.Server.Name != "" {
				result["server"] = s.mcpClients.ProtectedServer(mutation.Server)
			}
			if err != nil {
				result["safe_error"] = protectedMCPError(err)
				return result, nil
			}
		}
	case "desktop_env_snapshot", "desktop_env_set", "desktop_env_unset", "desktop_env_purge":
		snapshot, envErr := s.mcpClients.DesktopEnvironment(r.Name, strings.TrimPrefix(action, "desktop_env_"), r.Key, r.Value, r.ExpectedEnvRevision, r.ExpectedRegistryRevision, r.ExpectedGeneration)
		err = envErr
		result["env_revision"], result["items"], result["count"] = snapshot.Revision, snapshot.Entries, len(snapshot.Entries)
	case "desktop_reconnect":
		var server mcpclient.ProtectedServer
		var tools []mcpclient.ToolSummary
		server, tools, err = s.mcpClients.DesktopReconnect(ctx, r.Name, r.ExpectedRegistryRevision, r.ExpectedGeneration)
		result["server"], result["tools"], result["tool_count"] = server, tools, len(tools)
	case "desktop_auth_status":
		status, statusErr := s.mcpClients.DesktopAuthorizationStatus(r.Name)
		err = statusErr
		result["status"], result["callback_options"] = status.Status, status.CallbackOptions
	case "desktop_authorize":
		auth, authErr := s.mcpClients.DesktopAuthorize(ctx, r.Name, r.CallbackID, r.ExpectedRegistryRevision, r.ExpectedGeneration)
		err = authErr
		if auth.AuthorizationURL != "" {
			result["authorization_url"], result["callback_id"], result["expires_at"] = auth.AuthorizationURL, auth.CallbackID, auth.ExpiresAt
		}
		if len(auth.CallbackOptions) > 0 {
			result["callback_options"] = auth.CallbackOptions
		}
	case "desktop_auth_clear":
		err = s.mcpClients.DesktopClearAuthorization(r.Name, r.ExpectedRegistryRevision, r.ExpectedGeneration)
		result["removed"] = err == nil
	default:
		return nil, toolErrorDetails("INVALID_ACTION", "Unsupported Desktop MCP action", "validation", nil)
	}
	if err != nil {
		return nil, protectedMCPError(err)
	}
	return result, nil
}
