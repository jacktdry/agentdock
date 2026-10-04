package app

import (
	"context"
	"strings"
	"time"
)

func browserToolSpecs() []ToolSpec {
	return []ToolSpec{
		{Name: "browser_broker", Contract: browserToolContract, Title: "Browser Broker control plane", Description: "Inspect Browser Broker ownership, leases, workers, queue, TTL and cleanup state, or run bounded AgentDock-owned stale/session cleanup. Capability tokens are never returned and external browser/profile processes are preserved.", Annotations: mutatingToolAnnotations(false, true), Availability: requiresACPBrowser, Handler: func(ctx context.Context, r *Runtime, args map[string]any) (Result, error) {
			if r.acpBrowser == nil {
				return nil, toolError("BROWSER_BROKER_UNAVAILABLE", "Browser Broker is unavailable in this runtime", "runtime")
			}
			action, _ := args["action"].(string)
			result := Result{"browser_broker_ok": true}
			var cleanupErr error
			switch action {
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
