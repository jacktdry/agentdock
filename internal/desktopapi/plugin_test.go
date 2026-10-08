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

func TestPluginServiceCandidatePickerKeepsNativePathPrivate(t *testing.T) {
	privatePath := "/Users/example/private/secret-plugin.zip"
	candidateID := strings.Repeat("a", 64)
	service := &PluginService{
		core: testMCPService(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == pluginDesktopCandidatePath && r.Header.Get("Authorization") != "Bearer desktop-control-secret" {
				t.Fatal("candidate route did not use Desktop control credential")
			}
			body, _ := io.ReadAll(r.Body)
			switch r.URL.Path {
			case pluginDesktopCandidatePath:
				if !strings.Contains(string(body), privatePath) {
					t.Fatal("private candidate route did not receive picker path")
				}
				_, _ = io.WriteString(w, `{"ok":true,"candidate":{"candidate_id":"`+candidateID+`","kind":"install","expires_at":"2026-10-08T02:00:00Z","review":{"valid":true,"name":"demo","version":"1.0.0","format":"portable","package_fingerprint":"sha256:safe","skills":[],"mcp":[{"name":"remote","transport":"streamable-http","endpoint":"https://example.invalid"}],"executables":[],"warnings":[],"issues":[],"review_token":"RAW_REVIEW_CANARY","storage_key":"RAW_STORAGE_CANARY"},"source":"`+privatePath+`"}}`)
			case pluginDesktopPath:
				if strings.Contains(string(body), privatePath) || strings.Contains(string(body), "source") {
					t.Fatal("main Desktop mutation carried native path")
				}
				if !strings.Contains(string(body), candidateID) {
					t.Fatal("candidate id missing from mutation")
				}
				_, _ = io.WriteString(w, `{"ok":true,"request_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","outcome":"completed","completed":true,"persisted":true,"runtime_applied":true,"runtime_impact":"installed_disabled"}`)
			default:
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
		}),
		picker: func(sourceType string) (string, error) {
			if sourceType != "zip" {
				t.Fatalf("sourceType=%s", sourceType)
			}
			return privatePath, nil
		},
		bootstrap: func(context.Context, string) (string, error) { return "desktop-control-secret", nil },
	}
	chosen := service.ChooseCandidate(context.Background(), PluginCandidatePickerInput{Kind: "install", SourceType: "zip"})
	if chosen.Error != nil || chosen.Cancelled || chosen.Candidate == nil || chosen.Candidate.CandidateID != candidateID || chosen.SourceLabel != "secret-plugin.zip" {
		t.Fatalf("chosen %#v", chosen)
	}
	data, _ := json.Marshal(chosen)
	for _, forbidden := range []string{privatePath, "RAW_REVIEW_CANARY", "RAW_STORAGE_CANARY", "review_token", "storage_key"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("renderer candidate leaked %q: %s", forbidden, data)
		}
	}
	result := service.InstallCandidate(context.Background(), PluginCandidateMutationInput{
		RequestID: strings.Repeat("b", 32), CandidateID: candidateID, Name: "demo", ExpectedRegistryRevision: strings.Repeat("c", 64),
	})
	if result.Error != nil || !result.Completed {
		t.Fatalf("install result %#v", result)
	}
}

func TestPluginServiceCandidatePickerCancelIsNotError(t *testing.T) {
	service := &PluginService{core: testMCPService(t, func(http.ResponseWriter, *http.Request) { t.Fatal("Core called after picker cancel") }), picker: func(string) (string, error) { return "", nil }}
	result := service.ChooseCandidate(context.Background(), PluginCandidatePickerInput{Kind: "install", SourceType: "folder"})
	if !result.Cancelled || result.Error != nil || result.Candidate != nil {
		t.Fatalf("cancel result %#v", result)
	}
}

func TestPluginServiceCandidateRequiresNativeBootstrap(t *testing.T) {
	service := &PluginService{
		core: testMCPService(t, func(http.ResponseWriter, *http.Request) {
			t.Fatal("Core HTTP called without native Desktop credential")
		}),
		picker: func(string) (string, error) {
			return "/private/plugin.zip", nil
		},
		bootstrap: func(context.Context, string) (string, error) {
			return "", errors.New("native bootstrap unavailable")
		},
	}
	result := service.ChooseCandidate(context.Background(), PluginCandidatePickerInput{Kind: "install", SourceType: "zip"})
	if result.Error == nil || result.Error.Code != "DESKTOP_CONTROL_UNAUTHORIZED" || result.Error.Category != ErrorCategoryAuthentication {
		t.Fatalf("bootstrap failure %#v", result)
	}
}
