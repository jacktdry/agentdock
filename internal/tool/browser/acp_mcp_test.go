package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/permission"
)

func newTestACPBridge(t *testing.T) *ACPBridge {
	t.Helper()
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := NewRoutePlanner(nil, catalog, nil)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := NewACPBridge(planner, NewWorkerRegistry(WorkerDependencies{}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bridge.Close() })
	return bridge
}

func acpMCPRequest(t *testing.T, handler http.Handler, token string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/acp-browser/mcp", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:55000"
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestACPBrowserMCPRequiresCapabilityAndOnlyExposesBrokerTools(t *testing.T) {
	bridge := newTestACPBridge(t)
	token, err := bridge.RegisterSession("acps-one", "codex", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := bridge.MCPHTTPHandler()
	unauthorized := acpMCPRequest(t, handler, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{}})
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	initialize := acpMCPRequest(t, handler, token, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}},
	})
	if initialize.Code != http.StatusOK {
		t.Fatalf("initialize status=%d body=%s", initialize.Code, initialize.Body.String())
	}
	listed := acpMCPRequest(t, handler, token, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}})
	if listed.Code != http.StatusOK {
		t.Fatalf("tools/list status=%d body=%s", listed.Code, listed.Body.String())
	}
	var response struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(response.Result.Tools))
	for _, tool := range response.Result.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	want := []string{"browser_acquire", "browser_click", "browser_evaluate", "browser_fill", "browser_navigate", "browser_press_key", "browser_release", "browser_screenshot", "browser_snapshot"}
	sort.Strings(want)
	if len(names) != len(want) {
		t.Fatalf("tool names=%v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("tool names=%v want=%v", names, want)
		}
	}
}

func TestACPBrowserCapabilityCannotUseAnotherSessionsLease(t *testing.T) {
	bridge := newTestACPBridge(t)
	tokenA, err := bridge.RegisterSession("acps-a", "codex", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tokenB, err := bridge.RegisterSession("acps-b", "codex", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ownerA, err := bridge.owner(tokenA)
	if err != nil {
		t.Fatal(err)
	}
	bridge.mu.Lock()
	ownerA.leases["lease-a"] = acpBrowserLease{route: browserpolicy.RouteManaged}
	bridge.mu.Unlock()
	if _, err := bridge.Call(context.Background(), tokenB, "lease-a", "take_snapshot", nil); err == nil {
		t.Fatal("cross-session lease call succeeded")
	} else {
		assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
	}
}

func TestACPBrowserHTTPRejectsMalformedAndRevokedCapabilities(t *testing.T) {
	b := newTestACPBridge(t)
	root := t.TempDir()
	token, err := b.RegisterSession("owner", "codex", root)
	if err != nil {
		t.Fatal(err)
	}
	h := b.MCPHTTPHandler()
	for _, header := range []string{"", "Basic " + token, token, "bearer " + token, "Bearer", "Bearer invalid", "Bearer " + token + " extra"} {
		req := httptest.NewRequest("POST", "/internal/acp-browser/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Authorization", header)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("malformed bearer authorized: status=%d", rec.Code)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := b.RegisterSession("owner", "codex", root)
			if err != nil || got != token {
				t.Error("same session capability changed", err)
			}
		}()
	}
	wg.Wait()
	for i := 0; i < 2; i++ {
		if err := b.ReleaseSession(context.Background(), "owner"); err != nil {
			t.Fatal(err)
		}
	}
	rec := acpMCPRequest(t, h, token, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatal("revoked token authorized")
	}
	renewed, err := b.RegisterSession("owner", "codex", root)
	if err != nil || renewed == token {
		t.Fatal("revoked token reused", err)
	}
}

func TestACPBrowserHTTPLeaseScopeAndManagedRoute(t *testing.T) {
	b := newTestACPBridge(t)
	backend := &fakeLeaseBackend{}
	b.managed.manager = NewManagedLeaseManager(backend)
	a, err := b.RegisterSession("a", "codex", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other, err := b.RegisterSession("b", "codex", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := b.Acquire(context.Background(), a, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if meta.ProfileClass != ProfileIsolated || meta.OwnerProfileID != "codex" || backend.starts != 1 {
		t.Fatalf("managed route=%+v starts=%d", meta, backend.starts)
	}
	h := b.MCPHTTPHandler()
	for _, name := range []string{"browser_snapshot", "browser_release"} {
		rec := acpMCPRequest(t, h, other, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": map[string]any{"lease_id": meta.BrowserLeaseID}}})
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), ErrLeaseOwnerMismatch) || !strings.Contains(rec.Body.String(), `"isError":true`) {
			t.Fatalf("cross-session %s: %d %s", name, rec.Code, rec.Body)
		}
	}
	if len(backend.calls) != 1 || len(backend.stopped) != 0 {
		t.Fatal("cross-session request reached backend")
	}
	if err := b.ReleaseSession(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if len(backend.stopped) != 1 {
		t.Fatal("session release leaked worker")
	}
}

func TestACPBridgeCompanyWithoutVerifiedStatusNeverStartsManaged(t *testing.T) {
	planner, scope := plannerFixture(t, nil)
	b := newTestACPBridge(t)
	b.planner = planner
	backend := &fakeLeaseBackend{}
	b.managed.manager = NewManagedLeaseManager(backend)
	token, err := b.RegisterSession("company", "codex", scope.CanonicalWorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := b.Acquire(context.Background(), token, "")
	assertBrowserCode(t, err, ErrRequiredRouteUnavailable)
	if meta.BrowserLeaseID != "" || backend.starts != 0 {
		t.Fatal("required external silently fell back managed")
	}
}

func TestACPBrowserRegisterSessionWithTokenRequiresStableCapability(t *testing.T) {
	bridge := newTestACPBridge(t)
	root := t.TempDir()
	got, err := bridge.RegisterSessionWithToken("owner", "codex", root, "shared-capability-token")
	if err != nil {
		t.Fatal(err)
	}
	if got != "shared-capability-token" {
		t.Fatalf("token=%q", got)
	}
	got, err = bridge.RegisterSessionWithToken("owner", "codex", root, "shared-capability-token")
	if err != nil || got != "shared-capability-token" {
		t.Fatalf("stable registration token=%q err=%v", got, err)
	}
	if _, err := bridge.RegisterSessionWithToken("owner", "codex", root, "different-token"); err == nil {
		t.Fatal("same ACP session accepted a different host capability token")
	} else {
		assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
	}
}

func TestACPBrowserPrincipalBindsSessionProfileNotCapabilityToken(t *testing.T) {
	bridge := newTestACPBridge(t)
	root := t.TempDir()
	firstToken, err := bridge.RegisterSessionWithToken("session-a", "codex", root, "token-one")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := bridge.owner(firstToken)
	if err != nil {
		t.Fatal(err)
	}
	first := acpBrowserAuthPrincipal(owner)
	if err := bridge.ReleaseSession(context.Background(), "session-a"); err != nil {
		t.Fatal(err)
	}
	secondToken, err := bridge.RegisterSessionWithToken("session-a", "codex", root, "token-two")
	if err != nil {
		t.Fatal(err)
	}
	owner2, _ := bridge.owner(secondToken)
	second := acpBrowserAuthPrincipal(owner2)
	if first.ID == "" || first != second || first.Kind != "acp_bridge" {
		t.Fatalf("principal changed with capability rotation: %#v %#v", first, second)
	}
	otherToken, err := bridge.RegisterSession("session-b", "codex", root)
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, _ := bridge.owner(otherToken)
	if first.ID == acpBrowserAuthPrincipal(otherOwner).ID {
		t.Fatal("different ACP sessions shared browser principal")
	}
}

func TestACPBrowserAdmissionRunsBeforeBackendAndReleaseBypassesPolicy(t *testing.T) {
	bridge := newTestACPBridge(t)
	backend := &fakeLeaseBackend{}
	bridge.managed.manager = NewManagedLeaseManager(backend)
	token, err := bridge.RegisterSession("admission-browser", "codex", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	blocked := errors.New("permission blocked")
	calls := 0
	bridge.SetAdmissionHook(func(context.Context, permission.HostOperation) (permission.HostOperationFinish, error) {
		calls++
		return nil, blocked
	})
	if _, err := bridge.Acquire(context.Background(), token, "https://example.test"); !errors.Is(err, blocked) {
		t.Fatalf("Acquire error=%v", err)
	}
	if backend.starts != 0 || calls != 1 {
		t.Fatalf("backend starts=%d admission calls=%d", backend.starts, calls)
	}

	bridge.SetAdmissionHook(nil)
	meta, err := bridge.Acquire(context.Background(), token, "")
	if err != nil {
		t.Fatal(err)
	}
	beforeReleaseCalls := calls
	bridge.SetAdmissionHook(func(context.Context, permission.HostOperation) (permission.HostOperationFinish, error) {
		calls++
		return nil, blocked
	})
	if _, err := bridge.Release(context.Background(), token, meta.BrowserLeaseID); err != nil {
		t.Fatalf("Release was blocked by permission hook: %v", err)
	}
	if calls != beforeReleaseCalls {
		t.Fatalf("release invoked admission hook: before=%d after=%d", beforeReleaseCalls, calls)
	}
}
