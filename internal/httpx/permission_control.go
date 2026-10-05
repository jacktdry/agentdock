package httpx

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/runtimeapi"
)

func desktopPermissionControlHandler(runtime runtimeapi.Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isDirectLoopbackRequest(r) {
			writeRuntimeAPIError(w, http.StatusForbidden, "LOCAL_ACCESS_REQUIRED", "Desktop permission control requires direct local access")
			return
		}
		control, ok := runtime.(runtimeapi.DesktopPermissionControlRuntime)
		authorization := r.Header.Get("Authorization")
		credential := ""
		if strings.HasPrefix(authorization, "Bearer ") {
			credential = strings.TrimPrefix(authorization, "Bearer ")
		}
		if !ok || !control.AuthenticateDesktopPermissionControl(credential) {
			writeRuntimeAPIError(w, http.StatusUnauthorized, "DESKTOP_CONTROL_UNAUTHORIZED", "Desktop control authentication is required")
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeRuntimeAPIError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, runtimeapi.MaxPermissionRequestBytes+1))
		if err != nil {
			writeRuntimeAPIError(w, http.StatusBadRequest, "INVALID_PERMISSION_MUTATION", "failed to read permission mutation body")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		result, err := runtimeapi.DispatchDesktopPermissionControl(ctx, control, credential, runtimeapi.Request{Method: r.Method, Path: r.URL.Path, Body: body})
		if err != nil {
			writeRuntimeAPIHandlerError(w, err)
			return
		}
		writeJSON(w, result)
	}
}
