package mcp

import (
	"context"
	"errors"
	"strings"

	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/envstore"
	"github.com/uvwt/agentdock/internal/execution"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

func (s *Service) Manage(ctx context.Context, request ManageRequest) (Result, error) {
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if action == "" {
		action = "list"
	}
	if strings.HasPrefix(action, "desktop_") {
		return s.desktopManage(ctx, action, request)
	}
	switch action {
	case "list":
		servers := s.mcpClients.List()
		return Result{"action": action, "servers": servers, "count": len(servers)}, nil
	case "inspect":
		name := request.Name
		cfg, summary, err := s.mcpClients.Inspect(name)
		if err != nil {
			return nil, dynamicMCPToolError(err)
		}
		return Result{"action": action, "server": summary, "config": cfg}, nil
	case "add":
		cfg := mcpclient.ServerConfig{
			Name:            request.Name,
			Description:     request.Description,
			Transport:       request.Transport,
			ProtocolVersion: request.ProtocolVersion,
			URL:             request.URL,
			Command:         request.Command,
			Args:            append([]string(nil), request.Args...),
			Cwd:             request.CWD,
			HeaderEnv:       cloneStringMap(request.HeaderEnv),
			EnvFromEnv:      cloneStringMap(request.EnvFromEnv),
			Enabled:         boolValue(request.Enabled, true),
			TimeoutMS:       intValue(request.TimeoutMS, 30000),
		}
		server, err := s.mcpClients.Add(cfg)
		if err != nil {
			return nil, dynamicMCPToolError(err)
		}
		return Result{"action": action, "server": server}, nil
	case "remove":
		name := request.Name
		if err := s.mcpClients.Remove(name); err != nil {
			return nil, dynamicMCPToolError(err)
		}
		return Result{"action": action, "name": strings.TrimSpace(name), "removed": true}, nil
	case "enable", "disable":
		name := request.Name
		server, err := s.mcpClients.SetEnabled(name, action == "enable")
		if err != nil {
			return nil, dynamicMCPToolError(err)
		}
		return Result{"action": action, "server": server}, nil
	case "env_set", "env_unset", "env_list":
		name := strings.TrimSpace(request.Name)
		cfg, _, err := s.mcpClients.Inspect(name)
		if err != nil {
			return nil, dynamicMCPToolError(err)
		}
		storageKey := cfg.StorageKey
		if storageKey == "" {
			storageKey = cfg.Name
		}
		if cfg.SourceType == "plugin" && (action == "env_set" || action == "env_unset") &&
			config.IsReservedPluginEnvironmentKey(strings.TrimSpace(request.Key)) {
			return nil, toolErrorDetails(
				"VALIDATION_ERROR",
				"PLUGIN_DATA_DIR is reserved by the Plugin runtime",
				"validation",
				map[string]any{"name": cfg.Name, "plugin_name": cfg.PluginName, "key": strings.TrimSpace(request.Key)},
			)
		}
		return s.envAction(envstore.ScopeMCP, storageKey, action, request)
	case "refresh":
		name := request.Name
		server, tools, err := s.mcpClients.Refresh(ctx, name)
		if err != nil {
			return nil, dynamicMCPToolError(err)
		}
		return Result{"action": action, "server": server, "tools": tools, "tool_count": len(tools)}, nil
	case "authorize":
		authorization, err := s.mcpClients.Authorize(ctx, request.Name, request.CallbackID)
		if err != nil {
			return nil, dynamicMCPToolError(err)
		}
		result := Result{"action": action, "name": strings.TrimSpace(request.Name)}
		if authorization.AuthorizationURL != "" {
			result["authorization_url"] = authorization.AuthorizationURL
			result["callback_id"] = authorization.CallbackID
			result["expires_at"] = authorization.ExpiresAt
		}
		if len(authorization.CallbackOptions) > 0 {
			result["callback_options"] = authorization.CallbackOptions
		}
		return result, nil
	case "auth_clear":
		if err := s.mcpClients.ClearAuthorization(request.Name); err != nil {
			return nil, dynamicMCPToolError(err)
		}
		return Result{"action": action, "name": strings.TrimSpace(request.Name), "removed": true}, nil
	default:
		return nil, toolErrorDetails(
			"INVALID_ACTION",
			"unsupported mcp_manage action",
			"validation",
			map[string]any{"action": action, "allowed": []string{"list", "inspect", "add", "remove", "enable", "disable", "env_set", "env_unset", "env_list", "refresh", "authorize", "auth_clear"}},
		)
	}
}

func (s *Service) Search(ctx context.Context, request SearchRequest) (Result, error) {
	query := request.Query
	server := request.Server
	limit := boundedInt(intValue(request.Limit, 10), 10, 1, 100)
	tools, err := s.mcpClients.Search(ctx, query, server, limit)
	if err != nil {
		return nil, dynamicMCPToolError(err)
	}
	return Result{"query": query, "server": server, "tools": tools, "count": len(tools)}, nil
}

func (s *Service) Inspect(ctx context.Context, request InspectRequest) (Result, error) {
	qualifiedName := request.Name
	server, tool, err := s.mcpClients.InspectTool(ctx, qualifiedName)
	if err != nil {
		return nil, dynamicMCPToolError(err)
	}
	result := Result{
		"name":         qualifiedName,
		"server":       server,
		"tool_name":    tool.Name,
		"title":        tool.Title,
		"description":  tool.Description,
		"input_schema": tool.InputSchema,
	}
	if tool.OutputSchema != nil {
		result["output_schema"] = tool.OutputSchema
	}
	if tool.Annotations != nil {
		result["annotations"] = tool.Annotations
	}
	return result, nil
}

func (s *Service) Call(ctx context.Context, request CallRequest) (Result, error) {
	qualifiedName := request.Name
	arguments := request.Arguments
	if arguments == nil {
		arguments = map[string]any{}
	}

	result, err := s.mcpClients.CallObserved(ctx, qualifiedName, arguments, func(callCtx context.Context, server string, tool mcpclient.Tool) (context.Context, func(map[string]any, error)) {
		if s.execution == nil {
			return callCtx, nil
		}
		childCtx, child := s.execution.Begin(callCtx, execution.BeginInput{
			Tool:   server + ":" + tool.Name,
			Source: "dynamic_mcp",
		})
		return childCtx, func(result map[string]any, callErr error) {
			finish := execution.FinishInput{Status: execution.StatusCompleted}
			switch {
			case errors.Is(callErr, context.Canceled) || errors.Is(childCtx.Err(), context.Canceled):
				finish.Status = execution.StatusCancelled
				finish.ErrorCode = "CANCELED"
				finish.ErrorCategory = "runtime"
			case callErr != nil:
				finish.Status = execution.StatusFailed
				finish.ErrorCode = "MCP_ERROR"
				finish.ErrorCategory = "external"
				toolErr := dynamicMCPToolError(callErr)
				var typed *ToolError
				if errors.As(toolErr, &typed) {
					finish.ErrorCode = typed.Code
					finish.ErrorCategory = typed.Category
				}
			case mcpExecutionResultFailed(result):
				finish.Status = execution.StatusFailed
				finish.ErrorCode = "MCP_TOOL_RESULT_ERROR"
				finish.ErrorCategory = "external"
			}
			s.execution.Finish(child.ID, finish)
		}
	})
	if err != nil {
		return nil, dynamicMCPToolError(err)
	}
	return Result{"name": qualifiedName, "result": result}, nil
}

func mcpExecutionResultFailed(result map[string]any) bool {
	for _, key := range []string{"isError", "is_error"} {
		if failed, ok := result[key].(bool); ok && failed {
			return true
		}
	}
	return false
}

func dynamicMCPToolError(err error) error {
	var mcpErr *mcpclient.Error
	if !errors.As(err, &mcpErr) {
		return toolErrorCause("MCP_ERROR", err.Error(), "external", nil, err)
	}
	category := "external"
	if strings.Contains(mcpErr.Code, "INVALID") || strings.Contains(mcpErr.Code, "NOT_FOUND") ||
		strings.Contains(mcpErr.Code, "EXISTS") || strings.Contains(mcpErr.Code, "DISABLED") ||
		strings.Contains(mcpErr.Code, "REQUIRED") || strings.Contains(mcpErr.Code, "OWNED") ||
		strings.Contains(mcpErr.Code, "COLLISION") {
		category = "validation"
	}
	if strings.HasPrefix(mcpErr.Code, "MCP_AUTH_") {
		category = "auth"
	}
	toolErr := toolErrorCause(mcpErr.Code, mcpErr.Message, category, mcpErr.Details, err)
	toolErr.Retryable = mcpErr.Retryable
	return toolErr
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for name, value := range values {
		out[name] = strings.TrimSpace(value)
	}
	return out
}
