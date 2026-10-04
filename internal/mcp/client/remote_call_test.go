package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallRemoteToolDoesNotRequireRegistry(t *testing.T) {
	const token = "desktop-secret"
	var deleted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("authorization=%q", r.Header.Get("Authorization"))
		}
		if r.Method == http.MethodDelete {
			deleted.Add(1)
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
	if deleted.Load() != 1 {
		t.Fatal("MCP session was not closed exactly once")
	}
}

func TestCallRemoteToolDoesNotForwardCredentialsOnRedirect(t *testing.T) {
	var forwarded atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1); w.WriteHeader(500) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := CallRemoteTool(context.Background(), ServerConfig{URL: server.URL, TimeoutMS: 100, StaticHeaders: map[string]string{"Authorization": "Bearer private-token"}}, "status", nil)
	if err == nil || forwarded.Load() != 0 {
		t.Fatalf("redirect followed: requests=%d err=%v", forwarded.Load(), err)
	}
}

func TestCallRemoteToolTimeoutCancellationAndSafeErrors(t *testing.T) {
	for _, mode := range []string{"initialize-timeout", "call-timeout", "cancel", "remote-error", "close-timeout"} {
		t.Run(mode, func(t *testing.T) {
			var deleted atomic.Int32
			entered := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deleted.Add(1)
					if mode == "close-timeout" {
						<-r.Context().Done()
						return
					}
					w.WriteHeader(http.StatusNoContent)
					return
				}
				var request struct {
					ID     any    `json:"id"`
					Method string `json:"method"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch request.Method {
				case "initialize":
					if mode == "initialize-timeout" {
						<-r.Context().Done()
						return
					}
					w.Header().Set("Mcp-Session-Id", "one-shot")
					writeRPCResult(t, w, request.ID, map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "test", "version": "1"}})
				case "notifications/initialized":
					w.WriteHeader(http.StatusAccepted)
				case "tools/call":
					if mode == "remote-error" {
						_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32603, "message": "Bearer private-token", "data": "private-payload"}})
						return
					}
					if mode == "call-timeout" || mode == "cancel" {
						entered <- struct{}{}
						<-r.Context().Done()
						return
					}
					writeRPCResult(t, w, request.ID, map[string]any{"content": []any{}})
				default:
					t.Errorf("unexpected method: %s", request.Method)
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				go func() {
					select {
					case <-entered:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			start := time.Now()
			_, err := CallRemoteTool(ctx, ServerConfig{URL: server.URL, ProtocolVersion: "2025-11-25", TimeoutMS: 100, StaticHeaders: map[string]string{"Authorization": "Bearer private-token"}}, "status", nil)
			if time.Since(start) > time.Second {
				t.Fatal("one-shot call exceeded timeout budget")
			}
			if err == nil {
				t.Fatal("failure was reported as success")
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
			if (mode == "initialize-timeout" || mode == "call-timeout") && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("timeout lost: %v", err)
			}
			if mode != "initialize-timeout" && deleted.Load() != 1 {
				t.Fatal("established session was not closed")
			}
			for _, text := range []string{err.Error(), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err), fmt.Sprint(errors.Unwrap(err))} {
				if strings.Contains(text, "private-token") || strings.Contains(text, "private-payload") {
					t.Fatal("remote error leaked secrets")
				}
			}
		})
	}
}
