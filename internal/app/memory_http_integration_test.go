//go:build memory_integration

package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/config"
	toolmcp "github.com/uvwt/agentdock/internal/tool/mcp"
)

func TestRuntimeUsesSharedMemoryHTTPIntegration(t *testing.T) {
	if os.Getenv("AGENTDOCK_MEMORY_HTTP_INTEGRATION") != "1" {
		t.Skip("set AGENTDOCK_MEMORY_HTTP_INTEGRATION=1 to test AgentDock Runtime shared Memory")
	}
	home := t.TempDir()
	workspace := t.TempDir()
	cfg := config.Config{AgentDockHome: home, AgentDockDefaultDir: workspace, Host: "127.0.0.1", Port: 0}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rt.Close() }()

	enabled := true
	timeoutMS := 60_000
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := rt.dynamicMCP.Manage(ctx, toolmcp.ManageRequest{
		Action: "add", Name: "memory", Description: "Shared local Memory",
		Transport: "streamable_http", ProtocolVersion: "2025-11-25", URL: "http://127.0.0.1:8766/mcp",
		Enabled: &enabled, TimeoutMS: &timeoutMS,
	}); err != nil {
		t.Fatal(err)
	}
	refreshed, err := rt.dynamicMCP.Manage(ctx, toolmcp.ManageRequest{Action: "refresh", Name: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	if count, _ := refreshed["tool_count"].(int); count < 20 {
		t.Fatalf("shared Memory tool_count=%d, want >=20", count)
	}
	called, err := rt.dynamicMCP.Call(ctx, toolmcp.CallRequest{Name: "memory:memory_health", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(called["result"])
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`11.14.0`, `sqlite-vec`, `healthy`} {
		if !strings.Contains(text, want) {
			t.Fatalf("shared Memory health missing %q: %s", want, text)
		}
	}

	registryPath := filepath.Join(home, "mcp", "servers.json")
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Servers []struct {
			Name            string `json:"name"`
			Transport       string `json:"transport"`
			ProtocolVersion string `json:"protocol_version"`
			URL             string `json:"url"`
			Command         string `json:"command"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, server := range registry.Servers {
		if server.Name != "memory" {
			continue
		}
		found = true
		if server.Transport != "streamable_http" || server.ProtocolVersion != "2025-11-25" || server.URL != "http://127.0.0.1:8766/mcp" || server.Command != "" {
			t.Fatalf("persisted Memory config=%+v", server)
		}
	}
	if !found {
		t.Fatal("persisted dynamic MCP registry missing memory")
	}
}
