package desktopapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPluginServiceSafeSnapshotAndWriteOnlyBoundary(t *testing.T) {
	service := &PluginService{core: testMCPService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pluginDesktopPath || r.Header.Get("Authorization") != "Bearer core-token" {
			t.Fatalf("wrong route/auth %s", r.URL.Path)
		}
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"ok":true,"authoritative":true,"registry_revision":"rev","plugins":[{"name":"demo","generation":"generation","skills_count":2,"mcp_count":1,"warning_count":3}],"recovery_items":[],"storage_key":"RAW_CANARY","review_token":"RAW_CANARY"}`)
		} else {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "SECRET_CANARY") {
				t.Fatal("missing write-only value")
			}
			_, _ = io.WriteString(w, `{"ok":true,"request_id":"id","outcome":"completed","completed":true,"persisted":true,"runtime_applied":false,"runtime_impact":"next_connection","env_revision":"env-2","items":[{"key":"TOKEN","configured":true,"value":"SECRET_CANARY"}]}`)
		}
	})}
	snapshot := service.Snapshot(context.Background())
	if snapshot.Error != nil || !snapshot.Snapshot.Authoritative || snapshot.Snapshot.Plugins[0].SkillsCount != 2 {
		t.Fatalf("snapshot %#v", snapshot)
	}
	value := "SECRET_CANARY"
	result := service.SetEnvironment(context.Background(), PluginEnvironmentInput{PluginMutationInput: PluginMutationInput{RequestID: "id", Name: "demo"}, Component: "one", Key: "TOKEN", Value: &value})
	if result.Error != nil || !result.Persisted || result.RuntimeApplied || result.EnvRevision != "env-2" {
		t.Fatalf("result %#v", result)
	}
	data, _ := json.Marshal([]any{snapshot, result})
	if strings.Contains(string(data), "CANARY") || strings.Contains(string(data), "storage_key") {
		t.Fatalf("unsafe %s", data)
	}
}
func TestPluginServiceSafeErrorsAndUnknownOutcome(t *testing.T) {
	service := &PluginService{core: testMCPService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"code":"RAW_CANARY /private/path","error":"RAW_CANARY","details":{"storage_key":"RAW_CANARY"}}`)
	})}
	result := service.RemovePurge(context.Background(), PluginMutationInput{RequestID: "id", Name: "demo"})
	data, _ := json.Marshal(result)
	if result.Error == nil || result.Error.Code != "PLUGIN_OPERATION_FAILED" || strings.Contains(string(data), "CANARY") {
		t.Fatalf("error %s", data)
	}
	service.core.client = &http.Client{Transport: mcpRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })}
	result = service.RemoveKeep(context.Background(), PluginMutationInput{RequestID: "id", Name: "demo"})
	if !result.OutcomeUnknown || result.Outcome != "outcome_unknown" || result.Error.Code != "PLUGIN_TIMEOUT" {
		t.Fatalf("timeout %#v", result)
	}
	service.core.client = &http.Client{Transport: mcpRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("/private/RAW_CANARY") })}
	result = service.SetEnabled(context.Background(), PluginMutationInput{RequestID: "id", Name: "demo"}, true)
	if !result.OutcomeUnknown || result.Error.Code != "PLUGIN_CORE_UNAVAILABLE" {
		t.Fatalf("unavailable %#v", result)
	}
}
