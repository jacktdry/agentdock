package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/app"
	"github.com/uvwt/agentdock/internal/auth"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	"github.com/uvwt/agentdock/internal/permission"
	"github.com/uvwt/agentdock/internal/runtimeapi"
)

type permissionHTTPRuntime struct {
	*app.Runtime
	begins, applies int
	workspace       string
	applyError      error
}

func (r *permissionHTTPRuntime) BeginPermissionConfirmation(credential string, input permission.ControlMutationRequest) (permission.ConfirmationChallenge, error) {
	r.begins++
	return r.Runtime.BeginPermissionConfirmation(credential, input)
}
func (r *permissionHTTPRuntime) ApplyPermissionControlMutation(ctx context.Context, credential, id string, input permission.ControlMutationRequest) (app.Result, error) {
	r.applies++
	if r.applyError != nil {
		return nil, r.applyError
	}
	return r.Runtime.ApplyPermissionControlMutation(ctx, credential, id, input)
}
func permissionHTTPFixture(t *testing.T, token string) (*permissionHTTPRuntime, *http.ServeMux) {
	t.Helper()
	cfg := testConfig(t)
	cfg.AuthToken = token
	rt, err := app.NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	wrapped := &permissionHTTPRuntime{Runtime: rt, workspace: cfg.AgentDockDefaultDir}
	mux := http.NewServeMux()
	registerRuntimeAPI(mux, wrapped, cfg, auth.NewOAuthStore())
	return wrapped, mux
}
func permissionHTTPCall(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if raw, ok := body.(string); ok {
		data = []byte(raw)
	} else if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "http://127.0.0.1:8767"+path, strings.NewReader(string(data)))
	req.RemoteAddr = "127.0.0.1:12345"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
func permissionHTTPChallenge(t *testing.T, h http.Handler, token string, input permission.ControlMutationRequest) string {
	t.Helper()
	w := permissionHTTPCall(t, h, "POST", "/internal/desktop-control/permission-confirmations", token, input)
	var result struct {
		Confirmation permission.ConfirmationChallenge
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Confirmation.ID == "" {
		t.Fatalf("confirmation: %d %s", w.Code, w.Body)
	}
	return result.Confirmation.ID
}
func TestPermissionHTTPAuthorityAndStrictBodyBeforeHandlers(t *testing.T) {
	for _, normalToken := range []string{"", "ordinary-mcp-token"} {
		t.Run(normalToken, func(t *testing.T) {
			rt, mux := permissionHTTPFixture(t, normalToken)
			credential := rt.DesktopPermissionControlCredential()
			for _, token := range []string{"", "ordinary-mcp-token", "oauth-token", "nexus-token", "provider-token"} {
				w := permissionHTTPCall(t, mux, "POST", "/internal/desktop-control/permission-confirmations", token, `{}`)
				if w.Code != 401 {
					t.Fatalf("normal credential authorized: %d", w.Code)
				}
			}
			if rt.begins != 0 || rt.applies != 0 {
				t.Fatal("unauthorized reached mutation handlers")
			}
			for _, body := range []string{`null`, `[]`, `{}`, `{"kind":"reject","actor":"forged"}`, `{"policy":{"settings":{"unknown":1}}}`, `{} {}`, strings.Repeat(" ", runtimeapi.MaxPermissionRequestBytes+1)} {
				before := rt.begins
				w := permissionHTTPCall(t, mux, "POST", "/internal/desktop-control/permission-confirmations", credential, body)
				if w.Code != 400 {
					t.Fatalf("strict body accepted: %d %s", w.Code, w.Body)
				}
				// {} reaches Core normalization, other malformed shapes do not.
				if body != `{}` && rt.begins != before {
					t.Fatalf("malformed body reached Core: %q", body[:min(len(body), 60)])
				}
			}
			req := httptest.NewRequest("POST", "http://127.0.0.1:8767/internal/desktop-control/permissions", strings.NewReader(`{}`))
			req.RemoteAddr = "127.0.0.1:12345"
			req.Header.Set("Authorization", "Bearer "+credential)
			req.Header.Set("X-Forwarded-For", "127.0.0.1")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != 403 || rt.applies != 0 {
				t.Fatal("proxy reached control handler")
			}
			w = permissionHTTPCall(t, mux, "GET", "/internal/desktop-control/permissions", credential, nil)
			if w.Code != 405 || w.Header().Get("Allow") != "POST" {
				t.Fatal("control method contract")
			}
			for _, path := range []string{"/internal/runtime/permissions", "/internal/runtime/approvals"} {
				w = permissionHTTPCall(t, mux, "GET", path, normalToken, nil)
				if w.Code != 200 || strings.Contains(w.Body.String(), credential) {
					t.Fatalf("read projection: %d %s", w.Code, w.Body)
				}
				w = permissionHTTPCall(t, mux, "POST", path, credential, `{}`)
				if w.Code != 405 {
					t.Fatal("normal Runtime route accepts permission mutation")
				}
			}
		})
	}
}
func TestPermissionHTTPConfirmationPolicyReplayAndConflict(t *testing.T) {
	rt, mux := permissionHTTPFixture(t, "ordinary-token")
	credential := rt.DesktopPermissionControlCredential()
	policy := permission.DefaultPolicy()
	policy.GlobalMode = permission.Rules
	mutation := permission.ControlMutationRequest{Kind: permission.ControlMutationUpdatePolicy, PolicyRevision: 1, Policy: &policy}
	id := permissionHTTPChallenge(t, mux, credential, mutation)
	payload := runtimeapi.PermissionControlRequest{ControlMutationRequest: mutation, ConfirmationID: id}
	w := permissionHTTPCall(t, mux, "POST", "/internal/desktop-control/permissions", credential, payload)
	if w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body)
	}
	w = permissionHTTPCall(t, mux, "POST", "/internal/desktop-control/permissions", credential, payload)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "CONFIRMATION_NOT_FOUND") {
		t.Fatal("replay accepted")
	}
	payload.ConfirmationID = permissionHTTPChallenge(t, mux, credential, mutation)
	w = permissionHTTPCall(t, mux, "POST", "/internal/desktop-control/permissions", credential, payload)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "POLICY_REVISION_CONFLICT") {
		t.Fatalf("revision: %d %s", w.Code, w.Body)
	}
	payload.ConfirmationID = permissionHTTPChallenge(t, mux, credential, mutation)
	changed := policy
	changed.GlobalMode = permission.ReadOnly
	payload.Policy = &changed
	w = permissionHTTPCall(t, mux, "POST", "/internal/desktop-control/permissions", credential, payload)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "CONFIRMATION_MISMATCH") {
		t.Fatal("broader payload reused confirmation")
	}
}
func TestPermissionHTTPApprovalDoesNotDispatchUntilExactRetry(t *testing.T) {
	for _, kind := range []string{permission.ControlMutationApproveOnce, permission.ControlMutationApproveWorkspace, permission.ControlMutationReject, "approve_workspace_trusted"} {
		t.Run(kind, func(t *testing.T) {
			rt, mux := permissionHTTPFixture(t, "")
			credential := rt.DesktopPermissionControlCredential()
			policy := permission.DefaultPolicy()
			policy.GlobalMode = permission.Rules
			update := permission.ControlMutationRequest{Kind: permission.ControlMutationUpdatePolicy, PolicyRevision: 1, Policy: &policy}
			id := permissionHTTPChallenge(t, mux, credential, update)
			w := permissionHTTPCall(t, mux, "POST", "/internal/desktop-control/permissions", credential, runtimeapi.PermissionControlRequest{ControlMutationRequest: update, ConfirmationID: id})
			if w.Code != 200 {
				t.Fatal(w.Body)
			}
			ctx := requestmeta.WithAuthPrincipal(context.Background(), requestmeta.NewStableAuthPrincipal("static_bearer", "api-test"))
			target := filepath.Join(t.TempDir(), "operation.txt")
			if kind == "approve_workspace_trusted" {
				target = filepath.Join(rt.workspace, "operation.txt")
			}
			args := map[string]any{"action": "add", "path": target, "content": "approved retry"}
			_, err := rt.Call(ctx, "file_edit", args)
			var te *app.ToolError
			if !errors.As(err, &te) || te.Code != "APPROVAL_REQUIRED" {
				t.Fatalf("Ask: %v", err)
			}
			input := permission.ControlMutationRequest{Kind: strings.TrimSuffix(kind, "_trusted"), ApprovalID: te.Details["approval_id"].(string), ApprovalVersion: te.Details["approval_version"].(uint64), PolicyRevision: te.Details["policy_revision"].(uint64)}
			// A stale approval version is an explicit conflict and never dispatches.
			stale := input
			stale.ApprovalVersion++
			id = permissionHTTPChallenge(t, mux, credential, stale)
			w = permissionHTTPCall(t, mux, "POST", "/internal/desktop-control/approvals", credential, runtimeapi.PermissionControlRequest{ControlMutationRequest: stale, ConfirmationID: id})
			if w.Code != 409 || !strings.Contains(w.Body.String(), "APPROVAL_VERSION_CONFLICT") {
				t.Fatalf("version: %d %s", w.Code, w.Body)
			}
			id = permissionHTTPChallenge(t, mux, credential, input)
			w = permissionHTTPCall(t, mux, "POST", "/internal/desktop-control/approvals", credential, runtimeapi.PermissionControlRequest{ControlMutationRequest: input, ConfirmationID: id})
			if strings.HasPrefix(kind, permission.ControlMutationApproveWorkspace) {
				// Built-in file_edit has no Core-enforceable WorkspaceRuleEligible class, even inside the workspace.
				if w.Code != 403 || !strings.Contains(w.Body.String(), "APPROVAL_NOT_ELIGIBLE") {
					t.Fatalf("workspace ceiling: %d %s", w.Code, w.Body)
				}
			} else if w.Code != 200 || !strings.Contains(w.Body.String(), `"decided_by":"desktop-control"`) {
				t.Fatalf("decision: %d %s", w.Code, w.Body)
			}
			if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("approval dispatched original handler")
			}
			_, err = rt.Call(ctx, "file_edit", args)
			if kind == permission.ControlMutationApproveOnce {
				if err != nil {
					t.Fatalf("retry: %v", err)
				}
				if data, err := os.ReadFile(target); err != nil || string(data) != "approved retry" {
					t.Fatal("retry failed to dispatch")
				}
			} else if !errors.As(err, &te) || te.Code != "APPROVAL_REQUIRED" {
				t.Fatalf("unapproved retry: %v", err)
			}
		})
	}
}
func TestPermissionHTTPErrorMappingIsBounded(t *testing.T) {
	rt, mux := permissionHTTPFixture(t, "")
	input := runtimeapi.PermissionControlRequest{ControlMutationRequest: permission.ControlMutationRequest{Kind: permission.ControlMutationReject, ApprovalID: "approval", ApprovalVersion: 1, PolicyRevision: 1}, ConfirmationID: "challenge"}
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{permission.ErrConfirmationExpired, 409, "CONFIRMATION_EXPIRED"}, {permission.ErrExpired, 409, "APPROVAL_EXPIRED"},
		{permission.ErrNotFound, 404, "APPROVAL_NOT_FOUND"}, {permission.ErrConfirmationLimit, 429, "PERMISSION_LIMIT_REACHED"},
		{errors.New("secret credential /private/state/path"), 500, "PERMISSION_STATE_ERROR"},
	} {
		rt.applyError = test.err
		w := permissionHTTPCall(t, mux, "POST", "/internal/desktop-control/approvals", rt.DesktopPermissionControlCredential(), input)
		if w.Code != test.status || !strings.Contains(w.Body.String(), test.code) || strings.Contains(w.Body.String(), "secret credential") {
			t.Fatalf("mapping: %d %s", w.Code, w.Body)
		}
	}
}

func TestRuntimeHTTPApprovalErrorPreservesSafeRetryBinding(t *testing.T) {
	w := httptest.NewRecorder()
	writeRuntimeAPIHandlerError(w, &app.ToolError{Code: "APPROVAL_REQUIRED", Category: "permission", Message: "approval required", Details: map[string]any{"approval_id": "approval-safe", "approval_version": uint64(2), "policy_revision": uint64(3), "executed": false, "retry": "retry after approval", "credential": "must-not-leak"}})
	if w.Code != 409 || !strings.Contains(w.Body.String(), "approval-safe") || strings.Contains(w.Body.String(), "must-not-leak") {
		t.Fatalf("approval retry binding: %d %s", w.Code, w.Body)
	}
}
