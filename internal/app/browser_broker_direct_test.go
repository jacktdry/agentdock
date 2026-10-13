package app

import (
	"context"
	"testing"

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
		{"action": "direct_evaluate", "lease_id": "lease", "function": "() => 1"},
		{"action": "direct_snapshot", "lease_id": "lease"},
		{"action": "direct_release", "lease_id": "lease"},
	} {
		if err := validator.Validate(args); err != nil {
			t.Fatalf("valid direct schema input %#v: %v", args, err)
		}
	}
	for _, args := range []map[string]any{
		{"action": "direct_acquire", "workspace_root": "/private/other"},
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
