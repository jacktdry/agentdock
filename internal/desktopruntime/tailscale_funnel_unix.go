//go:build darwin || linux

package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type tailscaleStatusPayload struct {
	BackendState string `json:"BackendState"`
	Self         struct {
		DNSName      string   `json:"DNSName"`
		Online       bool     `json:"Online"`
		Capabilities []string `json:"Capabilities"`
	} `json:"Self"`
}

func parseTailscaleFunnelStatus(data []byte) (string, error) {
	var status tailscaleStatusPayload
	if err := json.Unmarshal(data, &status); err != nil {
		return "", errors.New("无法读取 Tailscale 状态")
	}
	if !strings.EqualFold(strings.TrimSpace(status.BackendState), "Running") || !status.Self.Online {
		return "", errors.New("Tailscale 当前未连接")
	}
	capabilities := make(map[string]bool, len(status.Self.Capabilities))
	for _, capability := range status.Self.Capabilities {
		capabilities[strings.ToLower(strings.TrimSpace(capability))] = true
	}
	if !capabilities["funnel"] || !capabilities["https"] {
		return "", errors.New("当前设备或 tailnet 未启用 Tailscale Funnel")
	}
	host := strings.Trim(strings.TrimSpace(status.Self.DNSName), ".")
	if host == "" || !strings.Contains(host, ".") {
		return "", errors.New("无法确定 Tailscale DNS 名称")
	}
	return (&url.URL{Scheme: "https", Host: host}).String(), nil
}

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

func tailscaleFunnelArgs(target string) []string {
	return []string{"funnel", "--yes", target}
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
