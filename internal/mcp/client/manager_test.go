package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/envstore"
)

func TestManagerStreamableHTTPFlowAndPersistence(t *testing.T) {
	t.Setenv("TEST_MCP_AUTH", "Bearer test-token")
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
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
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch request.Method {
		case "server/discover":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "Method not found"},
			})
		case "initialize":
			if got := r.Header.Get("MCP-Protocol-Version"); got != "" {
				t.Errorf("initialize protocol header = %q, want empty before negotiation", got)
			}
			if got := request.Params["protocolVersion"]; got != "2025-11-25" {
				t.Errorf("initialize protocol version = %#v", got)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "session-1")
			writeRPCResult(t, w, request.ID, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "test", "version": "1.0.0"},
			})
		case "notifications/initialized":
			if got := r.Header.Get("Mcp-Session-Id"); got != "session-1" {
				t.Errorf("notification session id = %q", got)
			}
			if got := r.Header.Get("MCP-Protocol-Version"); got != "2024-11-05" {
				t.Errorf("notification negotiated protocol version = %q", got)
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if got := r.Header.Get("Mcp-Session-Id"); got != "session-1" {
				t.Errorf("tools/list session id = %q", got)
			}
			if got := r.Header.Get("MCP-Protocol-Version"); got != "2024-11-05" {
				t.Errorf("tools/list negotiated protocol version = %q", got)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			payload, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{"tools": []map[string]any{
					{
						"name":        "echo",
						"title":       "Echo",
						"description": "Echo supplied text",
						"inputSchema": map[string]any{
							"type":       "object",
							"required":   []string{"text"},
							"properties": map[string]any{"text": map[string]any{"type": "string"}},
						},
					},
				}},
			})
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload)
		case "tools/call":
			callCount.Add(1)
			arguments, _ := request.Params["arguments"].(map[string]any)
			writeRPCResult(t, w, request.ID, map[string]any{
				"content":           []map[string]any{{"type": "text", "text": arguments["text"]}},
				"structuredContent": map[string]any{"echo": arguments["text"]},
			})
		default:
			t.Errorf("unexpected method %q", request.Method)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	home := t.TempDir()
	manager, err := NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	added, err := manager.Add(ServerConfig{
		Name:        "demo",
		Description: "Demo MCP server",
		Transport:   TransportStreamableHTTP,
		URL:         server.URL,
		HeaderEnv:   map[string]string{"Authorization": "TEST_MCP_AUTH"},
		Enabled:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if added.Status != "idle" {
		t.Fatalf("added status = %q", added.Status)
	}

	refreshed, tools, err := manager.Refresh(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Status != "ready" || refreshed.ToolCount != 1 || len(tools) != 1 {
		t.Fatalf("unexpected refresh: summary=%#v tools=%#v", refreshed, tools)
	}

	matches, err := manager.Search(context.Background(), "echo text", "demo", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].QualifiedName != "demo:echo" {
		t.Fatalf("unexpected search matches: %#v", matches)
	}
	allTools, err := manager.Search(context.Background(), "*", "demo", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(allTools) != 1 || allTools[0].QualifiedName != "demo:echo" {
		t.Fatalf("unexpected wildcard search matches: %#v", allTools)
	}

	_, tool, err := manager.InspectTool(context.Background(), "demo:echo")
	if err != nil {
		t.Fatal(err)
	}
	if tool.Name != "echo" || tool.InputSchema["type"] != "object" {
		t.Fatalf("unexpected inspected tool: %#v", tool)
	}

	if _, err := manager.Call(context.Background(), "demo:echo", map[string]any{}); err == nil {
		t.Fatal("missing required argument was accepted")
	} else {
		var mcpErr *Error
		if !errors.As(err, &mcpErr) || mcpErr.Code != "MCP_ARGUMENT_INVALID" || !strings.Contains(fmt.Sprint(mcpErr.Details["reason"]), "$.text is required") {
			t.Fatalf("unexpected missing argument error: %#v", err)
		}
	}
	if _, err := manager.Call(context.Background(), "demo:echo", map[string]any{"text": 42}); err == nil {
		t.Fatal("wrong argument type was accepted")
	} else {
		var mcpErr *Error
		if !errors.As(err, &mcpErr) || mcpErr.Code != "MCP_ARGUMENT_INVALID" || !strings.Contains(fmt.Sprint(mcpErr.Details["reason"]), "$.text must be string") {
			t.Fatalf("unexpected argument type error: %#v", err)
		}
	}
	result, err := manager.Call(context.Background(), "demo:echo", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if callCount.Load() != 1 {
		t.Fatalf("tools/call count = %d", callCount.Load())
	}
	structured, _ := result["structuredContent"].(map[string]any)
	if structured["echo"] != "hello" {
		t.Fatalf("unexpected call result: %#v", result)
	}

	reloaded, err := NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	listed := reloaded.List()
	if len(listed) != 1 || listed[0].Name != "demo" || listed[0].Description != "Demo MCP server" {
		t.Fatalf("persisted registry mismatch: %#v", listed)
	}
}

func TestManagerStdioFlow(t *testing.T) {
	t.Setenv("TEST_MCP_HELPER_MODE", "1")
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	_, err = manager.Add(ServerConfig{
		Name:        "local",
		Description: "Local stdio MCP",
		Transport:   TransportStdio,
		Command:     os.Args[0],
		Args:        []string{"-test.run=TestMCPStdioHelperProcess"},
		EnvFromEnv:  map[string]string{"GO_WANT_MCP_HELPER": "TEST_MCP_HELPER_MODE"},
		Enabled:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Refresh(context.Background(), "local"); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Call(context.Background(), "local:echo", map[string]any{"text": "stdio"})
	if err != nil {
		t.Fatal(err)
	}
	structured, _ := result["structuredContent"].(map[string]any)
	if structured["echo"] != "stdio" {
		t.Fatalf("unexpected stdio result: %#v", result)
	}
	if err := manager.Remove("local"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
}

func TestManagerRejectsReservedHTTPHeaders(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	_, err = manager.Add(ServerConfig{
		Name:        "unsafe",
		Description: "Invalid header override",
		Transport:   TransportStreamableHTTP,
		URL:         "https://example.invalid/mcp",
		HeaderEnv:   map[string]string{"MCP-Protocol-Version": "TEST_MCP_VERSION"},
		Enabled:     true,
	})
	if err == nil {
		t.Fatal("reserved MCP protocol header override was accepted")
	}
	var mcpErr *Error
	if !errors.As(err, &mcpErr) || mcpErr.Code != "MCP_CONFIG_INVALID" {
		t.Fatalf("unexpected reserved header error: %#v", err)
	}
}

func TestMCPStdioHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(2)
		}
		switch request.Method {
		case "server/discover":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32601, "message": "Method not found"},
			})
		case "initialize":
			result := map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{"tools": map[string]any{}},
			}
			if os.Getenv("MCP_HELPER_OMIT_SERVER_INFO") != "1" {
				result["serverInfo"] = map[string]any{"name": "helper", "version": "1.0.0"}
			}
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result":  result,
			})
		case "notifications/initialized":
		case "tools/list":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{"tools": []map[string]any{{
					"name":        "echo",
					"description": "Echo text over stdio",
					"inputSchema": map[string]any{"type": "object", "required": []string{"text"}},
				}}},
			})
		case "tools/call":
			arguments, _ := request.Params["arguments"].(map[string]any)
			echo := arguments["text"]
			if envName, ok := arguments["readEnv"].(string); ok && envName != "" {
				echo = os.Getenv(envName)
			}
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{
					"content":           []map[string]any{{"type": "text", "text": echo}},
					"structuredContent": map[string]any{"echo": echo},
				},
			})
		}
	}
	if marker := os.Getenv("MCP_HELPER_EOF_MARKER"); marker != "" {
		if err := os.WriteFile(marker, []byte("EOF"), 0600); err != nil {
			os.Exit(3)
		}
		switch os.Getenv("MCP_HELPER_CLOSE_MODE") {
		case "graceful":
			time.Sleep(200 * time.Millisecond)
			if err := os.Remove(os.Getenv("MCP_HELPER_OWNED_PROFILE")); err != nil {
				os.Exit(4)
			}
		case "stubborn":
			time.Sleep(time.Minute)
		}
	}
	os.Exit(0)
}

func writeRPCResult(t *testing.T, w http.ResponseWriter, id any, result any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result}); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func TestIndependentManagersDoNotOverwriteRegistryUpdates(t *testing.T) {
	home := t.TempDir()
	first, err := NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	configs := []struct {
		manager *Manager
		config  ServerConfig
	}{
		{manager: first, config: ServerConfig{Name: "first", Description: "First server", Transport: TransportStreamableHTTP, URL: "https://example.invalid/first", Enabled: true}},
		{manager: second, config: ServerConfig{Name: "second", Description: "Second server", Transport: TransportStreamableHTTP, URL: "https://example.invalid/second", Enabled: true}},
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(configs))
	for _, item := range configs {
		wg.Add(1)
		go func(item struct {
			manager *Manager
			config  ServerConfig
		}) {
			defer wg.Done()
			_, err := item.manager.Add(item.config)
			errs <- err
		}(item)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
	}

	visible := first.List()
	if len(visible) != 2 || visible[0].Name != "first" || visible[1].Name != "second" {
		t.Fatalf("cross-process visible registry = %#v", visible)
	}

	reloaded, err := NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	listed := reloaded.List()
	if len(listed) != 2 || listed[0].Name != "first" || listed[1].Name != "second" {
		t.Fatalf("persisted registry = %#v, want both independent updates", listed)
	}
}

func TestSameManagerConcurrentRegistryUpdatesRemainVisible(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	configs := []ServerConfig{
		{Name: "alpha", Description: "Alpha", Transport: TransportStreamableHTTP, URL: "https://example.invalid/alpha", Enabled: true},
		{Name: "beta", Description: "Beta", Transport: TransportStreamableHTTP, URL: "https://example.invalid/beta", Enabled: true},
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(configs))
	for _, cfg := range configs {
		wg.Add(1)
		go func(cfg ServerConfig) {
			defer wg.Done()
			_, err := manager.Add(cfg)
			errs <- err
		}(cfg)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
	}
	listed := manager.List()
	if len(listed) != 2 || listed[0].Name != "alpha" || listed[1].Name != "beta" {
		t.Fatalf("manager registry = %#v", listed)
	}
}

func TestManagerStatePinBlocksDisableUntilOperationFinishes(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if _, err := manager.Add(ServerConfig{
		Name:        "demo",
		Description: "Demo",
		Transport:   TransportStreamableHTTP,
		URL:         "https://example.invalid/mcp",
		Enabled:     true,
	}); err != nil {
		t.Fatal(err)
	}

	_, _, unlockState, err := manager.lockServer("demo")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := manager.SetEnabled("demo", false)
		done <- err
	}()
	select {
	case err := <-done:
		unlockState()
		t.Fatalf("SetEnabled() completed while state was pinned: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	unlockState()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SetEnabled() remained blocked after state release")
	}
}

func TestManagerRejectsOperationsAfterClose(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if _, err := manager.Add(ServerConfig{
		Name:        "demo",
		Description: "Demo",
		Transport:   TransportStreamableHTTP,
		URL:         "https://example.invalid/mcp",
		Enabled:     true,
	}); err == nil {
		t.Fatal("Add() succeeded after Close()")
	}
	if _, _, err := manager.Refresh(context.Background(), "demo"); err == nil {
		t.Fatal("Refresh() succeeded after Close()")
	}
}

func TestLockServerRejectsCloseRace(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Add(ServerConfig{
		Name:        "demo",
		Description: "Demo",
		Transport:   TransportStreamableHTTP,
		URL:         "https://example.invalid/mcp",
		Enabled:     true,
	}); err != nil {
		t.Fatal(err)
	}
	manager.mu.RLock()
	state := manager.states["demo"]
	manager.mu.RUnlock()
	state.mu.Lock()
	lockResult := make(chan error, 1)
	go func() {
		_, _, unlock, err := manager.lockServer("demo")
		if unlock != nil {
			unlock()
		}
		lockResult <- err
	}()
	time.Sleep(20 * time.Millisecond)
	closeResult := make(chan error, 1)
	go func() { closeResult <- manager.Close() }()
	time.Sleep(20 * time.Millisecond)
	state.mu.Unlock()
	if err := <-closeResult; err != nil {
		t.Fatal(err)
	}
	err = <-lockResult
	var mcpErr *Error
	if !errors.As(err, &mcpErr) || mcpErr.Code != "MCP_MANAGER_CLOSED" {
		t.Fatalf("lockServer() error = %#v, want MCP_MANAGER_CLOSED", err)
	}
}

func TestPluginRuntimeConfigDistinguishesRequiredAndOptionalHeaderBindings(t *testing.T) {
	home := t.TempDir()
	envs, err := envstore.New(home)
	if err != nil {
		t.Fatal(err)
	}
	const storageKey = "plugin.demo.plugin.remote"
	scope := envstore.Scope{Kind: envstore.ScopeMCP, Name: storageKey}
	if err := envs.Set(scope, "REQUIRED_TOKEN", "required-value"); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(home, envs)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	cfg := ServerConfig{
		Name: storageKey, Description: "Plugin remote MCP", Transport: TransportStreamableHTTP,
		URL: "https://example.com/mcp",
		HeaderEnv: map[string]string{
			"Authorization": "OPTIONAL_TOKEN",
			"X-Required":    "REQUIRED_TOKEN",
		},
		EnvBindings: map[string]string{"OPTIONAL_CHILD": "OPTIONAL_CHILD_TOKEN"},
		RequiredEnv: []string{"REQUIRED_TOKEN"},
		StorageKey:  storageKey, SourceType: "plugin", PluginName: "demo.plugin",
		PluginDataDir: t.TempDir(), Enabled: true,
	}
	runtimeCfg, err := manager.runtimeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if value, exists := runtimeCfg.RuntimeEnv["OPTIONAL_TOKEN"]; !exists || value != "" {
		t.Fatalf("optional header binding = %#v, want OPTIONAL_TOKEN empty", runtimeCfg.RuntimeEnv)
	}
	if value, exists := runtimeCfg.RuntimeEnv["OPTIONAL_CHILD"]; !exists || value != "" {
		t.Fatalf("optional stdio binding = %#v, want OPTIONAL_CHILD empty", runtimeCfg.RuntimeEnv)
	}
	if runtimeCfg.RuntimeEnv["REQUIRED_TOKEN"] != "required-value" {
		t.Fatalf("required runtime binding = %#v", runtimeCfg.RuntimeEnv)
	}

	if _, err := envs.Unset(scope, "REQUIRED_TOKEN"); err != nil {
		t.Fatal(err)
	}
	_, err = manager.runtimeConfig(cfg)
	var mcpErr *Error
	if !errors.As(err, &mcpErr) || mcpErr.Code != "MCP_CREDENTIAL_REQUIRED" {
		t.Fatalf("runtimeConfig() error = %#v, want MCP_CREDENTIAL_REQUIRED", err)
	}
}

func TestPluginOwnedStdioUsesStableEnvAndRuntimeProvenance(t *testing.T) {
	home := t.TempDir()
	envs, err := envstore.New(home)
	if err != nil {
		t.Fatal(err)
	}
	const storageKey = "plugin.demo.plugin.local"
	if err := envs.Set(envstore.Scope{Kind: envstore.ScopeMCP, Name: storageKey}, "DEMO_TOKEN", "stored-secret"); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(home, envs)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	pluginRuntimeRoot := t.TempDir()
	pluginData := t.TempDir()
	runtimeName := "plugin.demo.plugin.local"
	if err := manager.SetOwnedServers([]ServerConfig{{
		Name: runtimeName, DisplayName: "local", Description: "Plugin stdio MCP",
		Transport: TransportStdio, Command: os.Args[0],
		Args:        []string{"-test.run=TestMCPStdioHelperProcess"},
		StaticEnv:   map[string]string{"GO_WANT_MCP_HELPER": "1", "MODE": "package-default"},
		EnvBindings: map[string]string{"TOKEN": "DEMO_TOKEN"},
		StorageKey:  storageKey, SourceType: "plugin", PluginName: "demo.plugin",
		PluginRuntimeRoot: pluginRuntimeRoot, PluginDataDir: pluginData, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}

	refreshed, tools, err := manager.Refresh(context.Background(), runtimeName)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.SourceType != "plugin" || refreshed.PluginName != "demo.plugin" ||
		refreshed.DisplayName != "local" || len(tools) != 1 {
		t.Fatalf("Plugin MCP refresh = %#v tools=%#v", refreshed, tools)
	}
	if tools[0].SourceType != "plugin" || tools[0].PluginName != "demo.plugin" {
		t.Fatalf("Plugin MCP tool provenance = %#v", tools[0])
	}

	readEnv := func(name string) string {
		t.Helper()
		result, err := manager.Call(context.Background(), runtimeName+":echo", map[string]any{
			"text": "unused", "readEnv": name,
		})
		if err != nil {
			t.Fatal(err)
		}
		structured, _ := result["structuredContent"].(map[string]any)
		value, _ := structured["echo"].(string)
		return value
	}
	if got := readEnv("TOKEN"); got != "stored-secret" {
		t.Fatalf("Plugin MCP env binding TOKEN = %q", got)
	}
	if got := readEnv("MODE"); got != "package-default" {
		t.Fatalf("Plugin MCP package default MODE = %q", got)
	}
	if got := readEnv("PLUGIN_ROOT"); got != "" {
		t.Fatalf("internal PLUGIN_ROOT placeholder leaked into child environment: %q", got)
	}
	if got := readEnv("PLUGIN_DATA"); got != "" {
		t.Fatalf("internal PLUGIN_DATA placeholder leaked into child environment: %q", got)
	}
	if got := readEnv("PLUGIN_DATA_DIR"); got != pluginData {
		t.Fatalf("PLUGIN_DATA_DIR = %q, want %q", got, pluginData)
	}

	searched, err := manager.Search(context.Background(), "echo", runtimeName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(searched) != 1 || searched[0].SourceType != "plugin" || searched[0].PluginName != "demo.plugin" {
		t.Fatalf("Plugin MCP search provenance = %#v", searched)
	}

	reloaded, err := NewManager(home, envs)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	for _, item := range reloaded.List() {
		if item.Name == runtimeName {
			t.Fatalf("Plugin-owned MCP leaked into standalone servers.json: %#v", item)
		}
	}
}

func TestManagerStreamableHTTPProtocolPinSkipsDiscover(t *testing.T) {
	const pinned = "2025-11-25"
	var discoverCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch request.Method {
		case "server/discover":
			discoverCalls.Add(1)
			http.Error(w, "Bad Request: Unsupported protocol version: 2026-07-28. Supported versions: 2025-11-25", http.StatusBadRequest)
		case "initialize":
			if got := request.Params["protocolVersion"]; got != pinned {
				t.Errorf("initialize protocol version = %#v, want %q", got, pinned)
			}
			if got := r.Header.Get("MCP-Protocol-Version"); got != "" {
				t.Errorf("initialize protocol header = %q, want empty", got)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "legacy-session")
			writeRPCResult(t, w, request.ID, map[string]any{
				"protocolVersion": pinned,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "legacy", "version": "1.0.0"},
			})
		case "notifications/initialized":
			if got := r.Header.Get("MCP-Protocol-Version"); got != pinned {
				t.Errorf("initialized protocol header = %q, want %q", got, pinned)
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if got := r.Header.Get("MCP-Protocol-Version"); got != pinned {
				t.Errorf("tools/list protocol header = %q, want %q", got, pinned)
			}
			w.Header().Set("Content-Type", "application/json")
			writeRPCResult(t, w, request.ID, map[string]any{"tools": []map[string]any{{
				"name": "health", "description": "health", "inputSchema": map[string]any{"type": "object"},
			}}})
		default:
			t.Errorf("unexpected method %q", request.Method)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	home := t.TempDir()
	manager, err := NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	_, err = manager.Add(ServerConfig{
		Name: "legacy", Description: "Legacy MCP", Transport: TransportStreamableHTTP,
		ProtocolVersion: pinned, URL: server.URL, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	refreshed, tools, err := manager.Refresh(context.Background(), "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if discoverCalls.Load() != 0 {
		t.Fatalf("pinned legacy server received %d server/discover calls", discoverCalls.Load())
	}
	if refreshed.Status != "ready" || refreshed.ToolCount != 1 || len(tools) != 1 || tools[0].Name != "health" {
		t.Fatalf("refresh=%#v tools=%#v", refreshed, tools)
	}
	cfg, _, err := manager.Inspect("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProtocolVersion != pinned {
		t.Fatalf("persisted protocol_version=%q", cfg.ProtocolVersion)
	}
}

type failingCloseProtocolClient struct {
	closeCalls atomic.Int32
}

func (c *failingCloseProtocolClient) initialize(context.Context) error { return nil }
func (c *failingCloseProtocolClient) listTools(context.Context) ([]Tool, error) {
	return nil, nil
}
func (c *failingCloseProtocolClient) callTool(context.Context, string, map[string]any) (map[string]any, error) {
	return nil, nil
}
func (c *failingCloseProtocolClient) close() error {
	c.closeCalls.Add(1)
	return errors.New("close-canary")
}

func requireMCPClientErrorCode(t *testing.T, err error, code string) *Error {
	t.Helper()
	var mcpErr *Error
	if !errors.As(err, &mcpErr) || mcpErr.Code != code {
		t.Fatalf("error = %#v, want %s", err, code)
	}
	return mcpErr
}

func TestManagerRevisionedStandaloneMutations(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	initial, err := manager.Registry()
	if err != nil {
		t.Fatal(err)
	}
	if !validToken(initial.Revision) {
		t.Fatalf("initial revision = %q", initial.Revision)
	}

	cfg := ServerConfig{
		Name: "demo", Description: "Demo", Transport: TransportStreamableHTTP,
		URL: "https://example.invalid/mcp", Enabled: true,
	}
	created, err := manager.AddChecked(cfg, initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Persisted || !created.RuntimeApplied {
		t.Fatalf("create result = %#v", created)
	}
	createdCfg := created.Registry.Servers["demo"]
	if !validToken(createdCfg.Generation) || created.Registry.Revision == initial.Revision {
		t.Fatalf("create metadata = revision %q generation %q", created.Registry.Revision, createdCfg.Generation)
	}

	updatedCfg := cfg
	updatedCfg.Description = "Updated"
	updated, err := manager.Update("demo", updatedCfg, created.Registry.Revision, createdCfg.Generation)
	if err != nil {
		t.Fatal(err)
	}
	updatedEntry := updated.Registry.Servers["demo"]
	if updatedEntry.Description != "Updated" || updatedEntry.Generation == createdCfg.Generation ||
		updated.Registry.Revision == created.Registry.Revision {
		t.Fatalf("update result = %#v", updated)
	}

	_, err = manager.SetEnabledChecked("demo", false, created.Registry.Revision, createdCfg.Generation)
	requireMCPClientErrorCode(t, err, "MCP_REGISTRY_CONFLICT")

	_, err = manager.SetEnabledChecked("demo", false, updated.Registry.Revision, createdCfg.Generation)
	requireMCPClientErrorCode(t, err, "MCP_SERVER_GENERATION_CONFLICT")

	renamed := updatedCfg
	renamed.Name = "renamed"
	_, err = manager.Update("demo", renamed, updated.Registry.Revision, updatedEntry.Generation)
	requireMCPClientErrorCode(t, err, "MCP_NAME_IMMUTABLE")
	afterRejectedRename, err := manager.Registry()
	if err != nil {
		t.Fatal(err)
	}
	if afterRejectedRename.Revision != updated.Registry.Revision {
		t.Fatal("rejected rename changed registry revision")
	}

	removed, err := manager.RemoveChecked("demo", updated.Registry.Revision, updatedEntry.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := removed.Registry.Servers["demo"]; exists {
		t.Fatal("removed server remained in authoritative registry")
	}
	recreated, err := manager.AddChecked(cfg, removed.Registry.Revision)
	if err != nil {
		t.Fatal(err)
	}
	recreatedEntry := recreated.Registry.Servers["demo"]
	if recreatedEntry.Generation == updatedEntry.Generation {
		t.Fatal("delete/recreate reused server generation")
	}
	_, err = manager.SetEnabledChecked("demo", false, recreated.Registry.Revision, updatedEntry.Generation)
	requireMCPClientErrorCode(t, err, "MCP_SERVER_GENERATION_CONFLICT")
}

func TestManagerCheckedMutationRejectsCrossManagerStaleRevision(t *testing.T) {
	home := t.TempDir()
	first, err := NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	firstSnapshot, err := first.Registry()
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot, err := second.Registry()
	if err != nil {
		t.Fatal(err)
	}
	if firstSnapshot.Revision != secondSnapshot.Revision {
		t.Fatalf("initial revisions differ: first=%q second=%q", firstSnapshot.Revision, secondSnapshot.Revision)
	}

	if _, err := first.AddChecked(ServerConfig{
		Name: "first", Description: "First", Transport: TransportStreamableHTTP,
		URL: "https://example.invalid/first", Enabled: true,
	}, firstSnapshot.Revision); err != nil {
		t.Fatal(err)
	}
	_, err = second.AddChecked(ServerConfig{
		Name: "second", Description: "Second", Transport: TransportStreamableHTTP,
		URL: "https://example.invalid/second", Enabled: true,
	}, secondSnapshot.Revision)
	requireMCPClientErrorCode(t, err, "MCP_REGISTRY_CONFLICT")

	authoritative, err := second.Registry()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := authoritative.Servers["second"]; exists {
		t.Fatal("stale checked mutation persisted second server")
	}
	if _, exists := authoritative.Servers["first"]; !exists {
		t.Fatal("authoritative registry lost first manager update")
	}
}

func TestManagerRevisionedUpdateRejectsPluginOwnedServer(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	name := "plugin.demo.remote"
	if err := manager.SetOwnedServers([]ServerConfig{{
		Name: name, DisplayName: "Demo Plugin", Description: "Plugin-owned",
		Transport: TransportStreamableHTTP, URL: "https://example.invalid/mcp",
		StorageKey: name, SourceType: "plugin", PluginName: "demo.plugin",
		PluginRuntimeRoot: t.TempDir(), PluginDataDir: t.TempDir(), Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Registry()
	if err != nil {
		t.Fatal(err)
	}
	owned := snapshot.Servers[name]
	if owned.SourceType != "plugin" || !validToken(owned.Generation) {
		t.Fatalf("owned snapshot = %#v", owned)
	}
	_, err = manager.Update(name, ServerConfig{
		Name: name, Description: "Attempted standalone edit", Transport: TransportStreamableHTTP,
		URL: "https://example.invalid/other", Enabled: true,
	}, snapshot.Revision, owned.Generation)
	requireMCPClientErrorCode(t, err, "MCP_OWNED_BY_PLUGIN")
}

func TestManagerCleanupFailureKeepsOldRuntimeStateAndBlocksReplacement(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	cfg := ServerConfig{
		Name: "demo", Description: "Old runtime config", Transport: TransportStreamableHTTP,
		URL: "https://example.invalid/mcp", Enabled: true,
	}
	if _, err := manager.Add(cfg); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Registry()
	if err != nil {
		t.Fatal(err)
	}
	oldGeneration := snapshot.Servers["demo"].Generation
	failing := &failingCloseProtocolClient{}
	manager.mu.Lock()
	oldState := manager.states["demo"]
	oldState.client = failing
	manager.mu.Unlock()

	updatedCfg := cfg
	updatedCfg.Description = "Persisted new config"
	result, err := manager.Update("demo", updatedCfg, snapshot.Revision, oldGeneration)
	requireMCPClientErrorCode(t, err, "MCP_CLIENT_CLOSE_FAILED")
	if !result.Persisted || result.RuntimeApplied {
		t.Fatalf("partial update result = %#v", result)
	}

	persisted, readErr := manager.Registry()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if persisted.Servers["demo"].Description != "Persisted new config" ||
		persisted.Revision != result.Registry.Revision {
		t.Fatalf("persisted registry = %#v result=%#v", persisted, result)
	}

	manager.mu.RLock()
	runtimeCfg := manager.servers["demo"]
	runtimeState := manager.states["demo"]
	manager.mu.RUnlock()
	if runtimeCfg.Description != "Old runtime config" || runtimeState != oldState || runtimeState.client != failing {
		t.Fatalf("runtime state switched after cleanup failure: cfg=%#v state=%p old=%p", runtimeCfg, runtimeState, oldState)
	}

	_, _, err = manager.Refresh(context.Background(), "demo")
	requireMCPClientErrorCode(t, err, "MCP_CLIENT_CLOSE_FAILED")
	if failing.closeCalls.Load() < 2 {
		t.Fatalf("close attempts = %d, want retry before replacement", failing.closeCalls.Load())
	}
	manager.mu.RLock()
	if manager.states["demo"] != oldState || manager.states["demo"].client != failing {
		manager.mu.RUnlock()
		t.Fatal("failed cleanup was bypassed by replacement runtime state")
	}
	manager.mu.RUnlock()
}

func TestManagerPassiveRegistryAndLegacyMutations(t *testing.T) {
	home := t.TempDir()
	m, err := NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	cfg := ServerConfig{Name: "demo", Description: "Demo", Transport: TransportStreamableHTTP, URL: "https://example.invalid/mcp", Enabled: true}
	if _, err := m.Add(cfg); err != nil {
		t.Fatal(err)
	}
	failing := &failingCloseProtocolClient{}
	m.states["demo"].client = failing
	first, err := m.Registry()
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Registry()
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != second.Revision || first.Servers["demo"].Generation != second.Servers["demo"].Generation || failing.closeCalls.Load() != 0 {
		t.Fatal("passive read changed metadata or closed client")
	}
	// Snapshot maps must not alias the running manager's configs.
	first.Servers["demo"] = ServerConfig{}
	m.states["demo"].client = nil
	disabled, err := m.SetEnabledChecked("demo", false, second.Revision, second.Servers["demo"].Generation)
	if err != nil || !disabled.Persisted || !disabled.RuntimeApplied || disabled.Summary.Enabled {
		t.Fatalf("checked disable: %#v %v", disabled, err)
	}
	if _, err := m.SetEnabled("demo", true); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("demo"); err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 0 {
		t.Fatal("legacy remove left server registered")
	}
}

func TestManagerRegistryReadFailureNeverAuthorizesCachedMutation(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	cfg := ServerConfig{Name: "demo", Description: "Demo", Transport: TransportStreamableHTTP, URL: "https://example.invalid/mcp"}
	if _, err := m.Add(cfg); err != nil {
		t.Fatal(err)
	}
	snapshot, err := m.Registry()
	if err != nil {
		t.Fatal(err)
	}
	corrupt := []byte(`{"version":2,"servers":`)
	if err := os.WriteFile(m.store.path, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	operations := []func() error{
		func() error { _, err := m.Registry(); return err },
		func() error {
			other := cfg
			other.Name = "other"
			_, err := m.AddChecked(other, snapshot.Revision)
			return err
		},
		func() error {
			_, err := m.Update("demo", cfg, snapshot.Revision, snapshot.Servers["demo"].Generation)
			return err
		},
		func() error {
			_, err := m.SetEnabledChecked("demo", true, snapshot.Revision, snapshot.Servers["demo"].Generation)
			return err
		},
		func() error {
			_, err := m.RemoveChecked("demo", snapshot.Revision, snapshot.Servers["demo"].Generation)
			return err
		},
		func() error { return m.Remove("demo") },
	}
	for _, operation := range operations {
		requireMCPClientErrorCode(t, operation(), "MCP_REGISTRY_READ_FAILED")
	}
	after, err := os.ReadFile(m.store.path)
	if err != nil || string(after) != string(corrupt) {
		t.Fatalf("failed read overwrote registry: %q %v", after, err)
	}
}

func TestManagerRefreshCloseFailureDoesNotStartReplacement(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "replacement-started")
	t.Setenv("P5_REPLACEMENT_MARKER", marker)
	cfg := ServerConfig{Name: "local", Description: "Local", Transport: TransportStdio, Command: os.Args[0], Args: []string{"-test.run=TestP5ReplacementHelperProcess"}, EnvFromEnv: map[string]string{"P5_REPLACEMENT_MARKER": "P5_REPLACEMENT_MARKER"}, Enabled: true}
	if _, err := m.Add(cfg); err != nil {
		t.Fatal(err)
	}
	failing := &failingCloseProtocolClient{}
	state := m.states["local"]
	state.client = failing
	for i := 0; i < 2; i++ {
		_, _, err := m.Refresh(context.Background(), "local")
		requireMCPClientErrorCode(t, err, "MCP_CLIENT_CLOSE_FAILED")
		if state.client != failing || state.lastErrorCode != "MCP_CLIENT_CLOSE_FAILED" {
			t.Fatal("close failure lost old client or error state")
		}
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement process started: %v", err)
	}
	if failing.closeCalls.Load() != 2 {
		t.Fatal("refresh bypassed close retry")
	}
	state.client = nil
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestP5ReplacementHelperProcess(t *testing.T) {
	if marker := os.Getenv("P5_REPLACEMENT_MARKER"); marker != "" {
		if err := os.WriteFile(marker, []byte("started"), 0600); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
}
