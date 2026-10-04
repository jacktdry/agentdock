package client

import (
	"context"
	"fmt"
	"strings"
)

// CallRemoteTool performs one Streamable HTTP MCP initialize/call/close cycle
// without registering or persisting the server in AgentDock's dynamic MCP
// registry. It is intended for privileged local control-plane clients such as
// Shared Desktop. Credentials remain only in cfg and are never returned.
func CallRemoteTool(ctx context.Context, cfg ServerConfig, toolName string, arguments map[string]any) (map[string]any, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg = normalizeServerConfig(cfg)
	if cfg.Name == "" {
		cfg.Name = "remote"
	}
	if cfg.Description == "" {
		cfg.Description = "One-shot remote MCP server"
	}
	if cfg.Transport == "" {
		cfg.Transport = TransportStreamableHTTP
	}
	if cfg.TimeoutMS == 0 {
		cfg.TimeoutMS = defaultTimeoutMS
	}
	if cfg.Transport != TransportStreamableHTTP {
		return nil, fmt.Errorf("one-shot remote MCP calls require streamable_http")
	}
	if err := validateServerConfig(cfg); err != nil {
		return nil, err
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil, fmt.Errorf("tool name is required")
	}
	client := newStreamableHTTPClient(cfg, nil)
	if err := client.initialize(ctx); err != nil {
		_ = client.close()
		return nil, err
	}
	defer client.close()
	return client.callTool(ctx, toolName, arguments)
}
