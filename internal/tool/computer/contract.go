package computer

import toolcontract "github.com/uvwt/agentdock/internal/tool/contract"

const (
	ToolSession = "computer_session"
	ToolObserve = "computer_observe"
	ToolAct     = "computer_act"
	ToolBroker  = "computer_broker"
)

func InputSchema(name string) (map[string]any, bool) {
	stringProp := toolcontract.String
	boundedInt := toolcontract.BoundedInteger
	boolProp := toolcontract.Boolean
	props := map[string]any{}
	var required []string

	windowProps := func() {
		props["app"] = stringProp("Desktop app name, bundle id, or pid:N. Required for app/window state and actions.")
		props["window_id"] = boundedInt("Window id returned by computer_observe list_windows.", 0, 2147483647)
		props["window_index"] = boundedInt("Window index returned by computer_observe list_windows.", 0, 10000)
	}

	switch name {
	case ToolBroker:
		props["action"] = map[string]any{"type": "string", "enum": []string{"status", "cleanup_acp_session"}, "description": "Computer Control Broker control-plane action."}
		props["owner_acp_session_id"] = stringProp("ACP session owner to release for action=cleanup_acp_session.")
		required = []string{"action"}
	case ToolSession:
		props["action"] = map[string]any{"type": "string", "enum": []string{"acquire", "release"}, "description": "Computer control session lifecycle action."}
		props["session_id"] = stringProp("Computer control session id for action=release.")
		props["capability"] = map[string]any{"type": "string", "enum": []string{"observe", "act"}, "description": "Maximum capability requested for this session. Defaults to observe."}
		props["foreground"] = map[string]any{"type": "string", "enum": []string{"forbidden", "allowed"}, "description": "Whether foreground-changing native GUI operations may run. Defaults to forbidden."}
		props["owner_task_id"] = stringProp("Optional upstream task/request correlation id for diagnostics.")
		required = []string{"action"}
	case ToolObserve:
		props["session_id"] = stringProp("Computer control session id.")
		props["action"] = map[string]any{"type": "string", "enum": []string{"capabilities", "permissions", "list_apps", "list_windows", "get_app_state"}, "description": "Computer observation action. permissions may open provider permission setup and therefore requires foreground=allowed."}
		windowProps()
		props["restore_window"] = boolProp("Bring the target window forward before get_app_state. Requires a session acquired with foreground=allowed.")
		props["no_screenshot"] = boolProp("Skip screenshot capture when tree state is sufficient.")
		props["timeout_ms"] = boundedInt("Operation timeout in milliseconds. Defaults to 30000 and is capped at 300000.", 1, 300000)
		required = []string{"session_id", "action"}
	case ToolAct:
		props["session_id"] = stringProp("Computer control session id acquired with capability=act and foreground=allowed.")
		props["action"] = map[string]any{"type": "string", "enum": []string{"click", "set_value", "type_text", "paste_text", "press_key", "hotkey", "scroll", "drag", "perform_secondary_action"}, "description": "Native GUI action. All current actions are foreground-required."}
		windowProps()
		props["element_index"] = boundedInt("Element index from a fresh accessibility snapshot.", 0, 1000000)
		props["x"] = boundedInt("Window-local x coordinate.", -100000, 100000)
		props["y"] = boundedInt("Window-local y coordinate.", -100000, 100000)
		props["from_element_index"] = boundedInt("Drag source element index.", 0, 1000000)
		props["to_element_index"] = boundedInt("Drag destination element index.", 0, 1000000)
		props["from_x"] = boundedInt("Drag source x coordinate.", -100000, 100000)
		props["from_y"] = boundedInt("Drag source y coordinate.", -100000, 100000)
		props["to_x"] = boundedInt("Drag destination x coordinate.", -100000, 100000)
		props["to_y"] = boundedInt("Drag destination y coordinate.", -100000, 100000)
		props["value"] = stringProp("Value for set_value. Sent to Orca through stdin, not process arguments.")
		props["text"] = stringProp("Text for type_text/paste_text. Sent to Orca through stdin, not process arguments.")
		props["key"] = stringProp("Key or platform-aware hotkey chord.")
		props["direction"] = map[string]any{"type": "string", "enum": []string{"up", "down", "left", "right"}}
		props["secondary_action"] = stringProp("Advertised accessibility secondary action name.")
		props["modifiers"] = stringProp("Modifier chord for click, such as CmdOrCtrl+Shift.")
		props["mouse_button"] = map[string]any{"type": "string", "enum": []string{"left", "right", "middle"}}
		props["restore_window"] = boolProp("Bring the target app/window forward before the action. The session must already allow foreground control.")
		props["no_screenshot"] = boolProp("Skip screenshot capture after the action.")
		props["timeout_ms"] = boundedInt("Operation timeout in milliseconds. Defaults to 30000 and is capped at 300000.", 1, 300000)
		required = []string{"session_id", "action", "app"}
	default:
		return nil, false
	}
	return toolcontract.InputObject(props, required...), true
}

func OutputSchema(name string) (map[string]any, bool) {
	stringProp := toolcontract.String
	boolProp := toolcontract.Boolean
	objectProp := toolcontract.OpenObject
	props := map[string]any{
		"computer_ok":         boolProp("Whether the Computer Control Broker operation succeeded."),
		"code":                stringProp("Stable computer-control error code when computer_ok=false."),
		"error":               objectProp("Structured computer-control error."),
		"computer_session_id": stringProp("AgentDock computer-control session id."),
		"computer_provider":   stringProp("Selected Computer Control provider. Currently orca."),
		"foreground_policy":   stringProp("Session foreground policy: forbidden or allowed."),
	}
	switch name {
	case ToolBroker:
		props["computer_broker_ok"] = boolProp("Whether the Computer Control Broker control-plane operation completed successfully.")
		props["diagnostics"] = objectProp("Computer Control Broker active/released sessions and bounded recent focus/provider events.")
		props["cleanup_error"] = stringProp("Cleanup error summary when releasing an ACP-owned computer session failed.")
	case ToolSession:
		props["capability"] = stringProp("Maximum session capability: observe or act.")
		props["owner"] = objectProp("Session owner metadata for diagnostics.")
		props["cleanup_state"] = stringProp("Session cleanup state after release.")
	case ToolObserve, ToolAct:
		props["action_kind"] = stringProp("Computer observation/action kind.")
		props["foreground_required"] = boolProp("Whether this operation requires foreground control.")
		props["active_app_before"] = objectProp("Best-effort frontmost app identity before provider execution.")
		props["active_app_after"] = objectProp("Best-effort frontmost app identity after provider execution.")
		props["focus_changed"] = boolProp("Whether best-effort active-app identity changed during the operation.")
		props["action_verification"] = stringProp("Provider verification status for native actions.")
		props["provider_result"] = objectProp("Structured result returned by the selected provider.")
	default:
		return nil, false
	}
	return toolcontract.OutputObject(props), true
}
