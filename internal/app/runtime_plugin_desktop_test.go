package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	"github.com/uvwt/agentdock/internal/permission"
	pluginruntime "github.com/uvwt/agentdock/internal/plugin"
	toolplugin "github.com/uvwt/agentdock/internal/tool/plugin"
)

func runtimeDesktopPluginFixture(t *testing.T) (*Runtime, map[string]any) {
	t.Helper()
	rt := newPermissionRuntime(t)
	ctx := context.Background()
	source := writeAppPluginForTest(t, rt.ws.Root(), "1.0.0")
	writeAppPluginJSON(t, filepath.Join(source, "mcp.json"), map[string]any{"mcpServers": map[string]any{"remote": map[string]any{"type": "streamable-http", "url": "https://never-connect.test", "headers": map[string]string{"Authorization": "${TOKEN}"}}}})
	validated, err := rt.Call(ctx, "plugin_manage", map[string]any{"action": "validate", "source": source})
	if err != nil {
		t.Fatal(err)
	}
	review := validated["review"].(pluginruntime.Review)
	_, err = rt.Call(ctx, "plugin_manage", map[string]any{"action": "install", "source": source, "enabled": false, "review_token": review.ReviewToken})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := rt.RuntimePluginDesktop(ctx)
	if err != nil {
		t.Fatal(err)
	}
	item := snapshot["plugins"].([]toolplugin.DesktopItem)[0]
	env, err := rt.RuntimePluginDesktopManage(ctx, map[string]any{"action": "desktop_env_snapshot", "name": item.Name, "component": "remote"})
	if err != nil {
		t.Fatal(err)
	}
	return rt, map[string]any{"action": "desktop_env_set", "request_id": strings.Repeat("a", 32), "name": item.Name, "component": "remote", "key": "TOKEN", "value": "SECRET_CANARY",
		"expected_registry_revision": snapshot["registry_revision"], "expected_generation": item.Generation, "expected_env_revision": env["env_revision"]}
}
func TestRuntimeDesktopPluginApprovalFingerprintJournalAndNoLeak(t *testing.T) {
	rt, args := runtimeDesktopPluginFixture(t)
	body, _ := json.Marshal(args)
	request, err := toolplugin.DecodeDesktopRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := rt.desktopPluginRequestFingerprint(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*toolplugin.DesktopManageRequest){
		func(r *toolplugin.DesktopManageRequest) { r.CandidateID = strings.Repeat("c", 64) },
		func(r *toolplugin.DesktopManageRequest) { other := "OTHER_CANARY"; r.Value = &other },
		func(r *toolplugin.DesktopManageRequest) { r.Name = "other" }, func(r *toolplugin.DesktopManageRequest) { r.Component = "other" },
		func(r *toolplugin.DesktopManageRequest) { r.Key = "OTHER" }, func(r *toolplugin.DesktopManageRequest) { r.ExpectedRegistryRevision = "other" },
		func(r *toolplugin.DesktopManageRequest) { r.ExpectedGeneration = "other" }, func(r *toolplugin.DesktopManageRequest) { r.ExpectedEnvRevision = "other" },
	} {
		other := request
		mutate(&other)
		changed, _ := rt.desktopPluginRequestFingerprint(other)
		if changed == fingerprint {
			t.Fatal("semantic field unbound")
		}
	}
	other := request
	other.RequestID = strings.Repeat("b", 32)
	changed, _ := rt.desktopPluginRequestFingerprint(other)
	if changed != fingerprint {
		t.Fatal("request id became approval semantics")
	}
	replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Rules })
	ctx := requestmeta.WithAuthPrincipal(context.Background(), requestmeta.NewStableAuthPrincipal("static_bearer", "plugin-desktop-test"))
	_, err = rt.RuntimePluginDesktopManage(ctx, args)
	approval := requirePermissionError(t, err, "APPROVAL_REQUIRED")
	status, err := rt.RuntimePluginDesktopManage(ctx, map[string]any{"action": "desktop_operation_status", "request_id": args["request_id"]})
	if err != nil || status["found"] != false {
		t.Fatal("approval blocked semantic retry")
	}
	_, err = rt.permissions.ApproveOnce(context.Background(), permission.Mutation{ApprovalID: approval.Details["approval_id"].(string), ApprovalVersion: permissionDetailUint64(t, approval.Details, "approval_version"), PolicyRevision: permissionDetailUint64(t, approval.Details, "policy_revision"), Actor: "test-desktop-control"})
	if err != nil {
		t.Fatal(err)
	}
	// A changed secret cannot consume the exact one-time consent.
	args["value"] = "OTHER_CANARY"
	_, err = rt.RuntimePluginDesktopManage(ctx, args)
	requirePermissionError(t, err, "APPROVAL_REQUIRED")
	args["value"] = "SECRET_CANARY"
	args["request_id"] = strings.Repeat("b", 32)
	result, err := rt.RuntimePluginDesktopManage(ctx, args)
	if err != nil || result["outcome"] != "completed" {
		t.Fatalf("approved result %#v %v", result, err)
	}
	replay, err := rt.RuntimePluginDesktopManage(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	status, err = rt.RuntimePluginDesktopManage(ctx, map[string]any{"action": "desktop_operation_status", "request_id": args["request_id"]})
	if err != nil || status["found"] != true || status["pending"] != false {
		t.Fatal("missing completed journal")
	}
	history, _ := rt.permissions.History()
	data, _ := json.Marshal([]any{desktopPluginAdmissionDescriptor(request), fingerprint, approval, result, replay, status, history, rt.execution.Snapshot()})
	if strings.Contains(string(data), "CANARY") || strings.Contains(string(data), "storage_key") {
		t.Fatalf("secret leaked %s", data)
	}
	args["value"] = "CONFLICT_CANARY"
	_, err = rt.RuntimePluginDesktopManage(ctx, args)
	requirePluginOperationError(t, err, "PLUGIN_OPERATION_ID_CONFLICT")
	if _, err := rt.RuntimeMCPDesktopManage(ctx, args); err == nil {
		t.Fatal("P5 accepted Plugin env scope")
	}
	if _, err := rt.Call(ctx, "plugin_manage", args); err == nil {
		t.Fatal("model contract accepted Desktop request")
	}
}
func TestRuntimeDesktopPluginConcurrentJournal(t *testing.T) {
	rt, args := runtimeDesktopPluginFixture(t)
	replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Full })
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := rt.RuntimePluginDesktopManage(context.Background(), args)
			if err != nil {
				requirePluginOperationError(t, err, "PLUGIN_OPERATION_IN_PROGRESS")
			}
		}()
	}
	wg.Wait()
	status, err := rt.RuntimePluginDesktopManage(context.Background(), map[string]any{"action": "desktop_operation_status", "request_id": args["request_id"]})
	if err != nil || status["outcome"] != "completed" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	rt.pluginDesktopOpsMu.Lock()
	defer rt.pluginDesktopOpsMu.Unlock()
	if len(rt.pluginDesktopOps) != 1 {
		t.Fatalf("journal entries=%d", len(rt.pluginDesktopOps))
	}
}

func TestRuntimeDesktopPluginJournalEvictsOldestCompletedAtCapacity(t *testing.T) {
	now := time.Now().UTC()
	rt := &Runtime{pluginDesktopOps: make(map[string]desktopPluginOperationRecord)}
	for index := 0; index < maxDesktopPluginOperations; index++ {
		id := fmt.Sprintf("%032x", index+1)
		rt.pluginDesktopOps[id] = desktopPluginOperationRecord{
			RequestID:   id,
			Action:      "desktop_set_enabled",
			Target:      "demo",
			Fingerprint: "fingerprint",
			Outcome:     "completed",
			CompletedAt: now.Add(time.Duration(index) * time.Millisecond),
		}
	}

	rt.cleanupDesktopPluginOperationsLocked(now.Add(time.Second))
	if len(rt.pluginDesktopOps) != maxDesktopPluginOperations-1 {
		t.Fatalf("journal entries=%d, want %d", len(rt.pluginDesktopOps), maxDesktopPluginOperations-1)
	}
	oldest := fmt.Sprintf("%032x", 1)
	if _, ok := rt.pluginDesktopOps[oldest]; ok {
		t.Fatal("oldest completed operation was not evicted")
	}

	newID := strings.Repeat("f", 32)
	if _, err, handled := rt.beginDesktopPluginOperation(newID, "desktop_set_enabled", "demo", "new-fingerprint"); err != nil || handled {
		t.Fatalf("begin after eviction handled=%v err=%v", handled, err)
	}
}

func TestRuntimeDesktopPluginCandidateApprovalRetryDoesNotConsumeCandidate(t *testing.T) {
	rt := newPermissionRuntime(t)
	source := writeAppPluginForTest(t, rt.ws.Root(), "1.0.0")
	candidate, err := rt.plugins.PrepareDesktopCandidate(context.Background(), source, "install", "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.plugins.ReleaseDesktopCandidates)
	snapshot, err := rt.RuntimePluginDesktop(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{
		"action": "desktop_install_candidate", "request_id": strings.Repeat("a", 32),
		"candidate_id": candidate.CandidateID, "name": candidate.Review.Name,
		"expected_registry_revision": snapshot["registry_revision"],
	}
	replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Rules })
	ctx := requestmeta.WithAuthPrincipal(context.Background(), requestmeta.NewStableAuthPrincipal("static_bearer", "plugin-candidate-test"))
	_, err = rt.RuntimePluginDesktopManage(ctx, args)
	approval := requirePermissionError(t, err, "APPROVAL_REQUIRED")
	if _, err := rt.permissions.ApproveOnce(context.Background(), permission.Mutation{
		ApprovalID: approval.Details["approval_id"].(string), ApprovalVersion: permissionDetailUint64(t, approval.Details, "approval_version"),
		PolicyRevision: permissionDetailUint64(t, approval.Details, "policy_revision"), Actor: "test-desktop-control",
	}); err != nil {
		t.Fatal(err)
	}

	args["request_id"] = strings.Repeat("b", 32)
	result, err := rt.RuntimePluginDesktopManage(ctx, args)
	if err != nil || result["outcome"] != "completed" {
		t.Fatalf("approved candidate result %#v err=%v", result, err)
	}
	installed, err := rt.RuntimePlugin(context.Background(), candidate.Review.Name)
	if err != nil {
		t.Fatal(err)
	}
	if installed["enabled"] != false {
		t.Fatalf("Desktop candidate install was not disabled: %#v", installed)
	}

	originalBody, _ := json.Marshal(args)
	originalRequest, err := toolplugin.DecodeDesktopRequest(originalBody)
	if err != nil {
		t.Fatal(err)
	}
	original, err := rt.desktopPluginRequestFingerprint(originalRequest)
	if err != nil {
		t.Fatal(err)
	}
	other := originalRequest
	other.CandidateID = strings.Repeat("d", 64)
	changed, err := rt.desktopPluginRequestFingerprint(other)
	if err != nil {
		t.Fatal(err)
	}
	if changed == original {
		t.Fatal("candidate id was not bound into approval semantics")
	}
}

func TestRuntimeDesktopPluginPermissionFactsClassifyLifecycleEffects(t *testing.T) {
	rt := newPermissionRuntime(t)
	for _, test := range []struct {
		action            string
		network, commands bool
	}{
		{action: "desktop_install_candidate"},
		{action: "desktop_update_candidate", network: true, commands: true},
		{action: "desktop_set_enabled", network: true, commands: true},
		{action: "desktop_remove_keep", network: true, commands: true},
		{action: "desktop_remove_purge", network: true, commands: true},
		{action: "desktop_env_set"},
		{action: "desktop_env_unset"},
	} {
		facts := rt.hostOperationFacts(context.Background(), permission.HostOperation{
			Tool: "runtime_plugin", Action: test.action, Source: "internal",
		})
		if !facts.Management || !facts.MCP || !facts.Other || !facts.OneShotEligible {
			t.Fatalf("%s missing management classification: %#v", test.action, facts)
		}
		if facts.Network != test.network || facts.Commands != test.commands {
			t.Fatalf("%s network=%v commands=%v want %v/%v", test.action, facts.Network, facts.Commands, test.network, test.commands)
		}
	}
}

func requirePluginOperationError(t *testing.T, err error, code string) {
	t.Helper()
	var typed *ToolError
	if !errors.As(err, &typed) || typed.Code != code {
		t.Errorf("error=%v want %s", err, code)
	}
}
