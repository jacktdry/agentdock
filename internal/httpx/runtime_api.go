package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/app"
	"github.com/uvwt/agentdock/internal/auth"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	"github.com/uvwt/agentdock/internal/runtimeapi"
)

func registerRuntimeAPI(mux *http.ServeMux, runtime runtimeapi.Runtime, cfg config.Config, oauthStore *auth.OAuthStore) {
	h := runtimeAPIHandler(runtime, cfg, oauthStore)
	mux.HandleFunc("/internal/runtime/activity/stream", runtimeActivityStreamHandler(runtime, cfg, oauthStore))
	mux.HandleFunc("/internal/runtime/status", h)
	mux.HandleFunc("/internal/runtime/permissions", h)
	mux.HandleFunc("/internal/runtime/approvals", h)
	control := desktopPermissionControlHandler(runtime)
	for _, path := range []string{"/internal/desktop-control/permission-confirmations", "/internal/desktop-control/permissions", "/internal/desktop-control/approvals"} {
		mux.HandleFunc(path, control)
	}
	mux.HandleFunc("/internal/runtime/analytics", h)
	mux.HandleFunc("/internal/runtime/diagnostics", h)
	mux.HandleFunc("/internal/runtime/execution", h)
	mux.HandleFunc("/internal/runtime/activity", h)
	mux.HandleFunc("/internal/runtime/insertions", h)
	mux.HandleFunc("/internal/runtime/capabilities", h)
	mux.HandleFunc("/internal/runtime/skills", h)
	mux.HandleFunc("/internal/runtime/skills/", h)
	mux.HandleFunc("/internal/runtime/plugins", h)
	mux.HandleFunc("/internal/runtime/plugins/", h)
	mux.HandleFunc("/internal/runtime/tasks", h)
	mux.HandleFunc("/internal/runtime/tasks/", h)
	mux.HandleFunc("/internal/runtime/evolve", h)
	mux.HandleFunc("/internal/runtime/mcp", h)
	mux.HandleFunc("/internal/runtime/mcp/", h)
}

func runtimeAPIHandler(runtime runtimeapi.Runtime, cfg config.Config, oauthStore *auth.OAuthStore) http.HandlerFunc {
	authRequired := cfg.AuthRequired()
	return func(w http.ResponseWriter, r *http.Request) {
		if !runtimeapi.MethodAllowed(r.Method, r.URL.Path) {
			w.Header().Set("Allow", runtimeapi.AllowHeader(r.URL.Path))
			writeRuntimeAPIError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		authn := authenticateRequest(r, cfg, oauthStore)
		cleanPath := strings.TrimSuffix(r.URL.Path, "/")
		if cleanPath == "/internal/runtime/analytics" && !authRequired && !isDirectLoopbackRequest(r) {
			writeRuntimeAPIError(w, http.StatusForbidden, "LOCAL_ACCESS_REQUIRED", "runtime analytics requires local access or authentication")
			return
		}
		if (cleanPath == "/internal/runtime/execution" || cleanPath == "/internal/runtime/activity" || cleanPath == "/internal/runtime/insertions" || cleanPath == "/internal/runtime/permissions" || cleanPath == "/internal/runtime/approvals" || cleanPath == "/internal/runtime/mcp/desktop") && !isDirectLoopbackRequest(r) {
			writeRuntimeAPIError(w, http.StatusForbidden, "LOCAL_ACCESS_REQUIRED", "runtime execution state requires direct local access")
			return
		}
		if authRequired && !authn.OK {
			setBearerChallenge(w, cfg, r, strings.TrimSpace(r.Header.Get("Authorization")) != "")
			writeRuntimeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}

		body, err := runtimeRequestBody(r)
		if err != nil {
			writeRuntimeAPIError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "failed to read runtime request body")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		ctx = requestmeta.WithAuthPrincipal(ctx, authn.Principal)
		result, err := runtimeapi.Dispatch(ctx, runtime, runtimeapi.Request{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Body:   body,
		})
		if err != nil {
			writeRuntimeAPIHandlerError(w, err)
			return
		}
		writeJSON(w, result)
	}
}

func runtimeRequestBody(r *http.Request) ([]byte, error) {
	cleanPath := strings.TrimSuffix(r.URL.Path, "/")
	if r.Method != http.MethodPost || (cleanPath != "/internal/runtime/mcp" && cleanPath != "/internal/runtime/mcp/desktop" && cleanPath != "/internal/runtime/mcp/oauth/callback" && cleanPath != "/internal/runtime/evolve" && cleanPath != "/internal/runtime/insertions") {
		return nil, nil
	}
	return io.ReadAll(io.LimitReader(r.Body, 64*1024+1))
}

func writeRuntimeAPIHandlerError(w http.ResponseWriter, err error) {
	var toolErr *app.ToolError
	if errors.As(err, &toolErr) {
		status := http.StatusInternalServerError
		switch toolErr.Category {
		case "authentication":
			status = http.StatusUnauthorized
		case "permission":
			status = http.StatusForbidden
		case "conflict":
			status = http.StatusConflict
		case "capacity":
			status = http.StatusTooManyRequests
		case "validation":
			status = http.StatusBadRequest
		case "not_found":
			status = http.StatusNotFound
		}
		if toolErr.Code == "APPROVAL_REQUIRED" {
			w.Header().Set("content-type", "application/json")
			w.WriteHeader(http.StatusConflict)
			details := map[string]any{}
			for _, key := range []string{"approval_id", "approval_version", "policy_revision", "executed", "retry"} {
				if value, ok := toolErr.Details[key]; ok {
					details[key] = value
				}
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"ok": false, "code": toolErr.Code, "error": toolErr.Message, "details": details}); err != nil {
				slog.Warn("write runtime approval error failed", "error", err)
			}
			return
		}
		writeRuntimeAPIError(w, status, toolErr.Code, toolErr.Message)
		return
	}
	writeRuntimeAPIError(w, http.StatusInternalServerError, "RUNTIME_API_ERROR", err.Error())
}

func writeRuntimeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{"ok": false, "code": code, "error": message}); err != nil {
		slog.Warn("write runtime API error response failed", "status", status, "code", code, "error", err)
	}
}
