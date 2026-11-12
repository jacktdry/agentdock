package app

import (
	"context"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

func browserToolSpecs() []ToolSpec {
	return []ToolSpec{
		{Name: "browser_broker", Contract: browserToolContract, Title: "Browser Broker control plane", Description: "Acquire, navigate, snapshot, screenshot or release an authenticated direct lease; explicitly connect the configured company Edge once and reuse its authorized connection. Company Edge permits read-only scoped operations and never falls back to Managed Chrome. JavaScript evaluation is available on managed leases. Inspect ownership/lifecycle state or run bounded owned cleanup. Tokens and external browser/profile process authority are never returned.", Annotations: mutatingToolAnnotations(false, true), Availability: requiresACPBrowser, Handler: func(ctx context.Context, r *Runtime, args map[string]any) (Result, error) {
			if r.acpBrowser == nil {
				return nil, toolError("BROWSER_BROKER_UNAVAILABLE", "Browser Broker is unavailable in this runtime", "runtime")
			}
			action, _ := args["action"].(string)
			result := Result{"browser_broker_ok": true}
			var cleanupErr error
			switch action {
			case "connect_external", "disconnect_external", "direct_acquire", "direct_evaluate", "direct_snapshot", "direct_screenshot", "direct_navigate", "direct_release":
				root, err := r.browserBrokerWorkspace(args)
				if err != nil {
					return nil, toolError("BROWSER_SCOPE_REQUIRED", "configured browser workspace invalid", "validation")
				}
				if _, err := nextSharedBrowserWorkspace(r.cfg, root); err != nil {
					return nil, toolError("BROWSER_BROKER_UNAVAILABLE", "direct browser requires Next runtime", "runtime")
				}
				id, _ := args["lease_id"].(string)
				switch action {
				case "connect_external", "disconnect_external":
					if err := r.acpBrowser.ConnectExternal(ctx, root, action == "disconnect_external"); err != nil {
						return nil, err
					}
					return result, nil
				case "direct_acquire":
					url, _ := args["url"].(string)
					meta, err := r.acpBrowser.DirectAcquire(ctx, root, url)
					if err != nil {
						return nil, err
					}
					result["lease_id"] = meta.BrowserLeaseID
					result["cleanup_state"] = string(meta.CleanupState)
				case "direct_release":
					meta, err := r.acpBrowser.DirectRelease(ctx, root, id)
					if meta.BrowserLeaseID == "" {
						return nil, err
					}
					result["lease_id"], result["cleanup_state"] = meta.BrowserLeaseID, string(meta.CleanupState)
					if meta.CleanupError != "" {
						result["cleanup_error"] = meta.CleanupError
					}
					result["browser_broker_ok"] = err == nil
					if err != nil {
						return result, err
					}
				default:
					tool, callArgs := "take_snapshot", map[string]any{}
					if action == "direct_evaluate" {
						tool = "evaluate_script"
						callArgs["function"] = args["function"]
					}
					if action == "direct_screenshot" {
						tool = "take_screenshot"
					}
					if action == "direct_navigate" {
						tool = "navigate_page"
						callArgs["url"] = args["url"]
					}
					op, err := r.acpBrowser.DirectCall(ctx, root, id, tool, callArgs)
					if err != nil {
						return nil, err
					}
					result["lease_id"], result["operation_result"] = id, op
				}
				return result, nil
			case "status":
			case "cleanup_stale":
				cleanupErr = r.acpBrowser.CleanupStale(time.Now().UTC())
			case "cleanup_acp_session":
				sessionID, _ := args["owner_acp_session_id"].(string)
				sessionID = strings.TrimSpace(sessionID)
				if sessionID == "" {
					return nil, toolError("INVALID_ARGUMENT", "owner_acp_session_id is required for cleanup_acp_session", "validation")
				}
				cleanupErr = r.acpBrowser.ReleaseSession(ctx, sessionID)
			default:
				return nil, toolError("INVALID_ARGUMENT", "unsupported Browser Broker action", "validation")
			}
			if cleanupErr != nil {
				result["browser_broker_ok"] = false
				result["cleanup_error"] = cleanupErr.Error()
			}
			result["diagnostics"] = r.acpBrowser.Diagnostics()
			return result, nil
		}},
		{Name: "browser_session", Contract: browserToolContract, Title: "Browser session", Description: "Start an AgentDock-owned Chromium-family browser or attach to an existing CDP browser with a dedicated AgentDock target, then close or clean up the session. External browsers remain running when the session closes.", Annotations: mutatingToolAnnotations(true, true), Availability: requiresBrowser, Handler: func(ctx context.Context, r *Runtime, args map[string]any) (Result, error) {
			return r.browser.HandleSession(ctx, args)
		}},
		{Name: "browser_act", Contract: browserToolContract, Title: "Browser actions", Description: "Run strictly validated CSS/CDP browser actions against an AgentDock-managed browser target and return the final typed page snapshot plus screenshot Artifact.", Annotations: mutatingToolAnnotations(true, true), Availability: requiresBrowser, Handler: func(ctx context.Context, r *Runtime, args map[string]any) (Result, error) {
			return r.browser.HandleAct(ctx, args)
		}},
		{Name: "browser_snapshot", Contract: browserToolContract, Title: "Browser snapshot", Description: "Capture the active or requested CDP target with page text, viewport, page size, focus, visible interactive elements, diagnostics, and a PNG screenshot Artifact.", Annotations: readOnlyToolAnnotations(true), Availability: requiresBrowser, Handler: func(ctx context.Context, r *Runtime, args map[string]any) (Result, error) {
			return r.browser.HandleSnapshot(ctx, args)
		}},
	}
}

// A caller can choose an exact company root already admitted by private Core
// configuration, without changing the workspace used by unrelated sessions.
// No arbitrary parent root, relative path, browser/Profile or origin override.
func (r *Runtime) browserBrokerWorkspace(args map[string]any) (string, error) {
	selected, _ := args["workspace_root"].(string)
	defaultRoot, err := browserpolicy.CanonicalWorkspaceRoot(r.cfg.AgentDockDefaultDir)
	if selected == "" && err != nil {
		return "", err
	}
	if selected == "" || (err == nil && selected == defaultRoot) {
		return defaultRoot, nil
	}
	for _, mapping := range r.cfg.BrowserWorkspacePolicies {
		if mapping.Root != selected || mapping.Policy.Class != browserpolicy.WorkspaceCompany || mapping.Policy.Route != browserpolicy.RouteRequiredExternal {
			continue
		}
		root, err := browserpolicy.CanonicalWorkspaceRoot(selected)
		if err == nil && root == selected {
			return root, nil
		}
	}
	return "", toolError("BROWSER_SCOPE_REQUIRED", "workspace is not an exact configured company worktree", "validation")
}
