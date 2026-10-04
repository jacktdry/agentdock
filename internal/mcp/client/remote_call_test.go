package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCallRemoteToolDoesNotRequireRegistry(t *testing.T) {
	const token = "desktop-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("authorization=%q", r.Header.Get("Authorization"))
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var request struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request.Method {
		case "server/discover":
			w.Header().Set("Content-Type", "application/json")
			writeRPCResult(t, w, request.ID, map[string]any{"protocolVersions": []string{"2026-07-28"}})
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "desktop-session")
			writeRPCResult(t, w, request.ID, map[string]any{"protocolVersion": "2026-07-28", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "agentdock", "version": "test"}})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			w.Header().Set("Content-Type", "application/json")
			writeRPCResult(t, w, request.ID, map[string]any{"content": []any{}, "structuredContent": map[string]any{"ok": true}})
		default:
			t.Errorf("unexpected method=%s", request.Method)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	result, err := CallRemoteTool(context.Background(), ServerConfig{
		Name: "desktop-core", Description: "desktop", Transport: TransportStreamableHTTP, URL: server.URL,
		StaticHeaders: map[string]string{"Authorization": "Bearer " + token}, Enabled: true, TimeoutMS: 30000,
	}, "acp_session", map[string]any{"action": "status"})
	if err != nil {
		t.Fatal(err)
	}
	structured, _ := result["structuredContent"].(map[string]any)
	if structured["ok"] != true {
		t.Fatalf("result=%#v", result)
	}
}
