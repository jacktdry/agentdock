//go:build darwin

package desktopruntime

import (
	"context"
	"net"
	"strconv"
	"strings"
)

func platformSelectNextConnectionRuntime(ctx context.Context, root string) (NextConnectionRuntime, error) {
	values, err := nextCoreEnvironment(root)
	if err != nil {
		return NextConnectionRuntime{}, ErrNextIdentityUnavailable
	}
	manifest, err := explicitNextDarwinRuntime(root)
	if err != nil {
		return NextConnectionRuntime{}, ErrNextIdentityUnavailable
	}
	selection := NextPortRuntime{Variant: "next", Root: root, Binary: manifest.AgentDockBinary}
	if !darwinPortSelection(selection) {
		return NextConnectionRuntime{}, ErrNextIdentityUnavailable
	}
	host := strings.Trim(values["AGENTDOCK_HOST"], "[]")
	if host == "" {
		host = "0.0.0.0"
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	if net.ParseIP(host) == nil {
		return NextConnectionRuntime{}, ErrNextIdentityUnavailable
	}
	result := NextConnectionRuntime{PortRuntime: selection, BindHost: host, Health: "unknown"}
	// Only the explicitly selected Next service is queried. A PID from lsof
	// would adopt a candidate-port owner and must never seed this selection.
	output, err := runCommand(ctx, launchctlBinary(), "print", launchdTarget("dev.dropabit.agentdock.next.core"))
	if err != nil {
		return result, nil
	}
	running := launchdJobRunning(output)
	result.Running = &running
	if !running {
		return result, nil
	}
	pid := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "pid = ") {
			if pid != 0 {
				return result, nil
			}
			pid, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "pid = ")))
		}
	}
	binary, instance, err := platformPortProcess(pid)
	if err == nil && portBinaryMatches(binary, selection.Binary) && instance != "" {
		result.PortRuntime.PID, result.PortRuntime.ProcessInstance = pid, instance
	}
	return result, nil
}

func platformValidateNextSettingsIdentity(ctx context.Context, root string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if _, err := nextCoreEnvironment(root); err != nil {
		return ErrNextIdentityUnavailable
	}
	_, err := explicitNextDarwinRuntime(root)
	return err
}
