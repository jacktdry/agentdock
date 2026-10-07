package desktopapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testMCPService(t *testing.T, handler http.HandlerFunc) *MCPService {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	service := NewMCPService(t.TempDir())
	service.readAccess = func(string) (mcpCoreAccess, error) {
		return mcpCoreAccess{Endpoint: server.URL, Token: "core-token"}, nil
	}
	service.client = server.Client()
	service.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return service
}

func TestMCPServiceSnapshotProjectsProtectedCoreDTO(t *testing.T) {
	service := testMCPService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != mcpDesktopPath || r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer core-token" {
			t.Fatalf("request = %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, "{\"ok\":true,\"registry_revision\":\"rev-1\",\"authoritative\":true,\"servers\":[{\"name\":\"demo\",\"display_name\":\"Demo\",\"description\":\"Safe\",\"source_type\":\"standalone\",\"transport\":\"stdio\",\"command\":\"/usr/bin/demo\",\"args_protected\":true,\"enabled\":false,\"timeout_ms\":30000,\"generation\":\"gen-1\",\"observation\":{\"connection\":\"not_connected\",\"status\":\"idle\",\"auth_status\":\"not_applicable\",\"stale\":false}}]}")
	})
	result := service.Snapshot(context.Background())
	if result.Error != nil || result.Snapshot.RegistryRevision != "rev-1" || len(result.Snapshot.Servers) != 1 {
		t.Fatalf("snapshot = %#v", result)
	}
	server := result.Snapshot.Servers[0]
	if server.Name != "demo" || !server.ArgsProtected || len(server.Args) != 0 || server.Observation.LastErrorCode != "" {
		t.Fatalf("server = %#v", server)
	}
}

func TestMCPServicePreservesPartialMutationTruthAndReclassifiesSafeError(t *testing.T) {
	service := testMCPService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		_, _ = io.WriteString(w, "{\"ok\":true,\"completed\":false,\"persisted\":true,\"runtime_applied\":false,\"recovery_required\":true,\"registry_revision\":\"rev-2\",\"server\":{\"name\":\"demo\",\"display_name\":\"demo\",\"description\":\"Demo\",\"source_type\":\"standalone\",\"transport\":\"stdio\",\"command\":\"demo\",\"enabled\":true,\"timeout_ms\":30000,\"generation\":\"gen-2\",\"observation\":{\"connection\":\"unknown\",\"status\":\"unknown\",\"auth_status\":\"unknown\",\"stale\":true}},\"safe_error\":{\"code\":\"MCP_CLIENT_CLOSE_FAILED\",\"category\":\"external\",\"retryable\":false}}")
	})
	result := service.Update(context.Background(), "demo", "rev-1", "gen-1", MCPConfigInput{Description: "Demo", Transport: "stdio", Command: "demo"})
	if !result.Persisted || result.RuntimeApplied || !result.RecoveryRequired || result.RegistryRevision != "rev-2" {
		t.Fatalf("mutation = %#v", result)
	}
	if result.Error == nil || result.Error.Code != "MCP_CLIENT_CLOSE_FAILED" || result.Error.Category != ErrorCategoryOperation {
		t.Fatalf("safe error = %#v", result.Error)
	}
}

func TestMCPServiceWriteOnlyEnvironmentValueNeverReturns(t *testing.T) {
	const secret = "ENV_SECRET_CANARY_123456789"
	service := testMCPService(t, func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(data), secret) {
			t.Fatalf("write-only secret was not sent to Core")
		}
		_, _ = io.WriteString(w, "{\"ok\":true,\"env_revision\":\"env-2\",\"items\":[{\"key\":\"TOKEN\",\"configured\":true}],\"count\":1}")
	})
	result := service.SetEnvironment(context.Background(), "demo", "TOKEN", secret, "rev-1", "gen-1", "env-1")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Error != nil || result.Revision != "env-2" || strings.Contains(string(data), secret) {
		t.Fatalf("environment result = %s", data)
	}
}

func TestMCPServiceApprovalErrorKeepsOnlyAllowlistedDetails(t *testing.T) {
	const raw = "RAW_SECRET_CANARY"
	service := testMCPService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, "{\"ok\":false,\"code\":\"APPROVAL_REQUIRED\",\"error\":\"RAW_SECRET_CANARY\",\"details\":{\"approval_id\":\"ap-1\",\"approval_version\":2,\"policy_revision\":3,\"executed\":false,\"retry\":true,\"raw\":\"RAW_SECRET_CANARY\"}}")
	})
	result := service.Create(context.Background(), "rev-1", MCPConfigInput{Name: "demo", Description: "Demo", Transport: "stdio", Command: "demo"})
	data, _ := json.Marshal(result)
	if result.Error == nil || result.Error.Code != "APPROVAL_REQUIRED" || result.Error.Category != ErrorCategoryPermission {
		t.Fatalf("approval = %#v", result.Error)
	}
	if result.Error.Details["approval_id"] != "ap-1" || result.Error.Details["raw"] != "" || strings.Contains(string(data), raw) {
		t.Fatalf("approval leaked unsafe data: %s", data)
	}
}

func TestMCPServiceRejectsNonLoopbackCoreEndpoint(t *testing.T) {
	service := NewMCPService(t.TempDir())
	service.readAccess = func(string) (mcpCoreAccess, error) {
		return mcpCoreAccess{Endpoint: "http://198.51.100.10:8765", Token: "secret"}, nil
	}
	result := service.Snapshot(context.Background())
	if result.Error == nil || result.Error.Code != "MCP_CORE_UNAVAILABLE" {
		t.Fatalf("result = %#v", result)
	}
}

func TestMCPServiceDoesNotFollowCoreRedirects(t *testing.T) {
	var destinationHits int
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		destinationHits++
	}))
	defer destination.Close()
	service := testMCPService(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusFound)
	})
	result := service.Snapshot(context.Background())
	if result.Error == nil || destinationHits != 0 {
		t.Fatalf("redirect result=%#v destinationHits=%d", result, destinationHits)
	}
}
