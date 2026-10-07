package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/uvwt/agentdock/internal/envstore"
	"github.com/uvwt/agentdock/internal/permission"
	toolmcp "github.com/uvwt/agentdock/internal/tool/mcp"
	"strings"
	"testing"
)

func TestRuntimeDesktopSecretAdmissionAndDispatch(t *testing.T) {
	rt := newPermissionRuntime(t)
	ctx := context.Background()
	snapshot, err := rt.RuntimeMCPDesktop(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RuntimeMCPDesktopManage(ctx, map[string]any{"action": "desktop_create", "name": "demo", "description": "Demo", "transport": "stdio", "command": "never-run", "expected_registry_revision": snapshot["registry_revision"]})
	if err != nil {
		t.Fatal(err)
	}
	env, err := rt.RuntimeMCPDesktopManage(ctx, map[string]any{"action": "desktop_env_snapshot", "name": "demo"})
	if err != nil {
		t.Fatal(err)
	}
	const canary = "DESKTOP_ENV_SECRET_CANARY_123456789"
	args := map[string]any{"action": "desktop_env_set", "name": "demo", "key": "KEY", "value": canary, "expected_env_revision": env["env_revision"]}
	body, _ := json.Marshal(args)
	request, err := toolmcp.DecodeDesktopRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := desktopAdmissionDescriptor(request)
	fingerprint, err := hostOperationFingerprint("runtime_mcp", request.Action, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	other := "different secret"
	request.Value = &other
	otherFingerprint, _ := hostOperationFingerprint("runtime_mcp", request.Action, desktopAdmissionDescriptor(request))
	if fingerprint != otherFingerprint {
		t.Fatal("secret influenced admission fingerprint")
	}
	for _, field := range []string{"name", "key", "expected_registry_revision", "expected_generation", "expected_env_revision"} {
		changed := desktopAdmissionDescriptor(request)
		changed[field] = "changed"
		next, _ := hostOperationFingerprint("runtime_mcp", request.Action, changed)
		if next == fingerprint {
			t.Fatalf("fingerprint did not bind %s", field)
		}
	}
	replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Rules })
	_, err = rt.RuntimeMCPDesktopManage(ctx, args)
	denied := requirePermissionError(t, err, "APPROVAL_REQUIRED")
	history, _ := rt.permissions.History()
	assertSafe := func(value any) {
		t.Helper()
		data, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if strings.Contains(string(data), canary) {
			t.Fatal("secret escaped write-only dispatch")
		}
	}
	assertSafe([]any{descriptor, fingerprint, denied, history, rt.execution.Snapshot()})
	store, err := envstore.New(rt.cfg.AgentDockHome)
	if err != nil {
		t.Fatal(err)
	}
	values, err := store.Load(envstore.Scope{Kind: envstore.ScopeMCP, Name: "demo"})
	if err != nil || values["KEY"] != "" {
		t.Fatal("mutation ran before approval")
	}
	replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Full })
	result, err := rt.RuntimeMCPDesktopManage(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	values, err = store.Load(envstore.Scope{Kind: envstore.ScopeMCP, Name: "demo"})
	if err != nil || values["KEY"] != canary {
		t.Fatal("DesktopManage did not persist intended value")
	}
	assertSafe([]any{result, rt.execution.Snapshot()})
	if _, err := rt.RuntimeMCPManage(ctx, args); err == nil {
		t.Fatal("legacy runtime admitted Desktop action")
	}
	if _, err := rt.Call(ctx, toolmcp.ToolManage, args); err == nil {
		t.Fatal("model tool admitted Desktop action")
	}
	for _, name := range rt.ToolNames() {
		if strings.Contains(name, "desktop") {
			t.Fatalf("Desktop exposed as tool: %s", name)
		}
	}
}

func TestRuntimeDesktopPassiveAndMutationAdmission(t *testing.T) {
	rt := newPermissionRuntime(t)
	replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Rules })
	before, _ := json.Marshal(rt.execution.Snapshot())
	for _, action := range []string{"desktop_snapshot", "desktop_inspect", "desktop_env_snapshot", "desktop_auth_status"} {
		_, _ = rt.RuntimeMCPDesktopManage(context.Background(), map[string]any{"action": action, "name": "missing"})
	}
	after, _ := json.Marshal(rt.execution.Snapshot())
	if string(before) != string(after) {
		t.Fatal("passive reads entered execution admission")
	}
	for _, action := range []string{"desktop_create", "desktop_update", "desktop_remove", "desktop_set_enabled", "desktop_env_set", "desktop_env_unset", "desktop_env_purge", "desktop_reconnect", "desktop_authorize", "desktop_auth_clear"} {
		_, err := rt.RuntimeMCPDesktopManage(context.Background(), map[string]any{"action": action, "name": "missing"})
		requirePermissionError(t, err, "APPROVAL_REQUIRED")
	}
	for _, args := range []map[string]any{{"action": "add"}, {"action": "desktop_snapshot", "unknown": "secret"}} {
		if _, err := rt.RuntimeMCPDesktopManage(context.Background(), args); err == nil {
			t.Fatal("invalid direct request accepted")
		}
	}
}

func TestDesktopDescriptorOmitsRawConfiguration(t *testing.T) {
	value := "CONFIG_SECRET_CANARY"
	d := desktopAdmissionDescriptor(toolmcp.DesktopManageRequest{Action: "desktop_create", URL: "https://example.test/mcp?token=" + value, Args: []string{"--token", value}, Value: &value})
	data, _ := json.Marshal(d)
	if strings.Contains(string(data), value) || strings.Contains(string(data), "?token") {
		t.Fatal("raw configuration reached admission")
	}
}

func TestRuntimeDesktopOperationJournalReplaysAndRejectsConflictingReuse(t *testing.T) {
	rt := newPermissionRuntime(t)
	replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Full })
	ctx := context.Background()
	snapshot, err := rt.RuntimeMCPDesktop(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const requestID = "0123456789abcdef0123456789abcdef"
	args := map[string]any{
		"action": "desktop_create", "request_id": requestID,
		"name": "journal-demo", "description": "Journal demo", "transport": "stdio", "command": "never-run",
		"expected_registry_revision": snapshot["registry_revision"],
	}
	first, err := rt.RuntimeMCPDesktopManage(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if first["request_id"] != requestID || first["outcome"] != "completed" {
		t.Fatalf("first result = %#v", first)
	}
	second, err := rt.RuntimeMCPDesktopManage(ctx, args)
	if err != nil {
		t.Fatalf("idempotent replay failed: %v", err)
	}
	if second["request_id"] != requestID || second["outcome"] != "completed" {
		t.Fatalf("replayed result = %#v", second)
	}

	status, err := rt.RuntimeMCPDesktopManage(ctx, map[string]any{"action": "desktop_operation_status", "request_id": requestID})
	if err != nil {
		t.Fatal(err)
	}
	if status["found"] != true || status["pending"] != false || status["outcome"] != "completed" || status["operation_action"] != "desktop_create" {
		t.Fatalf("operation status = %#v", status)
	}

	conflicting := map[string]any{
		"action": "desktop_remove", "request_id": requestID, "name": "journal-demo",
		"expected_registry_revision": first["registry_revision"],
	}
	_, err = rt.RuntimeMCPDesktopManage(ctx, conflicting)
	var typed *ToolError
	if !errors.As(err, &typed) || typed.Code != "MCP_OPERATION_ID_CONFLICT" {
		t.Fatalf("conflicting request id err = %#v", err)
	}

	unknown, err := rt.RuntimeMCPDesktopManage(ctx, map[string]any{
		"action": "desktop_operation_status", "request_id": "ffffffffffffffffffffffffffffffff",
	})
	if err != nil {
		t.Fatal(err)
	}
	if unknown["found"] != false || unknown["outcome"] != "outcome_unknown" {
		t.Fatalf("unknown operation = %#v", unknown)
	}
}

func TestDesktopOperationJournalDropsEphemeralAuthorizationURL(t *testing.T) {
	const authURL = "https://provider.example/authorize?state=secret-state"
	result := cloneDesktopMCPJournalResult(Result{
		"action": "desktop_authorize", "flow_id": "flow-safe", "authorization_url": authURL,
	})
	if result["flow_id"] != "flow-safe" {
		t.Fatalf("journal lost flow id: %#v", result)
	}
	if _, exists := result["authorization_url"]; exists {
		t.Fatalf("journal retained ephemeral authorization URL: %#v", result)
	}
}

func TestRuntimeDesktopOperationJournalNeverStoresWriteOnlyEnvironmentValue(t *testing.T) {
	rt := newPermissionRuntime(t)
	replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Full })
	ctx := context.Background()
	snapshot, err := rt.RuntimeMCPDesktop(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const createID = "11111111111111111111111111111111"
	created, err := rt.RuntimeMCPDesktopManage(ctx, map[string]any{
		"action": "desktop_create", "request_id": createID,
		"name": "secret-demo", "description": "Secret demo", "transport": "stdio", "command": "never-run",
		"expected_registry_revision": snapshot["registry_revision"],
	})
	if err != nil {
		t.Fatal(err)
	}
	server, _ := created["server"].(map[string]any)
	generation, _ := server["generation"].(string)
	env, err := rt.RuntimeMCPDesktopManage(ctx, map[string]any{
		"action": "desktop_env_snapshot", "name": "secret-demo",
		"expected_registry_revision": created["registry_revision"], "expected_generation": generation,
	})
	if err != nil {
		t.Fatal(err)
	}

	const secret = "JOURNAL_SECRET_CANARY_123456789"
	const envID = "22222222222222222222222222222222"
	_, err = rt.RuntimeMCPDesktopManage(ctx, map[string]any{
		"action": "desktop_env_set", "request_id": envID, "name": "secret-demo",
		"key": "TOKEN", "value": secret,
		"expected_registry_revision": created["registry_revision"], "expected_generation": generation,
		"expected_env_revision": env["env_revision"],
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := rt.RuntimeMCPDesktopManage(ctx, map[string]any{"action": "desktop_operation_status", "request_id": envID})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("operation journal leaked write-only secret: %s", encoded)
	}
}
