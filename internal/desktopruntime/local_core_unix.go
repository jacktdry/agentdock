//go:build darwin || linux

package desktopruntime

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
)

func platformReadLocalCoreAccess(_ context.Context, runtimeRoot string) (LocalCoreAccess, error) {
	_, _, values, err := loadCoreEnvironment(runtimeRoot)
	if err != nil {
		return LocalCoreAccess{}, err
	}
	host := strings.TrimSpace(values["AGENTDOCK_HOST"])
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	if strings.EqualFold(host, "localhost") {
		host = "localhost"
	} else {
		host = strings.Trim(host, "[]")
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return LocalCoreAccess{}, fmt.Errorf("AgentDock Core host is not loopback: %q", host)
		}
	}
	port := 8765
	if raw := strings.TrimSpace(values["AGENTDOCK_PORT"]); raw != "" {
		port, err = strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return LocalCoreAccess{}, fmt.Errorf("invalid AgentDock Core port %q", raw)
		}
	}
	return LocalCoreAccess{
		MCPURL:    "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/mcp",
		AuthToken: values["AGENTDOCK_AUTH_TOKEN"],
	}, nil
}
