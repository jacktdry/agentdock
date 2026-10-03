package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

func TestBrowserWorkspacePoliciesConfig(t *testing.T) {
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
