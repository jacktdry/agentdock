package app

import (
	"context"
	"encoding/json"
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
