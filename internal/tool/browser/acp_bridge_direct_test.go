package browser

import (
	"context"
	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	"testing"
	"time"
)

func directContext(id string) context.Context {
	return requestmeta.WithAuthPrincipal(context.Background(), requestmeta.NewStableAuthPrincipal("mcp", id))
}

func TestDirectBrokerSharesACPWorkerAndFencesPrincipal(t *testing.T) {
	b, s, starts, root := sharedBridgeFixture(t, true)
	ctx := directContext("chatgpt-a")
	for _, invalid := range []context.Context{context.Background(), requestmeta.WithAuthPrincipal(context.Background(), requestmeta.AuthPrincipal{Kind: "mcp", ID: "forged", Authenticated: true})} {
		_, err := b.DirectAcquire(invalid, root, "")
		assertBrowserCode(t, err, ErrScopeRequired)
	}
	if *starts != 0 {
		t.Fatal("unauthenticated request started worker")
	}
	if _, err := b.DirectAcquire(ctx, "relative", ""); err == nil {
		t.Fatal("invalid root accepted")
	}
	token, acp := sharedBridgeAcquire(t, b, "codex", root)
	direct, err := b.DirectAcquire(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if *starts != 1 || direct.Scope.Provenance != ScopeUpstream || direct.Scope.OwnerACPSessionID != "" || len(b.bySession) != 1 {
		t.Fatal("direct invented ACP session or separate worker")
	}
	for _, tool := range []string{"take_snapshot", "evaluate_script"} {
		args := map[string]any{}
		if tool == "evaluate_script" {
			args["function"] = "() => 1"
		}
		if _, err := b.DirectCall(directContext("chatgpt-b"), root, direct.BrowserLeaseID, tool, args); err == nil {
			t.Fatal("cross principal call accepted")
		}
		if _, err := b.DirectCall(ctx, root, direct.BrowserLeaseID, tool, args); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.DirectRelease(directContext("chatgpt-b"), root, direct.BrowserLeaseID); err == nil {
		t.Fatal("cross principal release accepted")
	}
	if _, err := b.DirectRelease(ctx, root, acp.BrowserLeaseID); err == nil {
		t.Fatal("direct released ACP lease")
	}
	if _, err := b.DirectCall(ctx, root, direct.BrowserLeaseID, "take_snapshot", map[string]any{"pageId": float64(999)}); err == nil {
		t.Fatal("caller pageId accepted")
	}
	meta, err := b.DirectRelease(ctx, root, direct.BrowserLeaseID)
	if err != nil || meta.CleanupState != CleanupComplete {
		t.Fatalf("cleanup: %+v %v", meta, err)
	}
	if len(s.pages) != 2 {
		t.Fatal("cleanup touched sibling/keeper")
	}
	if _, err := b.Call(ctx, token, acp.BrowserLeaseID, "take_snapshot", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.DirectAcquire(ctx, root, ""); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if len(b.managed.active) != 0 {
		t.Fatal("close retained direct capacity")
	}
	if _, err := b.DirectAcquire(ctx, root, ""); err == nil {
		t.Fatal("closed bridge acquired")
	}
	if _, err := b.DirectCall(ctx, root, direct.BrowserLeaseID, "take_snapshot", nil); err == nil {
		t.Fatal("closed bridge called")
	}
	if _, err := b.DirectRelease(ctx, root, direct.BrowserLeaseID); err == nil {
		t.Fatal("closed bridge released")
	}
}

func TestDirectBrokerCancellationAndLifetime(t *testing.T) {
	b, _, starts, root := sharedBridgeFixture(t, true)
	ctx, cancel := context.WithCancel(directContext("a"))
	cancel()
	if _, err := b.DirectAcquire(ctx, root, ""); err == nil {
		t.Fatal("canceled acquire accepted")
	}
	if *starts != 0 {
		t.Fatal("canceled request started worker")
	}
	meta, err := b.DirectAcquire(directContext("a"), root, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.CleanupStale(time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	released, err := b.DirectRelease(directContext("a"), root, meta.BrowserLeaseID)
	if err != nil || released.CleanupState != CleanupComplete {
		t.Fatalf("TTL cleanup: %+v %v", released, err)
	}
}

func TestDirectBrokerRejectsCompanyBeforeManagedStart(t *testing.T) {
	b, _, starts, root := sharedBridgeFixture(t, true)
	// Policy selection is tested without catalog qualification or connector access.
	b.planner.policies = []browserpolicy.WorkspaceRootPolicy{{Root: root, Policy: browserpolicy.BrowserRoutePolicy{Class: browserpolicy.WorkspaceCompany, Route: browserpolicy.RouteRequiredExternal, RequiredConnectorID: "edge", RequiredProfileID: "company", AllowedOrigins: []string{"https://company.example.com"}}}}
	if _, err := b.DirectAcquire(directContext("a"), root, ""); err == nil {
		t.Fatal("company fell back to managed")
	}
	if *starts != 0 {
		t.Fatal("company request started managed worker")
	}
}
