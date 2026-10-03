//go:build darwin || linux

package desktopruntime

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func platformReadCoreConnection(runtimeRoot string) (CoreConnection, error) {
	_, _, values, err := readBasicEnvironment(runtimeRoot)
	if err != nil {
		return CoreConnection{}, err
	}
	host := strings.TrimSpace(values["AGENTDOCK_HOST"])
	switch host {
	case "", "0.0.0.0", "127.0.0.1", "localhost":
		host = "127.0.0.1"
	case "::", "[::]", "::1", "[::1]":
		host = "::1"
	default:
		return CoreConnection{}, fmt.Errorf("core host must include loopback: %s", host)
	}
	port := 8765
	if raw := strings.TrimSpace(values["AGENTDOCK_PORT"]); raw != "" {
		port, err = strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return CoreConnection{}, ErrBasicSettingsInvalid
		}
	}
	return CoreConnection{
		endpoint:  "http://" + net.JoinHostPort(host, strconv.Itoa(port)),
		authToken: strings.TrimSpace(values["AGENTDOCK_AUTH_TOKEN"]),
	}, nil
}
