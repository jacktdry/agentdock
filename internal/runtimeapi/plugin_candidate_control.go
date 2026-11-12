package runtimeapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/uvwt/agentdock/internal/app"
	toolplugin "github.com/uvwt/agentdock/internal/tool/plugin"
)

// DispatchDesktopPluginCandidate is intentionally separate from the normal
// Runtime dispatch surface because its prepare action accepts a native
// filesystem path. Its caller must enforce direct-loopback transport and
// Desktop control authentication before invoking it.
func DispatchDesktopPluginCandidate(ctx context.Context, runtime PluginDesktopCandidateRuntime, request Request) (map[string]any, error) {
	if runtime == nil {
		return nil, toolplugin.DesktopError("PLUGIN_CORE_UNAVAILABLE", "unavailable")
	}
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	path := strings.TrimSuffix(strings.TrimSpace(request.Path), "/")
	if method != http.MethodPost || path != "/internal/runtime/plugin/desktop/candidate" {
		return nil, &app.ToolError{Code: "NOT_FOUND", Message: "Desktop Plugin candidate route not found", Category: "not_found"}
	}
	typed, err := toolplugin.DecodeDesktopCandidateRequest(request.Body)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(typed)
	var args map[string]any
	_ = json.Unmarshal(body, &args)
	result, err := runtime.RuntimePluginDesktopCandidate(ctx, args)
	return map[string]any(result), err
}
