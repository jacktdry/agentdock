//go:build memory_integration

package client

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSharedMemoryHTTPIntegration(t *testing.T) {
	if os.Getenv("AGENTDOCK_MEMORY_HTTP_INTEGRATION") != "1" {
		t.Skip("set AGENTDOCK_MEMORY_HTTP_INTEGRATION=1 to test the shared Memory daemon")
	}
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	_, err = manager.Add(ServerConfig{
		Name: "memory", Description: "Shared local Memory", Transport: TransportStreamableHTTP,
		ProtocolVersion: "2025-11-25", URL: "http://127.0.0.1:8766/mcp", Enabled: true, TimeoutMS: 60000,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	summary, tools, err := manager.Refresh(ctx, "memory")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Status != "ready" || len(tools) < 20 {
		t.Fatalf("shared memory summary=%+v tool_count=%d", summary, len(tools))
	}
	byName := make(map[string]bool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = true
	}
	for _, required := range []string{"memory_store", "memory_search", "memory_list", "memory_delete", "memory_update", "memory_health", "memory_stats", "memory_graph", "memory_quality"} {
		if !byName[required] {
			t.Fatalf("shared memory missing %s", required)
		}
	}
	if byName["memory_harvest"] || byName["memory_ingest"] {
		t.Fatal("shared HTTP unexpectedly exposed host-filesystem local-only tools")
	}
	result, err := manager.Call(ctx, "memory:memory_health", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	if content, ok := result["content"].([]any); ok {
		for _, raw := range content {
			if item, ok := raw.(map[string]any); ok {
				if value, _ := item["text"].(string); value != "" {
					text += value
				}
			}
		}
	}
	if !strings.Contains(text, `"version": "11.14.0"`) || !strings.Contains(text, `"backend": "sqlite-vec"`) || !strings.Contains(text, `"status": "healthy"`) {
		t.Fatalf("unexpected shared memory health: %s", text)
	}
}
