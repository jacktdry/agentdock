package desktopapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/desktopruntime"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

func testACPService(t *testing.T) (*ACPService, *[]map[string]any) {
	t.Helper()
	calls := []map[string]any{}
	s := &ACPService{
		runtimeRoot: "/runtime",
		readSettings: func(context.Context, string) (desktopruntime.ACPSettings, error) {
			return desktopruntime.ACPSettings{Enabled: true, DefaultProfile: "codex", Profiles: []desktopruntime.ACPProfileSettings{{ID: "codex", Kind: "codex", Command: "/missing/codex-acp", Enabled: true}}}, nil
		},
		readAccess: func(context.Context, string) (desktopruntime.LocalCoreAccess, error) {
			return desktopruntime.LocalCoreAccess{MCPURL: "http://127.0.0.1:8765/mcp", AuthToken: "super-secret-core-token"}, nil
		},
		memoryURL: "http://127.0.0.1:8766/mcp",
	}
	s.call = func(_ context.Context, cfg mcpclient.ServerConfig, tool string, args map[string]any) (map[string]any, error) {
		calls = append(calls, map[string]any{"name": cfg.Name, "auth": cfg.StaticHeaders["Authorization"], "tool": tool, "args": args})
		if cfg.Name == "desktop-memory" {
			return map[string]any{"content": []any{map[string]any{"type": "text", "text": `{"status":"healthy","version":"11.14.0","backend":"sqlite-vec","db_path":"/memory/sqlite_vec.db","embedding_provider":"sentence-transformers","total_memories":321}`}}}, nil
		}
		return map[string]any{"structuredContent": map[string]any{
			"profile_id":         "codex",
			"diagnostics":        map[string]any{"observed_at": "2026-10-04T13:00:00Z", "counts": map[string]any{"managed": 2, "loaded": 1, "running": 0, "ready": 1, "idle_managed_idle": 1, "idle_managed_eligible": 0, "closed": 1, "auto_close_failures": 0}, "sessions": []any{map[string]any{"session_id": "acps-1", "status": "ready", "lifecycle_policy": "idle-managed", "loaded": true, "pending_interactions": 0, "session_operations": 0, "last_active_at": "2026-10-04T12:59:00Z", "idle_close_after_ms": 300000, "idle_managed_idle": true, "idle_managed_eligible": false}}},
			"adapter_process":    map[string]any{"root_pid": 123, "alive": true, "state": "alive", "descendant_count": 2, "rss_bytes": 1000, "tree_rss_bytes": 2000, "observed_at": "2026-10-04T13:00:00Z"},
			"broker_correlation": map[string]any{"sessions": []any{map[string]any{"session_id": "acps-1", "browser": map[string]any{"lease_ids": []any{"lease-1"}, "active_leases": 1, "cleanup_issues": 0}, "computer": map[string]any{"computer_session_ids": []any{"computer-1"}, "observe_sessions": 1, "act_sessions": 0}}}},
		}}, nil
	}
	return s, &calls
}

func TestACPServiceStatusUsesLocalCoreWithoutLeakingToken(t *testing.T) {
	s, calls := testACPService(t)
	result := s.Status(context.Background())
	if result.Error != nil || !result.Enabled || result.DefaultProfile != "codex" || len(result.Profiles) != 1 {
		t.Fatalf("result=%+v", result)
	}
	profile := result.Profiles[0]
	if profile.Error != nil || profile.Counts.Managed != 2 || len(profile.Sessions) != 1 || profile.AdapterProcess == nil || profile.AdapterProcess.RootPID != 123 || len(profile.Resources) != 1 {
		t.Fatalf("profile=%+v", profile)
	}
	if !result.Memory.Healthy || result.Memory.Version != "11.14.0" || result.Memory.Backend != "sqlite-vec" || result.Memory.TotalMemories != 321 {
		t.Fatalf("memory=%+v", result.Memory)
	}
	if len(*calls) != 2 {
		t.Fatalf("calls=%#v", *calls)
	}
	for _, call := range *calls {
		if call["name"] == "desktop-core" && call["auth"] != "Bearer super-secret-core-token" {
			t.Fatalf("auth=%#v", call)
		}
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "super-secret-core-token") {
		t.Fatalf("desktop result leaked token: %s", encoded)
	}
}

func TestACPServiceMutationsCallExistingCoreSession(t *testing.T) {
	s, calls := testACPService(t)
	if got := s.Close(context.Background(), "codex", "acps-1"); !got.Completed || got.Error != nil {
		t.Fatalf("close=%+v", got)
	}
	if got := s.UpdateLifecycle(context.Background(), ACPLifecycleUpdate{ProfileID: "codex", SessionID: "acps-1", Policy: "idle-managed", IdleCloseAfterMS: 600000}); !got.Completed || got.Error != nil {
		t.Fatalf("update=%+v", got)
	}
	var closeOK, updateOK bool
	for _, call := range *calls {
		args, _ := call["args"].(map[string]any)
		switch args["action"] {
		case "close":
			closeOK = args["profile_id"] == "codex" && args["session_id"] == "acps-1"
		case "update":
			updateOK = args["lifecycle_policy"] == "idle-managed" && args["idle_close_after_ms"] == int64(600000)
		}
	}
	if !closeOK || !updateOK {
		t.Fatalf("calls=%#v", *calls)
	}
}

func TestACPServiceRedactsRemoteFailures(t *testing.T) {
	s, _ := testACPService(t)
	s.call = func(context.Context, mcpclient.ServerConfig, string, map[string]any) (map[string]any, error) {
		return nil, fmt.Errorf("secret remote payload bearer-123")
	}
	result := s.Status(context.Background())
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "bearer-123") {
		t.Fatalf("leaked remote error: %s", encoded)
	}
}

func TestACPServiceRejectsMalformedMutationsBeforeReadingCredentials(t *testing.T) {
	s, calls := testACPService(t)
	s.readAccess = func(context.Context, string) (desktopruntime.LocalCoreAccess, error) {
		t.Fatal("invalid mutation read credentials")
		return desktopruntime.LocalCoreAccess{}, nil
	}
	for _, ids := range [][2]string{{"", "s"}, {"p", " "}} {
		if got := s.Close(context.Background(), ids[0], ids[1]); got.Completed || got.Error == nil || got.Error.Category != ErrorCategoryValidation {
			t.Fatalf("close = %+v", got)
		}
	}
	for _, update := range []ACPLifecycleUpdate{
		{SessionID: "s", Policy: "persistent"}, {ProfileID: "p", Policy: "persistent"},
		{ProfileID: "p", SessionID: "s"}, {ProfileID: "p", SessionID: "s", Policy: "unknown"},
		{ProfileID: "p", SessionID: "s", Policy: "idle-managed", IdleCloseAfterMS: -1},
		{ProfileID: "p", SessionID: "s", Policy: "idle-managed", IdleCloseAfterMS: 1},
		{ProfileID: "p", SessionID: "s", Policy: "idle-managed", IdleCloseAfterMS: 604800001},
		{ProfileID: "p", SessionID: "s", Policy: "idle-managed", IdleCloseAfterMS: 1<<63 - 1},
		{ProfileID: "p", SessionID: "s", Policy: "persistent", IdleCloseAfterMS: 60000},
	} {
		if got := s.UpdateLifecycle(context.Background(), update); got.Completed || got.Error == nil || got.Error.Category != ErrorCategoryValidation {
			t.Fatalf("update = %+v", got)
		}
	}
	if len(*calls) != 0 {
		t.Fatal("invalid mutation called Core")
	}
}

func TestACPServiceMemoryFailureDoesNotHideCoreAndSanitizesDiagnostics(t *testing.T) {
	s, _ := testACPService(t)
	call := s.call
	s.call = func(ctx context.Context, cfg mcpclient.ServerConfig, tool string, args map[string]any) (map[string]any, error) {
		if cfg.Name == "desktop-memory" {
			if cfg.ProtocolVersion != "2025-11-25" || cfg.URL != defaultSharedMemoryMCPURL || len(cfg.StaticHeaders) != 0 {
				t.Fatal("memory authority changed")
			}
			return nil, fmt.Errorf("private-memory-payload")
		}
		if tool != "acp_session" || args["action"] != "status" || args["profile_id"] != "codex" {
			t.Fatal("unexpected Core call")
		}
		result, err := call(ctx, cfg, tool, args)
		core := result["structuredContent"].(map[string]any)
		core["adapter_process"].(map[string]any)["error"] = "private-adapter-payload super-secret-core-token"
		session := core["diagnostics"].(map[string]any)["sessions"].([]any)[0].(map[string]any)
		session["auto_close_error"] = "private-close-payload super-secret-core-token"
		session["active_run_id"] = "super-secret-core-token"
		return result, err
	}
	result := s.Status(context.Background())
	if result.Error != nil || result.Memory.Error == nil || len(result.Profiles) != 1 || result.Profiles[0].Error != nil || result.Profiles[0].Counts.Managed != 2 {
		t.Fatalf("status=%+v", result)
	}
	encoded, _ := json.Marshal(result)
	for _, secret := range []string{"private-memory-payload", "private-adapter-payload", "private-close-payload", "super-secret-core-token"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("status leaks %q", secret)
		}
	}
}
