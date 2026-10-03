package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

func TestBrowserWorkspacePoliciesConfig(t *testing.T) {
	t.Setenv(browserCatalogEnv, validBrowserCatalogJSON)
	root := t.TempDir()
	mappings := []browserpolicy.WorkspaceRootPolicy{{Root: root, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "registered-connector", RequiredProfileID: "authenticated-profile"}}}
	raw, err := json.Marshal(mappings)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(browserWorkspacePoliciesEnv, string(raw))
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.BrowserWorkspacePolicies) != 1 || cfg.BrowserWorkspacePolicies[0].Policy != mappings[0].Policy {
		t.Fatalf("%+v", cfg.BrowserWorkspacePolicies)
	}
	setTestUserHome(t, t.TempDir())
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	canonical, err := browserpolicy.CanonicalWorkspaceRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BrowserWorkspacePolicies[0].Root != canonical {
		t.Fatalf("root=%q", cfg.BrowserWorkspacePolicies[0].Root)
	}
	for _, raw := range []string{`{}`, `[{"root":"relative","policy":{"class":"company","route":"managed-isolated"}}]`, `[{"unknown":true}]`, string(raw) + ` []`} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv(browserWorkspacePoliciesEnv, raw)
			if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), browserWorkspacePoliciesEnv) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	cfg.BrowserWorkspacePolicies[0].Root = filepath.Join(root, "missing")
	if err := cfg.Normalize(); err == nil {
		t.Fatal("unresolvable configured root accepted")
	}
}

const validBrowserCatalogJSON = `{"profiles":[{"id":"authenticated-profile","browser":"edge","class":"authenticated-external"}],"connectors":[{"id":"registered-connector","profile_id":"authenticated-profile","driver":"chrome-devtools-mcp-ws","endpoint":"ws://127.0.0.1:9222/devtools/browser"}]}`

func TestBrowserCatalogConfig(t *testing.T) {
	root := t.TempDir()
	policies, err := json.Marshal([]browserpolicy.WorkspaceRootPolicy{{Root: root, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "registered-connector", RequiredProfileID: "authenticated-profile"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(browserWorkspacePoliciesEnv, string(policies))
	for _, raw := range []string{"", `null`, `[]`, `{`, `{"healthy":true}`, validBrowserCatalogJSON + ` {}`, strings.Replace(validBrowserCatalogJSON, `"browser":"edge"`, `"browser":"chrome"`, 1), strings.Replace(validBrowserCatalogJSON, `"class":"authenticated-external"`, `"class":"external-persistent"`, 1), strings.Replace(validBrowserCatalogJSON, `"profile_id":"authenticated-profile"`, `"profile_id":"missing"`, 1), strings.Replace(validBrowserCatalogJSON, `127.0.0.1`, `example.com`, 1)} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv(browserCatalogEnv, raw)
			if _, err := FromEnv(); err == nil {
				t.Fatal("invalid company catalog accepted")
			}
		})
	}
	for _, field := range []string{`"healthy":true`, `"verified":true`, `"authenticated":true`, `"capabilities":[]`, `"ownership":{}`, `"command":"node"`, `"args":[]`, `"env":{}`, `"executable":"node"`, `"engine_version":"latest"`, `"cleanup_paths":[]`} {
		for _, needle := range []string{`"profiles":[{`, `"connectors":[{`} {
			t.Run(field+needle, func(t *testing.T) {
				t.Setenv(browserCatalogEnv, strings.Replace(validBrowserCatalogJSON, needle, needle+field+`,`, 1))
				if _, err := FromEnv(); err == nil {
					t.Fatal("runtime/command field accepted")
				}
			})
		}
	}
	t.Setenv(browserCatalogEnv, validBrowserCatalogJSON)
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.BrowserCatalog.Profiles) != 1 || len(cfg.BrowserCatalog.Connectors) != 1 {
		t.Fatalf("catalog=%+v", cfg.BrowserCatalog)
	}
	setTestUserHome(t, t.TempDir())
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	cfg.BrowserCatalog.Profiles[0].Browser = browserpolicy.BrowserChrome
	if err := cfg.Normalize(); err == nil {
		t.Fatal("Normalize accepted wrong company browser")
	}
	cfg.BrowserCatalog.Profiles[0].Browser = browserpolicy.BrowserEdge
	cfg.BrowserWorkspacePolicies[0].Policy.RequiredConnectorID = "missing"
	if err := cfg.Normalize(); err == nil {
		t.Fatal("Normalize accepted dangling company route")
	}
}
