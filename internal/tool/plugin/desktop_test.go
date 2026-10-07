package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/envstore"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	pluginruntime "github.com/uvwt/agentdock/internal/plugin"
	toolcore "github.com/uvwt/agentdock/internal/tool/core"
)

func desktopFixture(t *testing.T) (*Service, string) {
	t.Helper()
	home := t.TempDir()
	manager, err := pluginruntime.NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	envs, err := envstore.New(home)
	if err != nil {
		t.Fatal(err)
	}
	clients, err := mcpclient.NewManager(home, envs)
	if err != nil {
		t.Fatal(err)
	}
	service := New(config.Config{AgentDockHome: home}, manager, clients, envs, nil)
	t.Cleanup(func() { _ = clients.Close(); service.ReleaseMCPLeases() })
	source := t.TempDir()
	writePluginServiceJSON(t, filepath.Join(source, "plugin.json"), map[string]any{"name": "desktop-demo", "version": "1.0.0", "description": "Safe Plugin"})
	writePluginServiceJSON(t, filepath.Join(source, "mcp.json"), map[string]any{"mcpServers": map[string]any{
		"one": map[string]any{"type": "stdio", "command": "never-run-p6", "env": map[string]string{"TOKEN": "${TOKEN}", "OPTIONAL": "${OPTIONAL:-}"}},
		"two": map[string]any{"type": "streamable-http", "url": "https://never-connect.test/mcp?secret=URL_CANARY", "headers": map[string]string{"Authorization": "${SECOND_TOKEN}"}},
	}})
	installDesktopFixture(t, manager, source)
	return service, source
}
func installDesktopFixture(t *testing.T, manager *pluginruntime.Manager, source string) {
	t.Helper()
	review := manager.Validate(source)
	if !review.Valid {
		t.Fatalf("invalid fixture: %#v", review)
	}
	result, err := manager.InstallReviewedSource(context.Background(), source, false, review.ReviewToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.FinalizeActivation(result.Name); err != nil {
		t.Fatal(err)
	}
}
func desktopRequest(t *testing.T, s *Service, action string) DesktopManageRequest {
	t.Helper()
	reg, err := s.manager.Store().DesktopRegistrySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	return DesktopManageRequest{Action: action, Name: "desktop-demo", ExpectedRegistryRevision: reg.Revision, ExpectedGeneration: pluginruntime.DesktopGeneration(reg.States[0])}
}
func requireDesktopCode(t *testing.T, err error, code string) {
	t.Helper()
	var typed *toolcore.ToolError
	if !errors.As(err, &typed) || typed.Code != code {
		t.Fatalf("error=%v want %s", err, code)
	}
	data, _ := json.Marshal(typed)
	if strings.Contains(string(data), "CANARY") || strings.Contains(string(data), "/private/") {
		t.Fatalf("unsafe error %s", data)
	}
}
func TestDesktopPluginPassiveAndFencing(t *testing.T) {
	s, _ := desktopFixture(t)
	ctx := context.Background()
	before, _ := s.mcpClients.DesktopSnapshot()
	for _, action := range []string{"desktop_snapshot", "desktop_inspect", "desktop_env_snapshot"} {
		result, err := s.DesktopManage(ctx, DesktopManageRequest{Action: action, Name: "desktop-demo", Component: "two"})
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(result)
		for _, forbidden := range []string{s.manager.Store().Home(), "storage_key", "runtime_name", "URL_CANARY", "review_token"} {
			if strings.Contains(string(data), forbidden) {
				t.Fatalf("leak %s: %s", forbidden, data)
			}
		}
	}
	after, _ := s.mcpClients.DesktopSnapshot()
	if len(after.Servers) != len(before.Servers) {
		t.Fatal("passive read reconciled MCP")
	}
	entries, _ := os.ReadDir(filepath.Join(s.manager.Store().Home(), "run", "plugins"))
	if len(entries) != 0 {
		t.Fatal("passive read created runtime")
	}
	request := desktopRequest(t, s, "desktop_set_enabled")
	enabled := true
	request.Enabled = &enabled
	_, err := s.DesktopManage(ctx, request)
	requireDesktopCode(t, err, "PLUGIN_CREDENTIAL_REQUIRED")
	request.ExpectedGeneration = strings.Repeat("0", 64)
	_, err = s.DesktopManage(ctx, request)
	requireDesktopCode(t, err, "PLUGIN_GENERATION_CONFLICT")
	request = desktopRequest(t, s, "desktop_remove_keep")
	request.ExpectedRegistryRevision = strings.Repeat("0", 64)
	_, err = s.DesktopManage(ctx, request)
	requireDesktopCode(t, err, "PLUGIN_REGISTRY_CONFLICT")
}
func TestDesktopPluginEnvironmentIsolationRevisionsAndNoLeak(t *testing.T) {
	s, _ := desktopFixture(t)
	ctx := context.Background()
	snapshot, err := s.DesktopManage(ctx, DesktopManageRequest{Action: "desktop_env_snapshot", Name: "desktop-demo", Component: "one"})
	if err != nil {
		t.Fatal(err)
	}
	entries := snapshot["items"].([]envstore.Entry)
	if len(entries) != 2 {
		t.Fatalf("optional declaration missing: %#v", entries)
	}
	request := desktopRequest(t, s, "desktop_env_set")
	request.Component = "one"
	request.Key = "TOKEN"
	secret := "SECRET_CANARY"
	request.Value = &secret
	request.ExpectedEnvRevision = snapshot["env_revision"].(string)
	result, err := s.DesktopManage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), secret) || strings.Contains(string(data), pluginruntime.RuntimeMCPName("desktop-demo", "one")) {
		t.Fatalf("unsafe result %s", data)
	}
	_, err = s.DesktopManage(ctx, request)
	requireDesktopCode(t, err, "PLUGIN_ENV_CONFLICT")
	request.ExpectedEnvRevision = result["env_revision"].(string)
	request.Key = "SECOND_TOKEN"
	_, err = s.DesktopManage(ctx, request)
	requireDesktopCode(t, err, "PLUGIN_ENV_OWNERSHIP_INVALID")
	other, err := s.envs.Load(envstore.Scope{Kind: envstore.ScopeMCP, Name: pluginruntime.RuntimeMCPName("desktop-demo", "two")})
	if err != nil || len(other) != 0 {
		t.Fatal("cross component write")
	}
	// Unconditional model/env-store writes also advance the shared revision.
	_ = s.envs.Set(envstore.Scope{Kind: envstore.ScopeMCP, Name: pluginruntime.RuntimeMCPName("desktop-demo", "one")}, "TOKEN", "MODEL_CANARY")
	request.Key = "TOKEN"
	request.Action = "desktop_env_unset"
	request.Value = nil
	_, err = s.DesktopManage(ctx, request)
	requireDesktopCode(t, err, "PLUGIN_ENV_CONFLICT")
	// Forged persisted ownership never redirects a write to standalone state.
	state, _ := s.manager.Store().Load("desktop-demo")
	state.Components.MCP[0].StorageKey = "standalone-victim"
	if err := s.manager.Store().Save(state); err != nil {
		t.Fatal(err)
	}
	_, err = s.DesktopManage(ctx, DesktopManageRequest{Action: "desktop_env_snapshot", Name: "desktop-demo", Component: "one"})
	requireDesktopCode(t, err, "PLUGIN_ENV_OWNERSHIP_INVALID")
}
func TestDesktopPluginRemoveKeepPurgeAndIncarnation(t *testing.T) {
	s, source := desktopFixture(t)
	ctx := context.Background()
	initial := desktopRequest(t, s, "desktop_remove_keep")
	dataRoot, err := s.manager.EnsureDataDir("desktop-demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "saved"), []byte("DATA_CANARY"), 0600); err != nil {
		t.Fatal(err)
	}
	scope := envstore.Scope{Kind: envstore.ScopeMCP, Name: pluginruntime.RuntimeMCPName("desktop-demo", "one")}
	if err := s.envs.Set(scope, "TOKEN", "ENV_CANARY"); err != nil {
		t.Fatal(err)
	}
	result, err := s.DesktopManage(ctx, initial)
	if err != nil {
		t.Fatal(err)
	}
	if result["data_policy"] != "keep" || result["data_preserved"] != true || result["completed"] != true {
		t.Fatalf("keep contract %#v", result)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "saved")); err != nil {
		t.Fatal("keep lost data")
	}
	values, _ := s.envs.Load(scope)
	if values["TOKEN"] != "ENV_CANARY" {
		t.Fatal("keep lost environment")
	}
	installDesktopFixture(t, s.manager, source)
	current := desktopRequest(t, s, "desktop_remove_purge")
	if initial.ExpectedGeneration == current.ExpectedGeneration {
		t.Fatal("same-name reinstall reused generation")
	}
	stale := current
	stale.ExpectedGeneration = initial.ExpectedGeneration
	_, err = s.DesktopManage(ctx, stale)
	requireDesktopCode(t, err, "PLUGIN_GENERATION_CONFLICT")
	result, err = s.DesktopManage(ctx, current)
	if err != nil {
		t.Fatal(err)
	}
	if result["data_policy"] != "purge" || result["data_preserved"] != false || result["completed"] != true {
		t.Fatalf("purge contract %#v", result)
	}
	if _, err := os.Stat(dataRoot); !os.IsNotExist(err) {
		t.Fatal("purge retained data")
	}
	values, _ = s.envs.Load(scope)
	if len(values) != 0 {
		t.Fatal("purge retained environment")
	}
}
func TestDesktopPluginConcurrentRemovalFencesExactlyOne(t *testing.T) {
	s, _ := desktopFixture(t)
	first := desktopRequest(t, s, "desktop_remove_keep")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.DesktopManage(context.Background(), first); results <- err }()
	}
	wg.Wait()
	close(results)
	success := 0
	conflict := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			var typed *toolcore.ToolError
			if errors.As(err, &typed) && typed.Code == "PLUGIN_REGISTRY_CONFLICT" {
				conflict++
			} else {
				t.Fatal(err)
			}
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}
func TestDesktopPluginRequestBoundary(t *testing.T) {
	for _, body := range []string{`{"action":"install"}`, `{"action":"desktop_snapshot","source":"/private/CANARY"}`, `{"action":"desktop_env_set","name":"demo","storage_key":"victim"}`, `{"action":"desktop_inspect","name":"/private/CANARY"}`, `{"action":"desktop_snapshot","value":"SECRET_CANARY"}`} {
		_, err := DecodeDesktopRequest([]byte(body))
		requireDesktopCode(t, err, "INVALID_PLUGIN_REQUEST")
	}
}

func TestDecodeDesktopPluginRequestStrictFields(t *testing.T) {
	revision := strings.Repeat("a", 64)
	generation := strings.Repeat("b", 64)
	envRevision := strings.Repeat("c", 64)
	requestID := strings.Repeat("d", 32)
	candidateID := strings.Repeat("e", 64)
	valid := []string{
		`{"action":"desktop_snapshot"}`,
		`{"action":"desktop_inspect","name":"demo"}`,
		`{"action":"desktop_operation_status","request_id":"` + requestID + `"}`,
		`{"action":"desktop_install_candidate","request_id":"` + requestID + `","candidate_id":"` + candidateID + `","name":"demo","expected_registry_revision":"` + revision + `"}`,
		`{"action":"desktop_update_candidate","request_id":"` + requestID + `","candidate_id":"` + candidateID + `","name":"demo","expected_registry_revision":"` + revision + `","expected_generation":"` + generation + `"}`,
		`{"action":"desktop_set_enabled","request_id":"` + requestID + `","name":"demo","enabled":true,"expected_registry_revision":"` + revision + `","expected_generation":"` + generation + `"}`,
		`{"action":"desktop_remove_keep","request_id":"` + requestID + `","name":"demo","expected_registry_revision":"` + revision + `","expected_generation":"` + generation + `"}`,
		`{"action":"desktop_env_snapshot","name":"demo","component":"one"}`,
		`{"action":"desktop_env_set","request_id":"` + requestID + `","name":"demo","component":"one","key":"TOKEN","value":"","expected_registry_revision":"` + revision + `","expected_generation":"` + generation + `","expected_env_revision":"` + envRevision + `"}`,
		`{"action":"desktop_env_unset","request_id":"` + requestID + `","name":"demo","component":"one","key":"TOKEN","expected_registry_revision":"` + revision + `","expected_generation":"` + generation + `","expected_env_revision":"` + envRevision + `"}`,
	}
	for _, body := range valid {
		if _, err := DecodeDesktopRequest([]byte(body)); err != nil {
			t.Fatalf("valid body rejected: %s: %v", body, err)
		}
	}

	invalid := []string{
		`{"action":"desktop_snapshot","name":""}`,
		`{"action":"desktop_inspect","name":"demo","request_id":"` + requestID + `"}`,
		`{"action":"desktop_operation_status"}`,
		`{"action":"desktop_operation_status","request_id":"short"}`,
		`{"action":"desktop_install_candidate","request_id":"` + requestID + `","candidate_id":"short","name":"demo","expected_registry_revision":"` + revision + `"}`,
		`{"action":"desktop_install_candidate","request_id":"` + requestID + `","candidate_id":"` + candidateID + `","name":"demo","expected_registry_revision":"` + revision + `","expected_generation":"` + generation + `"}`,
		`{"action":"desktop_update_candidate","request_id":"` + requestID + `","candidate_id":"` + candidateID + `","name":"demo","expected_registry_revision":"` + revision + `"}`,
		`{"action":"desktop_set_enabled","request_id":"` + requestID + `","name":"demo","enabled":true,"expected_registry_revision":"` + revision + `"}`,
		`{"action":"desktop_remove_keep","request_id":"` + requestID + `","name":"demo","expected_registry_revision":"` + revision + `","expected_generation":"` + generation + `","enabled":false}`,
		`{"action":"desktop_env_snapshot","name":"demo"}`,
		`{"action":"desktop_env_snapshot","name":"demo","component":"one","expected_generation":"` + generation + `"}`,
		`{"action":"desktop_env_set","request_id":"` + requestID + `","name":"demo","component":"one","key":"TOKEN","value":"secret","expected_registry_revision":"` + revision + `","expected_generation":"` + generation + `"}`,
		`{"action":"desktop_env_unset","request_id":"` + requestID + `","name":"demo","component":"one","key":"TOKEN","value":"","expected_registry_revision":"` + revision + `","expected_generation":"` + generation + `","expected_env_revision":"` + envRevision + `"}`,
		`{"Action":"desktop_snapshot"}`,
	}
	for _, body := range invalid {
		if _, err := DecodeDesktopRequest([]byte(body)); err == nil {
			t.Fatalf("invalid body accepted: %s", body)
		}
	}
}

func TestDesktopPluginPurgeRecoveryAndRetry(t *testing.T) {
	s, _ := desktopFixture(t)
	ctx := context.Background()
	scope := envstore.Scope{Kind: envstore.ScopeMCP, Name: pluginruntime.RuntimeMCPName("desktop-demo", "one")}
	path, err := s.envs.Path(scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "blocked"), []byte("CANARY"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := s.DesktopManage(ctx, desktopRequest(t, s, "desktop_remove_purge"))
	if err != nil {
		t.Fatal(err)
	}
	if result["completed"] != false || result["persisted"] != true || result["recovery_required"] != true || result["runtime_impact"] != "cleanup_pending" {
		t.Fatalf("purge failure must preserve durable cleanup truth: %#v", result)
	}
	snapshot, err := s.DesktopManage(ctx, DesktopManageRequest{Action: "desktop_snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot["plugins"].([]DesktopItem)) != 0 {
		t.Fatal("purging Plugin displayed as installed")
	}
	recovery := snapshot["recovery_items"].([]DesktopRecoveryItem)[0]
	retry := DesktopManageRequest{Name: recovery.Name, ExpectedGeneration: recovery.Generation, ExpectedRegistryRevision: snapshot["registry_revision"].(string)}
	retry.Action = "desktop_set_enabled"
	enabled := true
	retry.Enabled = &enabled
	_, err = s.DesktopManage(ctx, retry)
	requireDesktopCode(t, err, "PLUGIN_RECOVERY_REQUIRED")
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	retry.Action = "desktop_remove_purge"
	retry.Enabled = nil
	result, err = s.DesktopManage(ctx, retry)
	if err != nil || result["completed"] != true {
		t.Fatalf("retry %#v %v", result, err)
	}
	snapshot, err = s.DesktopManage(ctx, DesktopManageRequest{Action: "desktop_snapshot"})
	if err != nil || len(snapshot["recovery_items"].([]DesktopRecoveryItem)) != 0 {
		t.Fatal("completed purge retains recovery")
	}
}
func TestDesktopPluginEnableDisableGenerationAndRevisionABA(t *testing.T) {
	s, _ := desktopFixture(t)
	ctx := context.Background()
	for _, component := range []string{"one", "two"} {
		key := "TOKEN"
		if component == "two" {
			key = "SECOND_TOKEN"
		}
		if err := s.envs.Set(envstore.Scope{Kind: envstore.ScopeMCP, Name: pluginruntime.RuntimeMCPName("desktop-demo", component)}, key, "CANARY"); err != nil {
			t.Fatal(err)
		}
	}
	request := desktopRequest(t, s, "desktop_set_enabled")
	original := request
	enabled := true
	request.Enabled = &enabled
	result, err := s.DesktopManage(ctx, request)
	if err != nil || result["completed"] != true {
		t.Fatalf("enable %#v %v", result, err)
	}
	next := desktopRequest(t, s, "desktop_set_enabled")
	if next.ExpectedGeneration != original.ExpectedGeneration || next.ExpectedRegistryRevision == original.ExpectedRegistryRevision {
		t.Fatal("enable changed incarnation or failed revision advance")
	}
	enabled = false
	next.Enabled = &enabled
	result, err = s.DesktopManage(ctx, next)
	if err != nil || result["completed"] != true {
		t.Fatalf("disable %#v %v", result, err)
	}
	final := desktopRequest(t, s, "desktop_set_enabled")
	if final.ExpectedRegistryRevision == original.ExpectedRegistryRevision || final.ExpectedGeneration != original.ExpectedGeneration {
		t.Fatal("ABA revived stale revision or changed incarnation")
	}
	original.Enabled = &enabled
	_, err = s.DesktopManage(ctx, original)
	requireDesktopCode(t, err, "PLUGIN_REGISTRY_CONFLICT")
}

func TestDesktopPluginDisableAndKeepPreserveAuthorizations(t *testing.T) {
	s, source := desktopFixture(t)
	ctx := context.Background()
	for _, component := range []string{"one", "two"} {
		key := "TOKEN"
		if component == "two" {
			key = "SECOND_TOKEN"
		}
		if err := s.envs.Set(envstore.Scope{Kind: envstore.ScopeMCP, Name: pluginruntime.RuntimeMCPName("desktop-demo", component)}, key, "CANARY"); err != nil {
			t.Fatal(err)
		}
	}
	enable := func() {
		request := desktopRequest(t, s, "desktop_set_enabled")
		enabled := true
		request.Enabled = &enabled
		if _, err := s.DesktopManage(ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	enable()
	grantPath := filepath.Join(s.manager.Store().Home(), "data", "mcp", "grant-"+pluginruntime.RuntimeMCPName("desktop-demo", "two")+".json")
	grant := []byte(`{"schema_version":1,"endpoint":"https://never-connect.test/mcp?secret=URL_CANARY","access_token":"OAUTH_CANARY"}`)
	if err := os.WriteFile(grantPath, grant, 0600); err != nil {
		t.Fatal(err)
	}
	skillScope := envstore.Scope{Kind: envstore.ScopePluginSkill, Plugin: "desktop-demo", Name: "retained-skill"}
	if err := s.envs.Set(skillScope, "SETTING", "SKILL_CANARY"); err != nil {
		t.Fatal(err)
	}
	disabled := false
	request := desktopRequest(t, s, "desktop_set_enabled")
	request.Enabled = &disabled
	if _, err := s.DesktopManage(ctx, request); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(grantPath)
	if err != nil || string(data) != string(grant) {
		t.Fatal("disable lost authorization")
	}
	enable()
	if _, err := s.DesktopManage(ctx, desktopRequest(t, s, "desktop_remove_keep")); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(grantPath)
	if err != nil || string(data) != string(grant) {
		t.Fatal("remove keep lost authorization")
	}
	values, _ := s.envs.Load(skillScope)
	if values["SETTING"] != "SKILL_CANARY" {
		t.Fatal("keep lost Skill environment")
	}
	installDesktopFixture(t, s.manager, source)
	if _, err := s.DesktopManage(ctx, desktopRequest(t, s, "desktop_remove_purge")); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(grantPath)
	if strings.Contains(string(data), "OAUTH_CANARY") {
		t.Fatal("purge retained authorization")
	}
	values, _ = s.envs.Load(skillScope)
	if len(values) != 0 {
		t.Fatal("purge retained Skill environment")
	}
}

func TestDesktopPluginEnvironmentRejectsStandaloneStorageAlias(t *testing.T) {
	s, _ := desktopFixture(t)
	ctx := context.Background()
	snapshot, err := s.DesktopManage(ctx, DesktopManageRequest{Action: "desktop_env_snapshot", Name: "desktop-demo", Component: "one"})
	if err != nil {
		t.Fatal(err)
	}
	key := pluginruntime.RuntimeMCPName("desktop-demo", "one")
	if _, err := s.mcpClients.Add(mcpclient.ServerConfig{Name: key, Description: "Standalone fixture", Transport: "stdio", Command: "never-run", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	scope := envstore.Scope{Kind: envstore.ScopeMCP, Name: key}
	if err := s.envs.Set(scope, "TOKEN", "STANDALONE_CANARY"); err != nil {
		t.Fatal(err)
	}
	request := desktopRequest(t, s, "desktop_env_set")
	request.Component = "one"
	request.Key = "TOKEN"
	value := "PLUGIN_CANARY"
	request.Value = &value
	request.ExpectedEnvRevision = snapshot["env_revision"].(string)
	_, err = s.DesktopManage(ctx, request)
	requireDesktopCode(t, err, "PLUGIN_ENV_OWNERSHIP_INVALID")
	values, _ := s.envs.Load(scope)
	if values["TOKEN"] != "STANDALONE_CANARY" {
		t.Fatal("Plugin overwrote standalone credentials")
	}
}

func TestDesktopPluginEnvironmentSuccessfulUnsetIsWriteOnly(t *testing.T) {
	s, _ := desktopFixture(t)
	ctx := context.Background()
	scope := envstore.Scope{Kind: envstore.ScopeMCP, Name: pluginruntime.RuntimeMCPName("desktop-demo", "one")}
	if err := s.envs.Set(scope, "TOKEN", "SECRET_CANARY"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.DesktopManage(ctx, DesktopManageRequest{Action: "desktop_env_snapshot", Name: "desktop-demo", Component: "one"})
	if err != nil {
		t.Fatal(err)
	}
	request := desktopRequest(t, s, "desktop_env_unset")
	request.Component = "one"
	request.Key = "TOKEN"
	request.ExpectedEnvRevision = snapshot["env_revision"].(string)
	result, err := s.DesktopManage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if result["env_revision"] == snapshot["env_revision"] {
		t.Fatal("unset did not advance revision")
	}
	values, _ := s.envs.Load(scope)
	if _, present := values["TOKEN"]; present {
		t.Fatal("unset retained value")
	}
	for _, entry := range result["items"].([]envstore.Entry) {
		if entry.Key == "TOKEN" && entry.Configured {
			t.Fatal("unset projection still configured")
		}
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), "SECRET_CANARY") {
		t.Fatal("unset leaked old value")
	}
}
func TestDesktopPluginPurgeNeverDeletesStandaloneAliasCredentials(t *testing.T) {
	s, _ := desktopFixture(t)
	ctx := context.Background()
	key := pluginruntime.RuntimeMCPName("desktop-demo", "one")
	if _, err := s.mcpClients.Add(mcpclient.ServerConfig{Name: key, Description: "Standalone fixture", Transport: "stdio", Command: "never-run", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	scope := envstore.Scope{Kind: envstore.ScopeMCP, Name: key}
	if err := s.envs.Set(scope, "TOKEN", "STANDALONE_CANARY"); err != nil {
		t.Fatal(err)
	}
	result, err := s.DesktopManage(ctx, desktopRequest(t, s, "desktop_remove_purge"))
	if err != nil {
		t.Fatal(err)
	}
	if result["completed"] != false || result["recovery_required"] != true {
		t.Fatalf("purge should require collision recovery: %#v", result)
	}
	values, _ := s.envs.Load(scope)
	if values["TOKEN"] != "STANDALONE_CANARY" {
		t.Fatal("purge deleted standalone credentials")
	}
}

func TestDesktopPluginPassiveEnabledSnapshotNeverReconciles(t *testing.T) {
	s, _ := desktopFixture(t)
	ctx := context.Background()
	for _, component := range []string{"one", "two"} {
		key := "TOKEN"
		if component == "two" {
			key = "SECOND_TOKEN"
		}
		if err := s.envs.Set(envstore.Scope{Kind: envstore.ScopeMCP, Name: pluginruntime.RuntimeMCPName("desktop-demo", component)}, key, "CANARY"); err != nil {
			t.Fatal(err)
		}
	}
	// Persist enabled state without runtime application to distinguish a passive
	// observation from accidentally reconciling or repairing cached runtime.
	if _, err := s.manager.SetEnabled(ctx, "desktop-demo", true); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"desktop_snapshot", "desktop_inspect", "desktop_env_snapshot"} {
		if _, err := s.DesktopManage(ctx, DesktopManageRequest{Action: action, Name: "desktop-demo", Component: "one"}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := s.mcpClients.DesktopSnapshot()
	if err != nil || len(snapshot.Servers) != 0 {
		t.Fatal("passive observation activated owned runtime")
	}
	entries, _ := os.ReadDir(filepath.Join(s.manager.Store().Home(), "run", "plugins"))
	if len(entries) != 0 {
		t.Fatal("passive observation created executable runtime snapshot")
	}
}
