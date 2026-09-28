//go:build darwin || linux

package desktopruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func tailscaleExecutable(configured string) (string, error) {
	candidates := []string{}
	if configured = strings.TrimSpace(configured); configured != "" {
		candidates = append(candidates, configured)
	}
	if path, err := exec.LookPath("tailscale"); err == nil {
		candidates = append(candidates, path)
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
			"/opt/homebrew/bin/tailscale",
			"/usr/local/bin/tailscale",
		)
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if candidate == "." || seen[candidate] {
			continue
		}
		seen[candidate] = true
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", errors.New("未找到可执行的 Tailscale CLI")
}

func resolveTailscaleFunnelInfo(ctx context.Context, configured string) (string, string, error) {
	binary, err := tailscaleExecutable(configured)
	if err != nil {
		return "", "", err
	}
	output, err := runCommand(ctx, binary, "status", "--json")
	if err != nil {
		return "", "", err
	}
	publicURL, err := parseTailscaleFunnelStatus([]byte(output))
	if err != nil {
		return "", "", err
	}
	return binary, publicURL, nil
}

func runTailscaleFunnel(ctx context.Context, values map[string]string, stdout, stderr io.Writer) error {
	target := strings.TrimSpace(values["AGENTDOCK_TUNNEL_TARGET"])
	if target == "" {
		return errors.New("Tailscale Funnel 缺少目标地址")
	}
	binary, err := tailscaleExecutable(values["AGENTDOCK_TAILSCALE_BIN"])
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, binary, tailscaleFunnelArgs(target)...)
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}
