//go:build darwin || linux

package desktopruntime

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
)

func platformReadLocalCoreAccess(_ context.Context, runtimeRoot string) (LocalCoreAccess, error) {
	_, _, values, err := readBasicEnvironment(runtimeRoot)
	if err != nil {
		return LocalCoreAccess{}, errors.New("protected Core environment could not be read")
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
			return LocalCoreAccess{}, errors.New("AgentDock Core host is not loopback")
		}
	}
	port := 8765
	if raw := strings.TrimSpace(values["AGENTDOCK_PORT"]); raw != "" {
		port, err = strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return LocalCoreAccess{}, errors.New("invalid AgentDock Core port")
		}
	}
	return LocalCoreAccess{
		MCPURL:    "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/mcp",
		AuthToken: values["AGENTDOCK_AUTH_TOKEN"],
	}, nil
}
