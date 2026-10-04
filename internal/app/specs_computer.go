package app

import (
	"context"
	"strings"
)

func computerToolSpecs() []ToolSpec {
	return []ToolSpec{
		{Name: "computer_broker", Contract: computerToolContract, Title: "Computer Control Broker control plane", Description: "Inspect Computer Control ownership, active/released sessions and bounded recent focus/provider events, or clean up one ACP-owned Computer Control session.", Annotations: mutatingToolAnnotations(false, true), Handler: func(ctx context.Context, r *Runtime, args map[string]any) (Result, error) {
			if r.computer == nil || r.computer.Broker() == nil {
				return nil, toolError("COMPUTER_BROKER_UNAVAILABLE", "Computer Control Broker is unavailable", "runtime")
			}
			action, _ := args["action"].(string)
			result := Result{"computer_broker_ok": true}
			var cleanupErr error
			switch action {
			case "status":
			case "cleanup_acp_session":
				sessionID, _ := args["owner_acp_session_id"].(string)
				sessionID = strings.TrimSpace(sessionID)
				if sessionID == "" {
					return nil, toolError("INVALID_ARGUMENT", "owner_acp_session_id is required for cleanup_acp_session", "validation")
				}
				if r.acpComputer == nil {
					cleanupErr = toolError("COMPUTER_BROKER_UNAVAILABLE", "ACP Computer Control bridge is unavailable", "runtime")
				} else {
					cleanupErr = r.acpComputer.ReleaseSession(ctx, sessionID)
				}
			default:
				return nil, toolError("INVALID_ARGUMENT", "unsupported Computer Control Broker action", "validation")
			}
			if cleanupErr != nil {
				result["computer_broker_ok"] = false
				result["cleanup_error"] = cleanupErr.Error()
			}
			result["diagnostics"] = r.computer.Broker().Diagnostics()
			return result, nil
		}},
		{Name: "computer_session", Contract: computerToolContract, Title: "Computer control session", Description: "Acquire or release an AgentDock Computer Control Broker session. Orca is the default provider; foreground native GUI control is forbidden unless explicitly acquired with foreground=allowed.", Annotations: mutatingToolAnnotations(false, true), Handler: func(ctx context.Context, r *Runtime, args map[string]any) (Result, error) {
			return r.computer.HandleSession(ctx, args)
		}},
		{Name: "computer_observe", Contract: computerToolContract, Title: "Computer observation", Description: "Inspect computer-use capabilities, running apps/windows, accessibility state, or provider permissions through AgentDock Computer Control Broker. permissions and restore_window are foreground-gated and never run silently under foreground=forbidden.", Annotations: mutatingToolAnnotations(false, true), Handler: func(ctx context.Context, r *Runtime, args map[string]any) (Result, error) {
			return r.computer.HandleObserve(ctx, args)
		}},
		{Name: "computer_act", Contract: computerToolContract, Title: "Computer action", Description: "Perform an explicit native GUI action through AgentDock Computer Control Broker. Current native actions are foreground-required and are rejected before provider execution unless the session was acquired with foreground=allowed.", Annotations: mutatingToolAnnotations(true, true), Handler: func(ctx context.Context, r *Runtime, args map[string]any) (Result, error) {
			return r.computer.HandleAct(ctx, args)
		}},
	}
}
