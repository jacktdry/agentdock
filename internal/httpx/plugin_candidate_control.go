package httpx

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/uvwt/agentdock/internal/runtimeapi"
)

// desktopPluginCandidateHandler is deliberately separate from the normal
// Runtime API handler. Candidate preparation can read a native filesystem path,
// so only the native Desktop control credential may authorize it.
func desktopPluginCandidateHandler(runtime runtimeapi.Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isDirectLoopbackRequest(r) {
			writeRuntimeAPIError(w, http.StatusForbidden, "LOCAL_ACCESS_REQUIRED", "Desktop Plugin candidate staging requires direct local access")
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeRuntimeAPIError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		control, ok := runtime.(runtimeapi.DesktopPermissionControlRuntime)
		candidateRuntime, candidateOK := runtime.(runtimeapi.PluginDesktopCandidateRuntime)
		authorization := r.Header.Get("Authorization")
		credential := ""
		if strings.HasPrefix(authorization, "Bearer ") {
			credential = strings.TrimPrefix(authorization, "Bearer ")
		}
		if !ok || !candidateOK || !control.AuthenticateDesktopPermissionControl(credential) {
			writeRuntimeAPIError(w, http.StatusUnauthorized, "DESKTOP_CONTROL_UNAUTHORIZED", "Desktop control authentication is required")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024+1))
		if err != nil || len(body) > 64*1024 {
			writeRuntimeAPIError(w, http.StatusBadRequest, "PLUGIN_CANDIDATE_INVALID", "invalid Plugin candidate request")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), runtimeMCPDesktopTimeout)
		defer cancel()
		result, err := runtimeapi.DispatchDesktopPluginCandidate(ctx, candidateRuntime, runtimeapi.Request{
			Method: r.Method, Path: r.URL.Path, Body: body,
		})
		if err != nil {
			writeRuntimeAPIHandlerError(w, err)
			return
		}
		writeJSON(w, result)
	}
}
