//go:build darwin || linux

package desktopruntime

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseTailscaleFunnelStatus(t *testing.T) {
	good := []byte(`{"BackendState":"Running","Self":{"DNSName":"agentdock-mac.example.ts.net.","Online":true,"Capabilities":["funnel","https"]}}`)
	publicURL, err := parseTailscaleFunnelStatus(good)
	if err != nil {
		t.Fatalf("parseTailscaleFunnelStatus: %v", err)
	}
	if publicURL != "https://agentdock-mac.example.ts.net" {
		t.Fatalf("publicURL=%q", publicURL)
	}

	for name, payload := range map[string]string{
		"offline":        `{"BackendState":"Stopped","Self":{"DNSName":"agentdock-mac.example.ts.net.","Online":false,"Capabilities":["funnel","https"]}}`,
		"missing-funnel": `{"BackendState":"Running","Self":{"DNSName":"agentdock-mac.example.ts.net.","Online":true,"Capabilities":["https"]}}`,
		"missing-dns":    `{"BackendState":"Running","Self":{"DNSName":"","Online":true,"Capabilities":["funnel","https"]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseTailscaleFunnelStatus([]byte(payload)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestTailscaleFunnelArgs(t *testing.T) {
	got := tailscaleFunnelArgs("http://127.0.0.1:8765")
	want := []string{"funnel", "--yes", "http://127.0.0.1:8765"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tailscaleFunnelArgs=%q want=%q", got, want)
	}
}

func TestTunnelModeRecognizesTailscale(t *testing.T) {
	if got := tunnelMode(map[string]string{"AGENTDOCK_TUNNEL_MODE": "tailscale"}); got != "tailscale" {
		t.Fatalf("tunnelMode=%q", got)
	}
	if got := tunnelMode(map[string]string{"AGENTDOCK_TUNNEL_MODE": "TAILSCALE"}); !strings.EqualFold(got, "tailscale") {
		t.Fatalf("case-insensitive tunnelMode=%q", got)
	}
}
