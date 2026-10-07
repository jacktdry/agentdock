package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/envstore"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	"github.com/uvwt/agentdock/internal/mcp/oauthclient"
)

func desktopService(t *testing.T) (*Service, *mcpclient.Manager) {
	t.Helper()
	home := t.TempDir()
	envs, err := envstore.New(home)
	if err != nil {
		t.Fatal(err)
	}
	m, err := mcpclient.NewManager(home, envs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return New(m, envs), m
}

func desktopManage(t *testing.T, s *Service, r ManageRequest) Result {
	t.Helper()
	result, err := s.Manage(context.Background(), r)
	if err != nil {
		t.Fatalf("%s: %v", r.Action, err)
	}
	return result
}

func desktopError(t *testing.T, s *Service, r ManageRequest, code string) {
	t.Helper()
	_, err := s.Manage(context.Background(), r)
	var typed *ToolError
	if !errors.As(err, &typed) || typed.Code != code {
		t.Fatalf("%s error = %#v, want %s", r.Action, err, code)
	}
}

func TestDesktopRegistryActionsUseAtomicRevisionsAndPreserveProtectedFields(t *testing.T) {
	s, m := desktopService(t)
	initial := desktopManage(t, s, ManageRequest{Action: "desktop_snapshot"})
	r := ManageRequest{Action: "desktop_create", Name: "local", Description: "Local", Transport: "stdio", Command: "never-run", ProtocolVersion: "2025-11-25", ExpectedRegistryRevision: initial["registry_revision"].(string)}
	created := desktopManage(t, s, r)
	server := created["server"].(mcpclient.ProtectedServer)
	if server.Enabled || server.ProtocolVersion != "2025-11-25" || created["persisted"] != true {
		t.Fatalf("created = %#v", created)
	}
	desktopError(t, s, r, "MCP_REGISTRY_CONFLICT")
	r.Action = "desktop_update"
	r.Description = "Edited"
	r.ExpectedRegistryRevision = created["registry_revision"].(string)
	r.ExpectedGeneration = "stale"
	desktopError(t, s, r, "MCP_SERVER_GENERATION_CONFLICT")
	r.ExpectedGeneration = server.Generation
	updated := desktopManage(t, s, r)
	updatedServer := updated["server"].(mcpclient.ProtectedServer)
	if updatedServer.Generation == server.Generation || updatedServer.Description != "Edited" {
		t.Fatalf("update = %#v", updated)
	}
	// The primitive itself prevents rename even when tokens are valid.
	registry, _ := m.Registry()
	cfg := registry.Servers["local"]
	cfg.Name = "renamed"
	_, err := m.Update("local", cfg, registry.Revision, updatedServer.Generation)
	var typed *mcpclient.Error
	if !errors.As(err, &typed) || typed.Code != "MCP_NAME_IMMUTABLE" {
		t.Fatalf("rename = %v", err)
	}
	enabled := true
	set := desktopManage(t, s, ManageRequest{Action: "desktop_set_enabled", Name: "local", Enabled: &enabled, ExpectedRegistryRevision: updated["registry_revision"].(string), ExpectedGeneration: updatedServer.Generation})
	setServer := set["server"].(mcpclient.ProtectedServer)
	if !setServer.Enabled || setServer.Observation.Connection != "not_connected" {
		t.Fatalf("set enabled = %#v", set)
	}
	desktopError(t, s, ManageRequest{Action: "desktop_remove", Name: "local", ExpectedRegistryRevision: set["registry_revision"].(string), ExpectedGeneration: updatedServer.Generation}, "MCP_SERVER_GENERATION_CONFLICT")
	desktopManage(t, s, ManageRequest{Action: "desktop_remove", Name: "local", ExpectedRegistryRevision: set["registry_revision"].(string), ExpectedGeneration: setServer.Generation})
	// Legacy secret-bearing config can be preserved, but never echoed.
	const canary = "B1_SECRET_CANARY_do_not_expose_123456789"
	if _, err := m.Add(mcpclient.ServerConfig{Name: "legacy", Description: "Legacy", Transport: "stdio", Command: "never-run", Args: []string{"--secret", canary}}); err != nil {
		t.Fatal(err)
	}
	before, _ := m.Registry()
	preserved := desktopManage(t, s, ManageRequest{Action: "desktop_update", Name: "legacy", Description: "Edited", Transport: "stdio", Command: "never-run", ExpectedRegistryRevision: before.Revision, ExpectedGeneration: before.Servers["legacy"].Generation})
	after, _ := m.Registry()
	if !reflect.DeepEqual(before.Servers["legacy"].Args, after.Servers["legacy"].Args) {
		t.Fatal("omitted protected args were lost")
	}
	data, _ := json.Marshal(preserved)
	if strings.Contains(string(data), canary) {
		t.Fatal("protected update exposed args")
	}
	desktopError(t, s, ManageRequest{Action: "desktop_create", Name: "bad", Description: "Bad", Transport: "stdio", Command: "never-run", Args: []string{"--secret", canary}, ExpectedRegistryRevision: after.Revision}, "MCP_CONFIG_INVALID")
}

func TestDesktopPluginInventoryAndEveryMutationFence(t *testing.T) {
	s, m := desktopService(t)
	name := "plugin.demo"
	if err := m.SetOwnedServers([]mcpclient.ServerConfig{{Name: name, Description: "Plugin", Transport: "stdio", Command: "never-run", SourceType: "plugin", PluginName: "demo", StorageKey: name, PluginRuntimeRoot: t.TempDir(), PluginDataDir: t.TempDir(), Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	snapshot := desktopManage(t, s, ManageRequest{Action: "desktop_snapshot"})
	servers := snapshot["servers"].([]mcpclient.ProtectedServer)
	if len(servers) != 1 || servers[0].SourceType != "plugin" || servers[0].PluginName != "demo" {
		t.Fatalf("inventory = %#v", snapshot)
	}
	desktopManage(t, s, ManageRequest{Action: "desktop_inspect", Name: name})
	value := "write-only"
	enabled := false
	for _, action := range []string{"desktop_update", "desktop_remove", "desktop_set_enabled", "desktop_env_snapshot", "desktop_env_set", "desktop_env_unset", "desktop_env_purge", "desktop_reconnect", "desktop_authorize", "desktop_auth_clear"} {
		desktopError(t, s, ManageRequest{Action: action, Name: name, Description: "Attempt", Transport: "stdio", Command: "never-run", Key: "KEY", Value: &value, Enabled: &enabled, ExpectedRegistryRevision: snapshot["registry_revision"].(string), ExpectedGeneration: servers[0].Generation, ExpectedEnvRevision: "unused"}, "MCP_OWNED_BY_PLUGIN")
	}
	after := desktopManage(t, s, ManageRequest{Action: "desktop_snapshot"})
	if !reflect.DeepEqual(snapshot, after) {
		t.Fatal("rejected Desktop mutations changed plugin inventory")
	}
}

func TestDesktopEnvironmentIsWriteOnlyAndErrorsAreProtected(t *testing.T) {
	s, m := desktopService(t)
	if _, err := m.Add(mcpclient.ServerConfig{Name: "local", Description: "Local", Transport: "stdio", Command: "never-run"}); err != nil {
		t.Fatal(err)
	}
	const canary = "B1_SECRET_CANARY_do_not_expose_123456789"
	value := canary
	snapshot := desktopManage(t, s, ManageRequest{Action: "desktop_env_snapshot", Name: "local"})
	set := desktopManage(t, s, ManageRequest{Action: "desktop_env_set", Name: "local", Key: "KEY", Value: &value, ExpectedEnvRevision: snapshot["env_revision"].(string)})
	desktopError(t, s, ManageRequest{Action: "desktop_env_unset", Name: "local", Key: "KEY", ExpectedEnvRevision: snapshot["env_revision"].(string)}, "MCP_ENV_CONFLICT")
	data, _ := json.Marshal(set)
	if strings.Contains(string(data), canary) {
		t.Fatal("env_set echoed value")
	}
	for _, err := range []error{errors.New(canary), &mcpclient.Error{Code: canary, Message: canary, Details: map[string]any{"reason": canary}, Cause: errors.New(canary)}} {
		protected := protectedMCPError(err)
		data, _ := json.Marshal(protected)
		if strings.Contains(string(data), canary) || errors.Unwrap(protected) != nil {
			t.Fatal("protected error leaked provider cause")
		}
	}
}

func TestDesktopSchemaIncludesActionsAndRevisions(t *testing.T) {
	schema, _ := InputSchema(ToolManage)
	props := schema["properties"].(map[string]any)
	actions := props["action"].(map[string]any)["enum"].([]string)
	for _, expected := range append([]string{"list", "inspect", "add", "authorize", "auth_clear"}, desktopActions...) {
		found := false
		for _, action := range actions {
			if action == expected {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing action %s", expected)
		}
	}
	for _, field := range []string{"expected_registry_revision", "expected_generation", "expected_env_revision", "protocol_version", "value"} {
		if props[field] == nil {
			t.Fatalf("missing field %s", field)
		}
	}
}

func TestDesktopAuthorizationCallbackSelectionIsProtected(t *testing.T) {
	s, m := desktopService(t)
	if _, err := m.Add(mcpclient.ServerConfig{Name: "remote", Description: "Remote", Transport: "streamable_http", URL: "https://example.invalid/mcp", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	for _, option := range []oauthclient.CallbackOption{{ID: "local", Label: "Local", RedirectURL: "http://127.0.0.1:12345/private-local-callback"}, {ID: "agentdock", Label: "Public", RedirectURL: "https://agentdock.example/private-public-callback"}} {
		if err := s.SetOAuthCallback(option); err != nil {
			t.Fatal(err)
		}
	}
	r, _ := m.Registry()
	status := desktopManage(t, s, ManageRequest{Action: "desktop_auth_status", Name: "remote"})
	// With multiple callback routes, Begin returns choices without discovery or
	// launching a flow. The private native/public redirect paths remain Core-only.
	selection := desktopManage(t, s, ManageRequest{Action: "desktop_authorize", Name: "remote", ExpectedRegistryRevision: r.Revision, ExpectedGeneration: r.Servers["remote"].Generation})
	for _, result := range []Result{status, selection} {
		data, _ := json.Marshal(result)
		if strings.Contains(string(data), "private-") || strings.Contains(string(data), "redirect_url") {
			t.Fatalf("callback metadata exposed private redirect: %s", data)
		}
		if result["callback_options"] == nil {
			t.Fatal("callback capabilities missing")
		}
	}
	desktopError(t, s, ManageRequest{Action: "desktop_authorize", Name: "remote", ExpectedRegistryRevision: "stale", ExpectedGeneration: r.Servers["remote"].Generation}, "MCP_REGISTRY_CONFLICT")
}
