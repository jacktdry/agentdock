//go:build darwin

package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/execution"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	"github.com/uvwt/agentdock/internal/observability"
	toolacp "github.com/uvwt/agentdock/internal/tool/acp"
)

// Test-only ACP adapter, never an installed Codex/AGY/ChatGPT process.
func TestNextM8ACPHelperProcess(t *testing.T) {
	if os.Getenv("NEXT_M8_ACP_HELPER") != "1" {
		return
	}
	input := bufio.NewScanner(os.Stdin)
	output := json.NewEncoder(os.Stdout)
	for input.Scan() {
		var call struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(input.Bytes(), &call) != nil {
			os.Exit(2)
		}
		// Only isolated test-owned Adapter requests use this optional trace.
		if call.Method == "session/prompt" && os.Getenv("NEXT_M8_TEST_PROMPT_TRACE") != "" {
			if os.WriteFile(os.Getenv("NEXT_M8_TEST_PROMPT_TRACE"), call.Params, 0600) != nil {
				os.Exit(12)
			}
		}
		var response any
		switch call.Method {
		case "initialize":
			response = map[string]any{"protocolVersion": 1, "agentInfo": map[string]any{"name": "next-m8-helper"}, "agentCapabilities": map[string]any{"loadSession": true, "sessionCapabilities": map[string]any{"close": map[string]any{}}}, "authMethods": []any{}}
		case "session/new":
			response = map[string]any{"sessionId": "next-native-thread-a", "modes": map[string]any{"currentModeId": "code", "availableModes": []any{}}}
		case "session/close":
			response = map[string]any{}
		default:
			response = map[string]any{}
		}
		if len(call.ID) > 0 {
			_ = output.Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": response})
		}
	}
}

func nextM8TestRuntime(t *testing.T) *Runtime {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv(nextRuntimeMCPAdmissionEnv, "source-only")
	t.Setenv("NEXT_M8_ACP_HELPER_HOST", "1")
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AgentDockHome: filepath.Join(root, ".agentdock-next"), AgentDockDefaultDir: work, ACPEnabled: true,
		ACPProfiles: []config.ACPProfile{{ID: "next-m8-test", Kind: "custom", Enabled: true, Command: os.Args[0], Args: []string{"-test.run=^TestNextM8ACPHelperProcess$"}, EnvFromEnv: map[string]string{"NEXT_M8_ACP_HELPER": "NEXT_M8_ACP_HELPER_HOST"}}}, ACPDefaultProfile: "next-m8-test",
	}
	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("private Next Core close: %v", err)
		}
	})
	return rt
}

func TestNextM8CoreScopeWithRealManagedACPAdapter(t *testing.T) {
	rt := nextM8TestRuntime(t)
	resp, err := rt.acp.Session(context.Background(), structSessionRequestNew())
	if err != nil {
		t.Fatal(err)
	}
	record, ok := resp["session"].(acpruntime.SessionRecord)
	if !ok || record.ID == "" {
		t.Fatalf("invalid private ACP Session: %+v", resp)
	}
	if !rt.acp.IsLiveManagedSession("next-m8-test", record.ID) {
		t.Fatal("new test ACP Session not loaded")
	}
	principal := requestmeta.NewStableAuthPrincipal("static_bearer", "private-next-m8-test")
	ctx, call := supervisorRuntimeMCPCall(t, rt, principal, "mcp_tool_call")
	binding, err := rt.nextM8ContextForLoadedACP(ctx, "next-m8-test", record.ID)
	if err != nil || binding.Gate != rt.admission || binding.Binding.ACPSessionID != record.ID || binding.Audit.CallID != call.ID || binding.Binding.Principal.ID != principal.ID {
		t.Fatalf("trusted Core binding not pinned: %+v %v", binding, err)
	}
	// All mutations must fail closed without sending a tool or starting an MCP.
	cases := []struct {
		name           string
		context        context.Context
		profile, owner string
	}{
		{"no-marker", requestmeta.WithAuthPrincipal(execution.WithScope(context.Background(), execution.Scope{CallID: call.ID}), principal), "next-m8-test", record.ID},
		{"forged-principal", requestmeta.WithAuthPrincipal(ctx, requestmeta.NewStableAuthPrincipal("static_bearer", "other")), "next-m8-test", record.ID},
		{"other-profile", ctx, "unknown-profile", record.ID},
		{"other-owner", ctx, "next-m8-test", "another-session"},
		{"forged-call", execution.WithScope(ctx, execution.Scope{CallID: "fake"}), "next-m8-test", record.ID},
		{"source-mismatch", observability.WithSource(ctx, observability.SourceNexus), "next-m8-test", record.ID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := rt.nextM8ContextForLoadedACP(tc.context, tc.profile, tc.owner); !errors.Is(err, mcpclient.ErrNextM8NotAllowed) {
				t.Fatalf("forged Core context admitted: %v", err)
			}
		})
	}
	// Removing explicit Next-only opt-in denies even a legitimate live ACP call.
	t.Setenv(nextRuntimeMCPAdmissionEnv, "")
	if _, err := rt.nextM8ContextForLoadedACP(ctx, "next-m8-test", record.ID); !errors.Is(err, mcpclient.ErrNextM8NotAllowed) {
		t.Fatal("opt-in absence allowed native MCP")
	}
	t.Setenv(nextRuntimeMCPAdmissionEnv, "source-only")
	if _, ok := rt.execution.Finish(call.ID, execution.FinishInput{Status: execution.StatusCompleted}); !ok {
		t.Fatal("failed to finish Core call")
	}
	if _, err := rt.nextM8ContextForLoadedACP(ctx, "next-m8-test", record.ID); !errors.Is(err, mcpclient.ErrNextM8NotAllowed) {
		t.Fatal("finished Core scope reused")
	}
}

func structSessionRequestNew() toolacp.SessionRequest {
	return toolacp.SessionRequest{Action: "new", ProfileID: "next-m8-test"}
}
