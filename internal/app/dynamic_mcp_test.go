package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/observability"
)

func TestDynamicMCPToolsStaySeparateAndAppearLightweightInContext(t *testing.T) {
	blockStarted := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var rpc struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&rpc); err != nil {
			t.Errorf("decode upstream request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch rpc.Method {
		case "server/discover":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      rpc.ID,
				"error":   map[string]any{"code": -32601, "message": "Method not found"},
			})
		case "initialize":
			writeDynamicMCPRPCResult(t, w, rpc.ID, map[string]any{
				"protocolVersion": "2025-11-25",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "demo-upstream", "version": "1.0.0"},
			})
		case "notifications/initialized", "notifications/cancelled":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeDynamicMCPRPCResult(t, w, rpc.ID, map[string]any{
				"tools": []map[string]any{{
					"name":        "echo",
					"description": "Echo supplied text",
					"inputSchema": map[string]any{
						"type":       "object",
						"required":   []string{"text"},
						"properties": map[string]any{"text": map[string]any{"type": "string"}},
					},
				}},
			})
		case "tools/call":
			arguments, _ := rpc.Params["arguments"].(map[string]any)
			switch arguments["text"] {
			case "fail":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      rpc.ID,
					"error":   map[string]any{"code": -32000, "message": "synthetic failure"},
				})
				return
			case "semantic_fail":
				writeDynamicMCPRPCResult(t, w, rpc.ID, map[string]any{
					"content": []map[string]any{{"type": "text", "text": "semantic failure"}},
					"isError": true,
				})
				return
			case "block":
				select {
				case blockStarted <- struct{}{}:
				default:
				}
				<-request.Context().Done()
				return
			}
			writeDynamicMCPRPCResult(t, w, rpc.ID, map[string]any{
				"content":           []map[string]any{{"type": "text", "text": arguments["text"]}},
				"structuredContent": map[string]any{"echo": arguments["text"]},
			})
		default:
			t.Errorf("unexpected upstream method %q", rpc.Method)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	cfg := config.Config{
		AgentDockDefaultDir: t.TempDir(),
		AgentDockHome:       filepath.Join(t.TempDir(), ".agentdock"),
	}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	added, err := runtime.Call(context.Background(), "mcp_manage", map[string]any{
		"action":      "add",
		"name":        "demo",
		"description": "Demo external capabilities",
		"transport":   "streamable_http",
		"url":         upstream.URL,
	})
	if err != nil {
		t.Fatalf("mcp_manage add: %v", err)
	}
	if added["action"] != "add" {
		t.Fatalf("unexpected add result: %#v", added)
	}
	assertToolResultMatchestestOutputSchema(t, "mcp_manage", added)

	contextResult, err := runtime.Call(context.Background(), "agentdock_context", map[string]any{})
	if err != nil {
		t.Fatalf("agentdock_context: %v", err)
	}
	var contextData capabilityContext
	if err := remarshal(contextResult, &contextData); err != nil {
		t.Fatal(err)
	}
	if len(contextData.DynamicMCP) != 1 || contextData.DynamicMCP[0].Name != "demo" || contextData.DynamicMCP[0].Description != "Demo external capabilities" || contextData.DynamicMCP[0].Status != "idle" || contextData.DynamicMCP[0].ToolCount != 0 {
		t.Fatalf("dynamic MCP context = %#v", contextData.DynamicMCP)
	}
	encodedContext, err := json.Marshal(contextResult)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{upstream.URL, "streamable_http", "demo:echo", "inputSchema"} {
		if strings.Contains(string(encodedContext), forbidden) {
			t.Fatalf("agentdock_context leaked %q: %s", forbidden, encodedContext)
		}
	}
	rules := strings.Join(contextData.Rules, "\n")
	for _, required := range []string{"mcp_tool_search", "mcp_tool_inspect", "mcp_tool_call"} {
		if !strings.Contains(rules, required) {
			t.Fatalf("agentdock_context rules missing %q: %s", required, rules)
		}
	}

	coldCall, err := runtime.Call(context.Background(), "mcp_tool_call", map[string]any{
		"name":      "demo:echo",
		"arguments": map[string]any{"text": "cold"},
	})
	if err != nil {
		t.Fatalf("cold mcp_tool_call: %v", err)
	}
	coldRemote, _ := coldCall["result"].(map[string]any)
	coldStructured, _ := coldRemote["structuredContent"].(map[string]any)
	if coldStructured["echo"] != "cold" {
		t.Fatalf("unexpected cold call result: %#v", coldCall)
	}
	coldRecord := runtime.observer.Snapshot().RecentCalls[0]
	if coldRecord.Tool != "mcp_tool_call" || len(coldRecord.Stages) != 2 {
		t.Fatalf("cold MCP analytics = %#v", coldRecord)
	}
	if coldRecord.Stages[0].Name != observability.StageMCPRefresh || coldRecord.Stages[1].Name != observability.StageMCPRemoteCall {
		t.Fatalf("cold MCP stage order = %#v", coldRecord.Stages)
	}
	for _, stage := range coldRecord.Stages {
		if !stage.Success {
			t.Fatalf("cold MCP stage failed: %#v", stage)
		}
	}
	executionSnapshot := runtime.execution.Snapshot()
	if len(executionSnapshot.Calls) < 2 {
		t.Fatalf("execution snapshot missing MCP root/child: %#v", executionSnapshot)
	}
	coldRoot := executionSnapshot.Calls[len(executionSnapshot.Calls)-2]
	coldChild := executionSnapshot.Calls[len(executionSnapshot.Calls)-1]
	if coldRoot.Tool != "mcp_tool_call" || coldRoot.Status != "completed" ||
		coldChild.Tool != "demo:echo" || coldChild.Status != "completed" ||
		coldChild.ParentCallID != coldRoot.ID || coldChild.Source != "dynamic_mcp" {
		t.Fatalf("cold MCP execution hierarchy = root %#v child %#v", coldRoot, coldChild)
	}

	search, err := runtime.Call(context.Background(), "mcp_tool_search", map[string]any{
		"server": "demo",
		"query":  "echo text",
	})
	if err != nil {
		t.Fatalf("mcp_tool_search: %v", err)
	}
	if search["count"] != 1 {
		t.Fatalf("unexpected search result: %#v", search)
	}
	readyContextResult, err := runtime.Call(context.Background(), "agentdock_context", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var readyContext capabilityContext
	if err := remarshal(readyContextResult, &readyContext); err != nil {
		t.Fatal(err)
	}
	if readyContext.DynamicMCP[0].Status != "ready" || readyContext.DynamicMCP[0].ToolCount != 1 || readyContext.DynamicMCP[0].LastErrorCode != "" {
		t.Fatalf("ready dynamic MCP context = %#v", readyContext.DynamicMCP[0])
	}
	assertToolResultMatchestestOutputSchema(t, "mcp_tool_search", search)

	inspect, err := runtime.Call(context.Background(), "mcp_tool_inspect", map[string]any{"name": "demo:echo"})
	if err != nil {
		t.Fatalf("mcp_tool_inspect: %v", err)
	}
	if inspect["tool_name"] != "echo" {
		t.Fatalf("unexpected inspect result: %#v", inspect)
	}
	normalizedInspect := assertToolResultMatchestestOutputSchema(t, "mcp_tool_inspect", inspect)
	for _, optional := range []string{"output_schema", "annotations"} {
		if _, exists := normalizedInspect[optional]; exists {
			t.Fatalf("mcp_tool_inspect returned absent upstream %s: %#v", optional, normalizedInspect[optional])
		}
	}

	called, err := runtime.Call(context.Background(), "mcp_tool_call", map[string]any{
		"name":      "demo:echo",
		"arguments": map[string]any{"text": "hello"},
	})
	if err != nil {
		t.Fatalf("mcp_tool_call: %v", err)
	}
	remote, _ := called["result"].(map[string]any)
	structured, _ := remote["structuredContent"].(map[string]any)
	if structured["echo"] != "hello" {
		t.Fatalf("unexpected call result: %#v", called)
	}
	assertToolResultMatchestestOutputSchema(t, "mcp_tool_call", called)
	warmRecord := runtime.observer.Snapshot().RecentCalls[0]
	if len(warmRecord.Stages) != 1 || warmRecord.Stages[0].Name != observability.StageMCPRemoteCall {
		t.Fatalf("warm MCP stages = %#v, want only remote call", warmRecord.Stages)
	}

	if _, err := runtime.Call(context.Background(), "mcp_tool_call", map[string]any{
		"name":      "demo:echo",
		"arguments": map[string]any{"text": "fail"},
	}); err == nil {
		t.Fatal("mcp_tool_call synthetic failure unexpectedly succeeded")
	}
	failedRecord := runtime.observer.Snapshot().RecentCalls[0]
	if len(failedRecord.Stages) != 1 ||
		failedRecord.Stages[0].Name != observability.StageMCPRemoteCall ||
		failedRecord.Stages[0].Success {
		t.Fatalf("failed MCP stages = %#v, want remote call success=false", failedRecord.Stages)
	}
	executionSnapshot = runtime.execution.Snapshot()
	failedRoot := executionSnapshot.Calls[len(executionSnapshot.Calls)-2]
	failedChild := executionSnapshot.Calls[len(executionSnapshot.Calls)-1]
	if failedRoot.Tool != "mcp_tool_call" || failedRoot.Status != "failed" ||
		failedChild.Tool != "demo:echo" || failedChild.Status != "failed" ||
		failedChild.ParentCallID != failedRoot.ID || failedChild.ErrorCode == "" {
		t.Fatalf("failed MCP execution hierarchy = root %#v child %#v", failedRoot, failedChild)
	}

	semantic, err := runtime.Call(context.Background(), "mcp_tool_call", map[string]any{
		"name":      "demo:echo",
		"arguments": map[string]any{"text": "semantic_fail"},
	})
	if err != nil {
		t.Fatalf("semantic MCP result changed public error contract: %v", err)
	}
	semanticRemote, _ := semantic["result"].(map[string]any)
	if semanticRemote["isError"] != true {
		t.Fatalf("semantic MCP result = %#v", semantic)
	}
	executionSnapshot = runtime.execution.Snapshot()
	semanticRoot := executionSnapshot.Calls[len(executionSnapshot.Calls)-2]
	semanticChild := executionSnapshot.Calls[len(executionSnapshot.Calls)-1]
	if semanticRoot.Status != "failed" || semanticRoot.ErrorCode != "TOOL_RESULT_ERROR" ||
		semanticChild.Status != "failed" || semanticChild.ErrorCode != "MCP_TOOL_RESULT_ERROR" ||
		semanticChild.ParentCallID != semanticRoot.ID {
		t.Fatalf("semantic MCP execution hierarchy = root %#v child %#v", semanticRoot, semanticChild)
	}

	cancelCtx, cancelCall := context.WithCancel(context.Background())
	type callOutcome struct {
		result Result
		err    error
	}
	cancelledCall := make(chan callOutcome, 1)
	go func() {
		result, callErr := runtime.Call(cancelCtx, "mcp_tool_call", map[string]any{
			"name":      "demo:echo",
			"arguments": map[string]any{"text": "block"},
		})
		cancelledCall <- callOutcome{result: result, err: callErr}
	}()
	select {
	case <-blockStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking MCP call did not start")
	}
	cancelCall()
	select {
	case <-cancelledCall:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled MCP call did not return")
	}
	executionSnapshot = runtime.execution.Snapshot()
	cancelRoot := executionSnapshot.Calls[len(executionSnapshot.Calls)-2]
	cancelChild := executionSnapshot.Calls[len(executionSnapshot.Calls)-1]
	if cancelRoot.Status != "cancelled" || cancelChild.Status != "cancelled" ||
		cancelChild.ParentCallID != cancelRoot.ID {
		t.Fatalf("cancelled MCP execution hierarchy = root %#v child %#v", cancelRoot, cancelChild)
	}

	for _, name := range runtime.ToolNames() {
		if name == "demo:echo" {
			t.Fatal("dynamic upstream tool leaked into AgentDock built-in tools/list")
		}
	}
}

func TestAgentDockContextReportsDynamicMCPRefreshErrorCode(t *testing.T) {
	cfg := config.Config{AgentDockDefaultDir: t.TempDir(), AgentDockHome: filepath.Join(t.TempDir(), ".agentdock")}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	if _, err := runtime.Call(context.Background(), "mcp_manage", map[string]any{
		"action": "add", "name": "broken", "description": "Missing required host environment",
		"transport": "stdio", "command": os.Args[0],
		"env_from_env": map[string]any{"REQUIRED": "AGENTDOCK_TEST_MISSING_MCP_ENV"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Call(context.Background(), "mcp_tool_search", map[string]any{"server": "broken", "query": "anything"}); err == nil {
		t.Fatal("mcp_tool_search succeeded with a missing required environment variable")
	}

	result, err := runtime.Call(context.Background(), "agentdock_context", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var contextData capabilityContext
	if err := remarshal(result, &contextData); err != nil {
		t.Fatal(err)
	}
	if len(contextData.DynamicMCP) != 1 {
		t.Fatalf("dynamic MCP context = %#v", contextData.DynamicMCP)
	}
	item := contextData.DynamicMCP[0]
	if item.Name != "broken" || item.Status != "error" || item.ToolCount != 0 || item.LastErrorCode != "MCP_CREDENTIAL_REQUIRED" {
		t.Fatalf("broken dynamic MCP context = %#v", item)
	}
}

func writeDynamicMCPRPCResult(t *testing.T, writer http.ResponseWriter, id any, result any) {
	t.Helper()
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}); err != nil {
		t.Errorf("write upstream response: %v", err)
	}
}
