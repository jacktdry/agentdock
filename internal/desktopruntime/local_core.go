package desktopruntime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type LocalCoreAccess struct {
	MCPURL    string
	AuthToken string
}

func ReadLocalCoreAccess(ctx context.Context, runtimeRoot string) (LocalCoreAccess, error) {
	if ctx == nil {
		return LocalCoreAccess{}, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return LocalCoreAccess{}, err
	}
	access, err := platformReadLocalCoreAccess(ctx, runtimeRoot)
	if err != nil {
		return LocalCoreAccess{}, err
	}
	validated, err := validateLocalMCPURL(access.MCPURL)
	if err != nil {
		return LocalCoreAccess{}, err
	}
	access.MCPURL = validated
	access.AuthToken = strings.TrimSpace(access.AuthToken)
	return access, nil
}

func validateLocalMCPURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" {
		return "", fmt.Errorf("local MCP URL must be absolute loopback HTTP: %q", raw)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.EscapedPath() != "/mcp" {
		return "", fmt.Errorf("local MCP URL must contain only the /mcp path: %q", raw)
	}
	host := parsed.Hostname()
	if !strings.EqualFold(host, "localhost") {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return "", fmt.Errorf("local MCP URL must use a loopback host: %q", raw)
		}
	}
	if parsed.Port() == "" {
		return "", fmt.Errorf("local MCP URL must include a port: %q", raw)
	}
	return parsed.String(), nil
}
