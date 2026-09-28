package desktopruntime

import (
	"encoding/json"
	"errors"
	"net/url"
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

func tailscaleFunnelArgs(target string) []string {
	return []string{"funnel", "--yes", target}
}

func tailscaleStatusReady(running bool, configuredOrigin, currentOrigin string, statusErr error) bool {
	return running && statusErr == nil && configuredOrigin != "" && strings.EqualFold(configuredOrigin, currentOrigin)
}
