package computer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

func newTestComputerACPBridge(t *testing.T) (*ACPBridge, *fakeProvider) {
	t.Helper()
	provider := &fakeProvider{}
	broker, err := NewBroker(provider)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := NewACPBridge(broker)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = bridge.Close()
		_ = broker.Close()
	})
	return bridge, provider
}

func computerMCPRequest(t *testing.T, handler http.Handler, token string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/acp-computer/mcp", bytes.NewReader(body))
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

func TestACPComputerMCPRequiresCapabilityAndOnlyExposesComputerTools(t *testing.T) {
	bridge, _ := newTestComputerACPBridge(t)
	token, err := bridge.RegisterSession("acps-one", "codex")
	if err != nil {
		t.Fatal(err)
	}
	handler := bridge.MCPHTTPHandler()
	unauthorized := computerMCPRequest(t, handler, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{}})
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.Code)
	}
	initialize := computerMCPRequest(t, handler, token, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}},
	})
	if initialize.Code != http.StatusOK {
		t.Fatalf("initialize status=%d body=%s", initialize.Code, initialize.Body.String())
	}
	listed := computerMCPRequest(t, handler, token, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}})
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
	want := []string{"computer_acquire", "computer_act", "computer_observe", "computer_release"}
	sort.Strings(want)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tools=%v want=%v", names, want)
	}
	for _, name := range names {
		if strings.HasPrefix(name, "browser_") {
			t.Fatalf("computer MCP leaked browser tool %q", name)
		}
	}
}

func TestACPComputerCapabilityCannotUseAnotherSessionsComputerSession(t *testing.T) {
	bridge, provider := newTestComputerACPBridge(t)
	tokenA, err := bridge.RegisterSession("acps-a", "codex")
	if err != nil {
		t.Fatal(err)
	}
	tokenB, err := bridge.RegisterSession("acps-b", "codex")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := bridge.Acquire(tokenA, CapabilityObserve, ForegroundForbidden)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.Observe(context.Background(), tokenB, meta.SessionID, ObservationRequest{Action: "capabilities"}); err == nil {
		t.Fatal("cross-session computer observation succeeded")
	} else {
		var typed *Error
		if !errorsAs(err, &typed) || typed.Code != ErrOwnerMismatch {
			t.Fatalf("err=%v", err)
		}
	}
	provider.mu.Lock()
	calls := provider.observeCall
	provider.mu.Unlock()
	if calls != 0 {
		t.Fatalf("cross-session request reached provider: %d", calls)
	}
}

func TestACPComputerForegroundGateRunsBeforeProvider(t *testing.T) {
	bridge, provider := newTestComputerACPBridge(t)
	token, _ := bridge.RegisterSession("acps-one", "codex")
	meta, err := bridge.Acquire(token, CapabilityAct, ForegroundForbidden)
	if err != nil {
		t.Fatal(err)
	}
	_, err = bridge.Act(context.Background(), token, meta.SessionID, ActionRequest{Action: "click", App: "Test", ElementIndex: intPtr(1)})
	var typed *Error
	if !errorsAs(err, &typed) || typed.Code != ErrForegroundRequired {
		t.Fatalf("err=%v", err)
	}
	provider.mu.Lock()
	calls := provider.actCall
	provider.mu.Unlock()
	if calls != 0 {
		t.Fatalf("provider action calls=%d", calls)
	}
}

func TestACPComputerReleaseSessionRevokesTokenAndOwnedSessions(t *testing.T) {
	bridge, _ := newTestComputerACPBridge(t)
	token, err := bridge.RegisterSessionWithToken("owner", "antigravity", "shared-capability-token")
	if err != nil {
		t.Fatal(err)
	}
	if token != "shared-capability-token" {
		t.Fatalf("token=%q", token)
	}
	meta, err := bridge.Acquire(token, CapabilityObserve, ForegroundForbidden)
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.ReleaseSession(context.Background(), "owner"); err != nil {
		t.Fatal(err)
	}
	if err := bridge.ReleaseSession(context.Background(), "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.Observe(context.Background(), token, meta.SessionID, ObservationRequest{Action: "capabilities"}); err == nil {
		t.Fatal("revoked capability still authorized")
	}
	rec := computerMCPRequest(t, bridge.MCPHTTPHandler(), token, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{}})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked HTTP token status=%d body=%s", rec.Code, rec.Body.String())
	}
	if renewed, err := bridge.RegisterSession("owner", "antigravity"); err != nil || renewed == token {
		t.Fatalf("renewed=%q err=%v", renewed, err)
	}
}

func TestACPComputerHTTPForegroundDeniedReturnsStructuredError(t *testing.T) {
	bridge, provider := newTestComputerACPBridge(t)
	token, _ := bridge.RegisterSession("owner", "codex")
	h := bridge.MCPHTTPHandler()
	acquire := computerMCPRequest(t, h, token, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "computer_acquire", "arguments": map[string]any{"capability": "act", "foreground": "forbidden"}}})
	var response struct {
		Result struct {
			Structured map[string]any `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(acquire.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	id, _ := response.Result.Structured["computer_session_id"].(string)
	if id == "" {
		t.Fatalf("acquire body=%s", acquire.Body.String())
	}
	acted := computerMCPRequest(t, h, token, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "computer_act", "arguments": map[string]any{"session_id": id, "action": "click", "app": "Test", "element_index": 1}}})
	if acted.Code != http.StatusOK || !strings.Contains(acted.Body.String(), ErrForegroundRequired) || !strings.Contains(acted.Body.String(), `"isError":true`) {
		t.Fatalf("act status=%d body=%s", acted.Code, acted.Body.String())
	}
	provider.mu.Lock()
	calls := provider.actCall
	provider.mu.Unlock()
	if calls != 0 {
		t.Fatalf("provider action calls=%d", calls)
	}
}

// Keep the test helper local so the production package stays on standard errors.As.
func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}

func TestACPComputerPrincipalBindsSessionProfileNotCapabilityToken(t *testing.T) {
	bridge, _ := newTestComputerACPBridge(t)
	firstToken, err := bridge.RegisterSessionWithToken("session-a", "codex", "token-one")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := bridge.owner(firstToken)
	if err != nil {
		t.Fatal(err)
	}
	first := acpComputerAuthPrincipal(owner)
	if err := bridge.ReleaseSession(context.Background(), "session-a"); err != nil {
		t.Fatal(err)
	}
	secondToken, err := bridge.RegisterSessionWithToken("session-a", "codex", "token-two")
	if err != nil {
		t.Fatal(err)
	}
	owner2, _ := bridge.owner(secondToken)
	second := acpComputerAuthPrincipal(owner2)
	if first.ID == "" || first != second || first.Kind != "acp_bridge" {
		t.Fatalf("principal changed with capability rotation: %#v %#v", first, second)
	}
	otherToken, err := bridge.RegisterSession("session-b", "codex")
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, _ := bridge.owner(otherToken)
	if first.ID == acpComputerAuthPrincipal(otherOwner).ID {
		t.Fatal("different ACP sessions shared computer principal")
	}
}
