package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	"github.com/uvwt/agentdock/internal/observability"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

func TestBrowserBrokerDirectStrictActionSchema(t *testing.T) {
	schema, ok := toolbrowser.InputSchema(toolbrowser.ToolBroker)
	if !ok {
		t.Fatal("broker schema missing")
	}
	validator, err := compileBuiltInInputValidator(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]any{
		{"action": "status"},
		{"action": "cleanup_acp_session", "owner_acp_session_id": "owned-session"},
		{"action": "direct_acquire"},
		{"action": "direct_acquire", "url": "about:blank"},
		{"action": "direct_acquire", "workspace_root": "/configured/company"},
		{"action": "connect_external", "workspace_root": "/configured/company"},
		{"action": "disconnect_external"},
		{"action": "direct_navigate", "lease_id": "lease", "url": "https://example.com"},
		{"action": "direct_screenshot", "lease_id": "lease"},
		{"action": "direct_evaluate", "lease_id": "lease", "function": "() => 1"},
		{"action": "direct_snapshot", "lease_id": "lease"},
		{"action": "direct_release", "lease_id": "lease"},
	} {
		if err := validator.Validate(args); err != nil {
			t.Fatalf("valid direct schema input %#v: %v", args, err)
		}
	}
	for _, args := range []map[string]any{
		{"action": "connect_external", "url": "https://external.invalid"},
		{"action": "direct_navigate", "lease_id": "lease"},
		{"action": "direct_screenshot", "lease_id": "lease", "function": "() => 1"},
		{"action": "direct_acquire", "owner_acp_session_id": "forged"},
		{"action": "direct_acquire", "lease_id": "another"},
		{"action": "direct_evaluate", "lease_id": "lease"},
		{"action": "direct_snapshot", "lease_id": "lease", "url": "https://external.invalid"},
		{"action": "direct_release"},
		{"action": "direct_release", "lease_id": "lease", "pageId": 7},
		{"action": "status", "function": "() => 1"},
		{"action": "unrecognized"},
	} {
		if err := validator.Validate(args); err == nil {
			t.Fatalf("invalid direct schema input accepted: %#v", args)
		}
	}
}

func TestBrowserBrokerDirectPermissionFactsAreBoundAndConservative(t *testing.T) {
	r := newRuntimeValidationTestRuntime(t)
	schema, _ := toolbrowser.InputSchema(toolbrowser.ToolBroker)
	v, err := compileBuiltInInputValidator(schema)
	if err != nil {
		t.Fatal(err)
	}
	r.toolValidators[toolbrowser.ToolBroker] = v
	principal := requestmeta.NewStableAuthPrincipal("oauth_grant", "direct-test", "grant")
	ctx := requestmeta.WithAuthPrincipal(context.Background(), principal)

	eval := r.runtimePermissionFacts(ctx, toolbrowser.ToolBroker, "direct_evaluate", map[string]any{"function": "() => 1"}, observability.SourceMCP)
	if !eval.OpaqueProviderExecution || eval.EffectsKnown || eval.ReadOnly || !eval.Network || !eval.OneShotEligible || eval.Binding.Principal.ID != principal.ID || !eval.Binding.Principal.Authenticated {
		t.Fatalf("JS evaluation not correctly classified for M8: %+v", eval)
	}
	snapshot := r.runtimePermissionFacts(ctx, toolbrowser.ToolBroker, "direct_snapshot", map[string]any{}, observability.SourceMCP)
	if !snapshot.EffectsKnown || !snapshot.ReadOnly || snapshot.Network || snapshot.Other || snapshot.Binding.Principal.ID != principal.ID {
		t.Fatalf("snapshot must be read-only but principal-bound: %+v", snapshot)
	}
	acquire := r.runtimePermissionFacts(ctx, toolbrowser.ToolBroker, "direct_acquire", map[string]any{"url": "https://example.com"}, observability.SourceMCP)
	if !acquire.Management || !acquire.Network || acquire.ReadOnly {
		t.Fatalf("browser acquire not privileged/network-scoped: %+v", acquire)
	}
	release := r.runtimePermissionFacts(ctx, toolbrowser.ToolBroker, "direct_release", map[string]any{"lease_id": "lease"}, observability.SourceMCP)
	if !release.Management || release.ReadOnly {
		t.Fatalf("browser release classified incorrectly: %+v", release)
	}
}

func TestBrowserBrokerWorkspaceRequiresExactConfiguredCompanyRoot(t *testing.T) {
	defaultRoot, _ := browserpolicy.CanonicalWorkspaceRoot(t.TempDir())
	company, _ := browserpolicy.CanonicalWorkspaceRoot(t.TempDir())
	r := &Runtime{cfg: config.Config{AgentDockDefaultDir: defaultRoot, BrowserWorkspacePolicies: []browserpolicy.WorkspaceRootPolicy{{Root: company,
		Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal}}}}}
	for _, selected := range []string{"", defaultRoot, company} {
		root, err := r.browserBrokerWorkspace(map[string]any{"workspace_root": selected})
		if err != nil || (root != defaultRoot && root != company) {
			t.Fatal("configured root unavailable", selected, err)
		}
	}
	for _, selected := range []string{t.TempDir(), filepath.Dir(company), company + "/child", company + "/.", "relative"} {
		if _, err := r.browserBrokerWorkspace(map[string]any{"workspace_root": selected}); err == nil {
			t.Fatalf("arbitrary root accepted: %s", selected)
		}
	}
	r.cfg.AgentDockDefaultDir = filepath.Join(defaultRoot, "missing")
	if root, err := r.browserBrokerWorkspace(map[string]any{"workspace_root": company}); err != nil || root != company {
		t.Fatal("unrelated default workspace prevented selected company operation", err)
	}
}
