package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/envstore"
	"github.com/uvwt/agentdock/internal/mcp/oauthclient"
)

const desktopCanary = "B1_SECRET_CANARY_do_not_expose_123456789"

func desktopFixture(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func assertProtected(t *testing.T, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), desktopCanary) {
		t.Fatalf("protected projection exposed canary: %s", data)
	}
}

// Compare actual durable payloads, including OAuth envelopes; lock files are
// synchronization metadata, not registry/environment/authorization mutations.
func desktopFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.HasSuffix(path, ".lock") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err == nil {
			files[path] = string(data)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestDesktopReadsArePassiveProtectedAndGenerationAware(t *testing.T) {
	m := desktopFixture(t)
	for _, cfg := range []ServerConfig{
		{Name: "local", Description: "Local", Transport: TransportStdio, Command: "never-run", Args: []string{"--token", desktopCanary}, Enabled: true},
		{Name: "remote", Description: "Remote", Transport: TransportStreamableHTTP, URL: "https://example.invalid/mcp?token=" + desktopCanary, Enabled: true},
	} {
		if _, err := m.Add(cfg); err != nil {
			t.Fatal(err)
		}
	}
	plugin := ServerConfig{Name: "plugin.demo", Description: "Plugin", Transport: TransportStdio, Command: "never-run", SourceType: "plugin", PluginName: "demo", StorageKey: "plugin.demo", PluginRuntimeRoot: t.TempDir(), PluginDataDir: t.TempDir(), StaticEnv: map[string]string{"KEY": desktopCanary}, Enabled: true}
	if err := m.SetOwnedServers([]ServerConfig{plugin}); err != nil {
		t.Fatal(err)
	}
	// A cached client must stay alive even when persisted configuration changes.
	failing := &failingCloseProtocolClient{}
	m.states["local"].client = failing
	m.states["local"].lastError = desktopCanary
	m.states["local"].lastErrorCode = desktopCanary
	m.states["local"].tools = map[string]Tool{"old": {Name: "old"}}
	m.states["local"].refreshedAt = time.Now()
	var creates atomic.Int32
	m.protocolClientHook = func(ServerConfig) (protocolClient, error) {
		creates.Add(1)
		t.Fatal("passive read created a protocol client")
		return nil, nil
	}
	if err := m.envs.Set(envstore.Scope{Kind: envstore.ScopeMCP, Name: "local"}, "KEY", desktopCanary); err != nil {
		t.Fatal(err)
	}
	if err := m.SetOAuthCallback(oauthclient.CallbackOption{ID: "local", Label: "Local", RedirectURL: "http://127.0.0.1:12345/" + desktopCanary}); err != nil {
		t.Fatal(err)
	}
	// Local OAuth grant fixture includes an expired token and a refresh token.
	home := filepath.Dir(filepath.Dir(m.store.path))
	grantRoot := filepath.Join(home, "data", "mcp")
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer provider.Close()
	if err := os.MkdirAll(grantRoot, 0700); err != nil {
		t.Fatal(err)
	}
	grantData, _ := json.Marshal(oauthclient.Grant{SchemaVersion: 1, Endpoint: "https://example.invalid/mcp?token=" + desktopCanary, AccessToken: desktopCanary, RefreshToken: desktopCanary, TokenURL: provider.URL, Expiry: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err := os.WriteFile(filepath.Join(grantRoot, "grant-remote.json"), grantData, 0600); err != nil {
		t.Fatal(err)
	}
	before := desktopFiles(t, home)
	snapshot, err := m.DesktopSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Servers) != 3 || !snapshot.Authoritative {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	assertProtected(t, snapshot)
	for _, name := range []string{"local", "remote", "plugin.demo"} {
		_, server, err := m.DesktopInspect(name)
		if err != nil {
			t.Fatal(err)
		}
		assertProtected(t, server)
		if name == "local" && (!server.ArgsProtected || len(server.Args) != 0 || server.Observation.LastErrorCode != "MCP_ERROR") {
			t.Fatalf("local = %#v", server)
		}
		if name == "remote" && (strings.Contains(server.URL, "?") || !server.URLProtected) {
			t.Fatalf("remote = %#v", server)
		}
		status, err := m.DesktopAuthorizationStatus(name)
		if err != nil {
			t.Fatal(err)
		}
		assertProtected(t, status)
		if name == "remote" && status.Status != "authorized" {
			t.Fatalf("expired local grant status = %#v", status)
		}
	}
	env, err := m.DesktopEnvironment("local", "snapshot", "", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertProtected(t, env)
	if !reflect.DeepEqual(before, desktopFiles(t, home)) {
		t.Fatal("passive read changed durable payloads")
	}
	if creates.Load() != 0 || failing.closeCalls.Load() != 0 || requests.Load() != 0 {
		t.Fatal("passive read affected runtime")
	}
	// Independent writer changes persisted generation without updating this cache.
	r, _ := m.Registry()
	other, err := NewManager(filepath.Dir(filepath.Dir(m.store.path)))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	r2, _ := other.Registry()
	cfg := r2.Servers["local"]
	cfg.Description = "Changed"
	if _, err := other.Update("local", cfg, r2.Revision, cfg.Generation); err != nil {
		t.Fatal(err)
	}
	_, stale, err := m.DesktopInspect("local")
	if err != nil {
		t.Fatal(err)
	}
	if stale.Generation == r.Servers["local"].Generation || !stale.Observation.Stale || stale.Observation.Status != "unknown" || stale.Observation.ToolCount != nil {
		t.Fatalf("stale = %#v", stale)
	}
	if failing.closeCalls.Load() != 0 {
		t.Fatal("passive inspect closed stale client")
	}
}

func TestDesktopEnvironmentCheckedAndPluginFence(t *testing.T) {
	m := desktopFixture(t)
	if _, err := m.Add(ServerConfig{Name: "local", Description: "Local", Transport: TransportStdio, Command: "never-run"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := m.DesktopEnvironment("local", "snapshot", "", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	value := desktopCanary
	updated, err := m.DesktopEnvironment("local", "set", "Z", &value, snapshot.Revision, "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.DesktopEnvironment("local", "set", "A", &value, snapshot.Revision, "", "")
	requireMCPClientErrorCode(t, err, "MCP_ENV_CONFLICT")
	_, err = m.DesktopEnvironment("local", "purge", "", nil, "", "", "")
	requireMCPClientErrorCode(t, err, "MCP_ENV_CONFLICT")
	updated, err = m.DesktopEnvironment("local", "set", "A", &value, updated.Revision, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertProtected(t, updated)
	if len(updated.Entries) != 2 || updated.Entries[0].Key != "A" || !updated.Entries[0].Configured {
		t.Fatalf("env = %#v", updated)
	}
	updated, err = m.DesktopEnvironment("local", "unset", "A", nil, updated.Revision, "", "")
	if err != nil || len(updated.Entries) != 1 {
		t.Fatalf("unset = %#v, %v", updated, err)
	}
	updated, err = m.DesktopEnvironment("local", "purge", "", nil, updated.Revision, "", "")
	if err != nil || len(updated.Entries) != 0 {
		t.Fatalf("purge = %#v, %v", updated, err)
	}
	plugin := ServerConfig{Name: "plugin.demo", Description: "Plugin", Transport: TransportStreamableHTTP, URL: "https://example.invalid/mcp", SourceType: "plugin", PluginName: "demo", StorageKey: "plugin.demo", PluginRuntimeRoot: t.TempDir(), PluginDataDir: t.TempDir(), StaticHeaders: map[string]string{"Authorization": desktopCanary}, Enabled: true}
	if err := m.SetOwnedServers([]ServerConfig{plugin}); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"snapshot", "set", "unset", "purge"} {
		_, err := m.DesktopEnvironment(plugin.Name, action, "KEY", &value, updated.Revision, "", "")
		requireMCPClientErrorCode(t, err, "MCP_OWNED_BY_PLUGIN")
	}
	_, _, err = m.DesktopReconnect(context.Background(), plugin.Name, "", "")
	requireMCPClientErrorCode(t, err, "MCP_OWNED_BY_PLUGIN")
	_, err = m.DesktopAuthorize(context.Background(), plugin.Name, "", "", "")
	requireMCPClientErrorCode(t, err, "MCP_OWNED_BY_PLUGIN")
	err = m.DesktopClearAuthorization(plugin.Name, "", "")
	requireMCPClientErrorCode(t, err, "MCP_OWNED_BY_PLUGIN")
	_, safe, err := m.DesktopInspect(plugin.Name)
	if err != nil {
		t.Fatal(err)
	}
	assertProtected(t, safe)
}

func TestDesktopEnableRequiresExplicitConfiguredEnvironmentReuse(t *testing.T) {
	m := desktopFixture(t)
	if _, err := m.Add(ServerConfig{Name: "local", Description: "Local", Transport: TransportStdio, Command: "never-run", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	value := desktopCanary
	env, err := m.DesktopEnvironment("local", "snapshot", "", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.DesktopEnvironment("local", "set", "TOKEN", &value, env.Revision, "", ""); err != nil {
		t.Fatal(err)
	}
	snapshot, err := m.DesktopSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	server := snapshot.Servers[0]
	if !server.EnvironmentConfigured || !server.EnableRequiresEnvironmentConfirmation ||
		!slices.Contains(server.BlockedReasons, "configured_environment_reuse_confirmation_required") {
		t.Fatalf("configured environment state = %#v", server)
	}
	_, err = m.DesktopSetEnabledChecked("local", true, false, snapshot.RegistryRevision, server.Generation)
	requireMCPClientErrorCode(t, err, "MCP_RETAINED_ENV_CONFIRMATION_REQUIRED")
	afterRejected, _ := m.Registry()
	if afterRejected.Servers["local"].Enabled {
		t.Fatal("rejected enable changed persisted state")
	}
	result, err := m.DesktopSetEnabledChecked("local", true, true, snapshot.RegistryRevision, server.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Server.Enabled || !result.Persisted || !result.RuntimeApplied {
		t.Fatalf("explicit reuse enable = %#v", result)
	}
}

type desktopProtocol struct{ starts, closes atomic.Int32 }

func (p *desktopProtocol) initialize(context.Context) error { p.starts.Add(1); return nil }
func (p *desktopProtocol) listTools(context.Context) ([]Tool, error) {
	return []Tool{{Name: "echo", Description: desktopCanary, InputSchema: map[string]any{"type": "object"}}}, nil
}
func (p *desktopProtocol) callTool(context.Context, string, map[string]any) (map[string]any, error) {
	return nil, nil
}
func (p *desktopProtocol) close() error { p.closes.Add(1); return nil }

func TestDesktopReconnectIsExplicitAndChecked(t *testing.T) {
	m := desktopFixture(t)
	if _, err := m.Add(ServerConfig{Name: "local", Description: "Local", Transport: TransportStdio, Command: "never-run", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	p := &desktopProtocol{}
	var created atomic.Int32
	m.protocolClientHook = func(ServerConfig) (protocolClient, error) { created.Add(1); return p, nil }
	snapshot, _ := m.DesktopSnapshot()
	if created.Load() != 0 {
		t.Fatal("snapshot connected")
	}
	_, _, err := m.DesktopReconnect(context.Background(), "local", "stale", snapshot.Servers[0].Generation)
	requireMCPClientErrorCode(t, err, "MCP_REGISTRY_CONFLICT")
	if created.Load() != 0 {
		t.Fatal("stale reconnect dispatched")
	}
	server, tools, err := m.DesktopReconnect(context.Background(), "local", snapshot.RegistryRevision, snapshot.Servers[0].Generation)
	if err != nil {
		t.Fatal(err)
	}
	if created.Load() != 1 || p.starts.Load() != 1 || server.Observation.ToolCount == nil || *server.Observation.ToolCount != 1 {
		t.Fatalf("reconnect = %#v", server)
	}
	assertProtected(t, tools)
	if _, err := m.DesktopSnapshot(); err != nil {
		t.Fatal(err)
	}
	if p.closes.Load() != 0 || created.Load() != 1 {
		t.Fatal("snapshot reconnected existing client")
	}
	if _, _, err := m.DesktopReconnect(context.Background(), "local", snapshot.RegistryRevision, snapshot.Servers[0].Generation); err != nil {
		t.Fatal(err)
	}
	if p.closes.Load() != 1 || created.Load() != 2 {
		t.Fatal("explicit reconnect did not replace old client")
	}
}

func TestDesktopCredentialClassifier(t *testing.T) {
	for _, args := range [][]string{{"--api-key", "abc"}, {"--key", "abc"}, {"--password=abc"}, {"-H", "Authorization: abc"}, {"https://example.invalid/?x=abc"}, {"AbCDef012345678901234567890"}} {
		if !protectedArgs(args) {
			t.Fatalf("unsafe args accepted: %#v", args)
		}
	}
	if protectedArgs([]string{"--port", "3000", "./server.js"}) {
		t.Fatal("safe args hidden")
	}
	for _, endpoint := range []string{"http://remote.invalid/mcp", "https://example.invalid/mcp?code=abc", "https://user:abc@example.invalid/mcp", "https://example.invalid/mcp#abc"} {
		if err := ValidateDesktopConfig(ServerConfig{Transport: " STREAMABLE_HTTP ", URL: endpoint}); err == nil {
			t.Fatalf("Desktop accepted endpoint %q", endpoint)
		}
	}
	for _, endpoint := range []string{"http://127.0.0.1/mcp", "http://[::1]/mcp", "https://example.invalid/mcp"} {
		if err := ValidateDesktopConfig(ServerConfig{Transport: "streamable_http", URL: endpoint}); err != nil {
			t.Fatalf("safe endpoint %q: %v", endpoint, err)
		}
	}
}

func TestDesktopFailedReconnectKeepsOldObservationStale(t *testing.T) {
	m := desktopFixture(t)
	if _, err := m.Add(ServerConfig{Name: "local", Description: "Local", Transport: TransportStdio, Command: "never-run", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	failing := &failingCloseProtocolClient{}
	m.states["local"].client = failing
	r, _ := m.Registry()
	// Simulate an independent persisted replace while the old runtime stays live.
	_, err := m.store.update(func(snapshot RegistrySnapshot) error {
		cfg := snapshot.Servers["local"]
		cfg.Description = "Changed"
		snapshot.Servers["local"] = cfg
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m.protocolClientHook = func(ServerConfig) (protocolClient, error) {
		t.Fatal("replacement created after close failure")
		return nil, nil
	}
	current, _ := m.Registry()
	server, _, err := m.DesktopReconnect(context.Background(), "local", current.Revision, current.Servers["local"].Generation)
	requireMCPClientErrorCode(t, err, "MCP_CLIENT_CLOSE_FAILED")
	if server.Generation == r.Servers["local"].Generation || !server.Observation.Stale || server.Observation.ToolCount != nil {
		t.Fatalf("failed reconnect observation = %#v", server)
	}
}

func TestDesktopAuthorizationStatusAndClearStayLocal(t *testing.T) {
	m := desktopFixture(t)
	if _, err := m.Add(ServerConfig{Name: "remote", Description: "Remote", Transport: TransportStreamableHTTP, URL: "https://example.invalid/mcp", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	grantRoot := filepath.Join(filepath.Dir(filepath.Dir(m.store.path)), "data", "mcp")
	grantData, _ := json.Marshal(oauthclient.Grant{SchemaVersion: 1, Endpoint: "https://example.invalid/mcp", AccessToken: desktopCanary, RefreshToken: desktopCanary})
	if err := os.WriteFile(filepath.Join(grantRoot, "grant-remote.json"), grantData, 0600); err != nil {
		t.Fatal(err)
	}
	p := &desktopProtocol{}
	m.states["remote"].client = p
	status, err := m.DesktopAuthorizationStatus("remote")
	if err != nil || status.Status != "authorized" || p.closes.Load() != 0 {
		t.Fatalf("auth status = %#v, %v", status, err)
	}
	r, _ := m.Registry()
	err = m.DesktopClearAuthorization("remote", r.Revision, "stale")
	requireMCPClientErrorCode(t, err, "MCP_SERVER_GENERATION_CONFLICT")
	if p.closes.Load() != 0 {
		t.Fatal("stale auth clear closed client")
	}
	if err := m.DesktopClearAuthorization("remote", r.Revision, r.Servers["remote"].Generation); err != nil {
		t.Fatal(err)
	}
	status, err = m.DesktopAuthorizationStatus("remote")
	if err != nil || status.Status != "unauthorized" || p.closes.Load() != 1 {
		t.Fatalf("clear = %#v, %v", status, err)
	}
	assertProtected(t, status)
}
