package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/uvwt/agentdock/internal/execution"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	"github.com/uvwt/agentdock/internal/observability"
	"github.com/uvwt/agentdock/internal/permission"
	toolcomputer "github.com/uvwt/agentdock/internal/tool/computer"
)

func snapshotValidatedToolArguments(args map[string]any) (map[string]any, error) {
	if args == nil {
		return map[string]any{}, nil
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var snapshot map[string]any
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, err
	}
	if snapshot == nil {
		snapshot = map[string]any{}
	}
	return snapshot, nil
}

func (r *Runtime) admitRuntimeTool(
	ctx context.Context,
	call execution.Call,
	name string,
	args map[string]any,
	source observability.Source,
) (permission.Admission, error) {
	if r == nil || r.admission == nil || r.permissions == nil || r.execution == nil {
		return permission.Admission{}, toolError("PERMISSION_STATE_ERROR", "AgentDock permission admission is unavailable", "permission")
	}

	action := permissionAction(args)
	facts := r.runtimePermissionFacts(ctx, name, action, args, source)
	fingerprint, err := runtimePermissionFingerprint(name, action, args)
	if err != nil {
		return permission.Admission{}, toolError("PERMISSION_STATE_ERROR", "AgentDock could not prepare permission admission", "permission")
	}
	prepared := permission.PreparedRequest{
		Fingerprint:  fingerprint,
		Binding:      facts.Binding,
		Generations:  map[string]string{},
		Tool:         name,
		Action:       action,
		RuntimeEpoch: r.execution.Epoch(),
	}
	admission, err := r.admission.Admit(ctx, permission.AdmissionRequest{
		Audit: permission.AuditBinding{
			CallID:       call.ID,
			ParentCallID: call.ParentCallID,
		},
		Facts:    facts,
		Prepared: prepared,
		Summary:  permissionDisplaySummary(name, action),
		Scope:    "AgentDock Core request",
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return permission.Admission{}, err
		}
		slog.Warn("permission admission failed", "tool", name, "error", err)
		return permission.Admission{}, toolError("PERMISSION_STATE_ERROR", "AgentDock permission state could not be evaluated", "permission")
	}
	return admission, nil
}

func permissionAdmissionToolError(admission permission.Admission) error {
	switch admission.Decision.Effect {
	case permission.Allow:
		return nil
	case permission.Deny:
		return toolErrorDetails(
			"PERMISSION_DENIED",
			"AgentDock permission policy denied this operation",
			"permission",
			map[string]any{
				"executed":        false,
				"policy_revision": admission.Decision.PolicyRevision,
				"rule_id":         admission.Decision.RuleID,
				"reason":          admission.Decision.Reason,
			},
		)
	case permission.Ask:
		if admission.Approval == nil {
			return toolError("PERMISSION_STATE_ERROR", "AgentDock permission approval was not recorded", "permission")
		}
		return toolErrorDetails(
			"APPROVAL_REQUIRED",
			"AgentDock requires local approval before this operation can run",
			"permission",
			map[string]any{
				"approval_id":      admission.Approval.ID,
				"approval_version": admission.Approval.Version,
				"executed":         false,
				"policy_revision":  admission.Decision.PolicyRevision,
				"retry":            "Retry the same request after approval.",
			},
		)
	default:
		return toolError("PERMISSION_STATE_ERROR", "AgentDock permission policy returned an invalid decision", "permission")
	}
}

func (r *Runtime) settleConsumedPermission(admission permission.Admission, result Result, callErr error) {
	if r == nil || r.permissions == nil || admission.ConsumedApprovalID == "" {
		return
	}
	outcome := permission.Succeeded
	if callErr != nil {
		outcome = permission.Failed
	} else if failed, _, _ := executionResultFailure(result); failed {
		outcome = permission.Failed
	}
	if _, err := r.permissions.SettleDispatch(context.Background(), admission.ConsumedApprovalID, outcome); err != nil {
		// Dispatch may already have happened. Do not turn a successful/failed
		// tool result into a misleading retry signal; recovery will report an
		// unresolved consumed grant as unknown on the next Core start.
		slog.Error("settle consumed permission dispatch failed", "approval_id", admission.ConsumedApprovalID, "error", err)
	}
}

func (r *Runtime) runtimePermissionFacts(ctx context.Context, name, action string, args map[string]any, source observability.Source) permission.PermissionFacts {
	binding := permission.PermissionBinding{
		RuntimeEpoch: r.execution.Epoch(),
		Source:       string(source),
	}
	if principal, ok := requestmeta.AuthPrincipalFromContext(ctx); ok {
		binding.Principal = permission.AuthorizationPrincipal{
			Kind:          principal.Kind,
			ID:            principal.ID,
			Authenticated: principal.Authenticated,
			Stable:        principal.Stable,
		}
	}
	facts := permission.PermissionFacts{
		EffectsKnown: false,
		Filesystem:   permission.FileNone,
		Tool:         name,
		Action:       action,
		Reason:       "Core has no stronger effect classification for this operation",
		Binding:      binding,
	}

	if definition, ok := r.ToolDefinition(name); ok && definition.Annotations != nil {
		facts.EffectsKnown = true
		facts.ReadOnly = definition.Annotations.ReadOnlyHint
		if !facts.ReadOnly {
			facts.Other = true
			facts.OneShotEligible = true
		}
		if definition.Annotations.OpenWorldHint != nil && *definition.Annotations.OpenWorldHint {
			facts.Network = true
		}
	}

	switch name {
	case "agentdock_context", "workspace_context":
		facts.EffectsKnown = true
		facts.ReadOnly = true
		facts.Other = false
		facts.Network = false
		facts.Reason = "Core-owned context inspection is read-only"
	case "read_file", "list_dir", "search_text":
		facts.EffectsKnown = true
		facts.ReadOnly = true
		facts.Filesystem = permission.FileRead
		facts.Other = false
		facts.Network = false
		facts.Reason = "Core-owned file inspection is read-only"
	case "file_edit":
		facts.EffectsKnown = true
		facts.ReadOnly = false
		facts.Filesystem = permission.FileWrite
		facts.Other = false
		facts.Network = false
		facts.OneShotEligible = true
		facts.Reason = "Core-owned file edit can mutate the filesystem"
	case "session_observe":
		facts.EffectsKnown = true
		facts.ReadOnly = true
		facts.Commands = false
		facts.Other = false
		facts.Network = false
		facts.Reason = "command session observation is read-only"
	case "session_act":
		facts.EffectsKnown = true
		facts.ReadOnly = false
		facts.Commands = true
		facts.Other = false
		facts.Network = false
		facts.OneShotEligible = true
		facts.Reason = "command session input/control is side-effecting"
	case "exec_command":
		facts.EffectsKnown = false
		facts.ReadOnly = false
		facts.Commands = true
		facts.Other = false
		facts.OneShotEligible = true
		facts.Reason = "arbitrary command effects cannot be proven before execution"
	case "mcp_tool_call":
		facts.EffectsKnown = false
		facts.ReadOnly = false
		facts.Network = true
		facts.MCP = true
		facts.Other = false
		facts.OpaqueProviderExecution = true
		facts.OneShotEligible = true
		facts.Reason = "dynamic MCP tool effects are opaque to Core"
	case "mcp_tool_search", "mcp_tool_inspect":
		facts.EffectsKnown = true
		facts.ReadOnly = true
		facts.Network = true
		facts.MCP = true
		facts.Other = false
		facts.Reason = "dynamic MCP catalog inspection is read-only"
	case "mcp_manage":
		facts.EffectsKnown = true
		facts.ReadOnly = false
		facts.Network = true
		facts.MCP = true
		facts.Management = true
		facts.Other = false
		facts.OneShotEligible = true
		facts.Reason = "dynamic MCP management changes Core/provider state"
	case "acp_session":
		facts.EffectsKnown = false
		facts.ReadOnly = false
		facts.OpaqueProviderExecution = true
		facts.Other = true
		facts.OneShotEligible = true
		facts.Reason = "ACP session operation can have opaque provider effects"
	case "acp_prompt":
		switch action {
		case "events":
			facts.EffectsKnown = true
			facts.ReadOnly = true
			facts.Other = false
			facts.Reason = "ACP prompt event inspection is read-only"
		case "cancel":
			facts.EffectsKnown = true
			facts.ReadOnly = true
			facts.Other = false
			facts.Reason = "ACP prompt cancellation only reduces outstanding provider work"
		default:
			facts.EffectsKnown = false
			facts.ReadOnly = false
			facts.OpaqueProviderExecution = true
			facts.Other = true
			facts.OneShotEligible = true
			facts.Reason = "ACP prompt continuation can have opaque provider effects"
		}
	case "acp_interaction":
		switch action {
		case "list":
			facts.EffectsKnown = true
			facts.ReadOnly = true
			facts.Other = false
			facts.Reason = "ACP interaction inspection is read-only"
		case "respond":
			// The outer Runtime.Call only reaches the Manager boundary. Cancellation
			// is intrinsically safe, while every provider option selection is
			// separately admitted immediately before the response resumes the
			// provider. Treating this wrapper as opaque would require two approvals
			// for the same continuation and consume the one-shot grant too early.
			facts.EffectsKnown = true
			facts.ReadOnly = true
			facts.Other = false
			facts.Reason = "ACP interaction response is a Core-controlled wrapper around separately admitted provider continuation"
		default:
			facts.EffectsKnown = false
			facts.ReadOnly = false
			facts.OpaqueProviderExecution = true
			facts.Other = true
			facts.OneShotEligible = true
			facts.Reason = "ACP interaction operation is not classified"
		}
	}

	return facts
}

func permissionAction(args map[string]any) string {
	if args == nil {
		return ""
	}
	value, _ := args["action"].(string)
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return ""
	}
	return value
}

func permissionDisplaySummary(tool, action string) string {
	if action == "" {
		return "Run " + tool
	}
	return "Run " + tool + " action " + action
}

func runtimePermissionFingerprint(tool, action string, args map[string]any) (string, error) {
	if args == nil {
		args = map[string]any{}
	}
	canonical, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("agentdock-permission-request-v1\x00"))
	_, _ = hash.Write([]byte(tool))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(action))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(canonical)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func (r *Runtime) admitHostOperation(ctx context.Context, op permission.HostOperation) (permission.HostOperationFinish, error) {
	if r == nil || r.admission == nil || r.permissions == nil || r.execution == nil {
		return nil, toolError("PERMISSION_STATE_ERROR", "AgentDock permission admission is unavailable", "permission")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tool := strings.TrimSpace(op.Tool)
	action := strings.TrimSpace(op.Action)
	source := strings.TrimSpace(op.Source)
	if source == "" {
		source = "acp_bridge"
	}
	if tool == "" || action == "" {
		return nil, toolError("PERMISSION_STATE_ERROR", "host capability operation identity is incomplete", "permission")
	}

	parent := execution.ScopeFromContext(ctx).CallID
	call := r.execution.BeginChild(parent, execution.BeginInput{
		Tool: tool, Source: source, InsertionSupported: false,
	})
	finishFailed := func(callErr error) {
		code, category := observableError(callErr)
		r.execution.Finish(call.ID, execution.FinishInput{
			Status: execution.StatusFailed, ErrorCode: code, ErrorCategory: category,
		})
	}

	facts := r.hostOperationFacts(ctx, op)
	fingerprint := strings.TrimSpace(op.PreparedFingerprint)
	if fingerprint == "" {
		var err error
		fingerprint, err = hostOperationFingerprint(tool, action, op.Payload)
		if err != nil {
			callErr := toolError("PERMISSION_STATE_ERROR", "AgentDock could not prepare host capability admission", "permission")
			finishFailed(callErr)
			return nil, callErr
		}
	}
	generations := map[string]string{}
	if source == "acp_bridge" {
		generations["acp_session"] = strings.TrimSpace(op.SessionID)
		generations["acp_profile"] = strings.TrimSpace(op.ProfileID)
	}
	prepared := permission.PreparedRequest{
		Fingerprint: fingerprint,
		Binding:     facts.Binding,
		Generations: generations,
		Tool:        tool, Action: action, RuntimeEpoch: r.execution.Epoch(),
	}
	admission, err := r.admission.Admit(ctx, permission.AdmissionRequest{
		Audit: permission.AuditBinding{CallID: call.ID, ParentCallID: call.ParentCallID},
		Facts: facts, Prepared: prepared,
		Summary: permissionDisplaySummary(tool, action),
		Scope:   hostOperationScope(source, tool),
	})
	if err != nil {
		callErr := err
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			slog.Warn("host capability permission admission failed", "tool", tool, "action", action, "error", err)
			callErr = toolError("PERMISSION_STATE_ERROR", "AgentDock permission state could not be evaluated", "permission")
		}
		finishFailed(callErr)
		return nil, callErr
	}
	if callErr := permissionAdmissionToolError(admission); callErr != nil {
		finishFailed(callErr)
		return nil, callErr
	}

	var once sync.Once
	finish := func(dispatchErr error) {
		once.Do(func() {
			status := execution.StatusCompleted
			code, category := "", ""
			if dispatchErr != nil {
				status = execution.StatusFailed
				code, category = observableError(dispatchErr)
			}
			r.execution.Finish(call.ID, execution.FinishInput{
				Status: status, ErrorCode: code, ErrorCategory: category,
			})
			if admission.ConsumedApprovalID != "" {
				outcome := permission.Succeeded
				if dispatchErr != nil {
					outcome = permission.Failed
				}
				if _, settleErr := r.permissions.SettleDispatch(context.Background(), admission.ConsumedApprovalID, outcome); settleErr != nil {
					slog.Error("settle host capability permission dispatch failed", "approval_id", admission.ConsumedApprovalID, "error", settleErr)
				}
			}
		})
	}
	return finish, nil
}

func (r *Runtime) runRuntimeManagementMutation(
	ctx context.Context,
	tool string,
	action string,
	payload any,
	dispatch func() (Result, error),
) (Result, error) {
	return r.runRuntimeManagementMutationBound(ctx, tool, action, payload, "", dispatch)
}

func (r *Runtime) runRuntimeManagementMutationBound(
	ctx context.Context,
	tool string,
	action string,
	payload any,
	preparedFingerprint string,
	dispatch func() (Result, error),
) (Result, error) {
	if dispatch == nil {
		return nil, toolError("PERMISSION_STATE_ERROR", "runtime management dispatch is unavailable", "permission")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	finish, err := r.admitHostOperation(ctx, permission.HostOperation{
		Tool:                tool,
		Action:              action,
		Source:              "internal",
		Payload:             payload,
		PreparedFingerprint: strings.TrimSpace(preparedFingerprint),
	})
	if err != nil {
		return nil, err
	}
	result, dispatchErr := dispatch()
	finish(dispatchErr)
	return result, dispatchErr
}

func (r *Runtime) hostOperationFacts(ctx context.Context, op permission.HostOperation) permission.PermissionFacts {
	source := strings.TrimSpace(op.Source)
	if source == "" {
		source = "acp_bridge"
	}
	binding := permission.PermissionBinding{
		RuntimeEpoch: r.execution.Epoch(),
		Source:       source,
		ACPSessionID: strings.TrimSpace(op.SessionID),
		Provider:     strings.TrimSpace(op.ProfileID),
	}
	if principal, ok := requestmeta.AuthPrincipalFromContext(ctx); ok {
		binding.Principal = permission.AuthorizationPrincipal{
			Kind: principal.Kind, ID: principal.ID,
			Authenticated: principal.Authenticated, Stable: principal.Stable,
		}
	}
	if root := strings.TrimSpace(op.WorkspaceRoot); root != "" {
		binding.WorkspaceRoot = root
		binding.WorkspaceID = trustedWorkspacePermissionID(root)
		binding.TrustedWorkspace = binding.WorkspaceID != ""
	}

	facts := permission.PermissionFacts{
		EffectsKnown: true,
		Filesystem:   permission.FileNone,
		Tool:         strings.TrimSpace(op.Tool),
		Action:       strings.TrimSpace(op.Action),
		Binding:      binding,
		Reason:       "Core classified ACP host capability operation",
	}
	switch facts.Tool {
	case "acp_browser":
		switch facts.Action {
		case "take_snapshot", "take_screenshot":
			facts.ReadOnly = true
			facts.Reason = "browser snapshot/screenshot observes the owned target"
		case "acquire":
			facts.Management = true
			facts.Other = true
			facts.OneShotEligible = true
			facts.Network = hostPayloadHasNonBlankString(op.Payload, "url")
			facts.Reason = "browser acquire creates a lease and may start or attach browser resources"
		case "navigate_page", "click", "fill", "press_key":
			facts.Network = true
			facts.Other = true
			facts.OneShotEligible = true
			facts.Reason = "browser action can mutate page state or trigger network activity"
		case "evaluate_script":
			facts.EffectsKnown = false
			facts.OpaqueProviderExecution = true
			facts.Network = true
			facts.Other = true
			facts.OneShotEligible = true
			facts.Reason = "browser JavaScript effects cannot be proven before execution"
		default:
			facts.EffectsKnown = false
			facts.Other = true
			facts.OneShotEligible = true
			facts.Reason = "browser host operation is not classified"
		}
	case "acp_computer":
		switch facts.Action {
		case "acquire":
			facts.Management = true
			facts.Other = true
			facts.OneShotEligible = true
			facts.Reason = "computer acquire creates a host control session"
		case "observe":
			if computerObservationIsCoreReadOnly(op.Payload) {
				facts.ReadOnly = true
				facts.Reason = "computer observation is Core-proven non-foreground inspection"
			} else {
				facts.Other = true
				facts.OneShotEligible = true
				facts.Reason = "computer observation may require foreground or permission interaction"
			}
		case "act":
			facts.Other = true
			facts.OneShotEligible = true
			facts.Reason = "computer action can mutate native GUI state"
		default:
			facts.EffectsKnown = false
			facts.Other = true
			facts.OneShotEligible = true
			facts.Reason = "computer host operation is not classified"
		}
	case "runtime_insertion":
		if facts.Action == "cancel" {
			facts.ReadOnly = true
			facts.Reason = "cancelling a pending insertion reduces outstanding work"
		} else {
			facts.Management = true
			facts.Other = true
			facts.OneShotEligible = true
			facts.Reason = "runtime insertion changes an active execution"
		}
	case "runtime_task":
		facts.Management = true
		facts.Other = true
		facts.OneShotEligible = true
		facts.Reason = "runtime task deletion mutates durable task state"
	case "runtime_plugin":
		facts.Management = true
		facts.MCP = true
		switch facts.Action {
		case "desktop_update_candidate", "desktop_set_enabled", "desktop_remove_keep", "desktop_remove_purge":
			// Plugin lifecycle can stop/start local-process MCP servers and
			// remote MCP connections. Conservatively classify both effects.
			facts.Network = true
			facts.Commands = true
		}
		facts.Other = true
		facts.OneShotEligible = true
		facts.Reason = "Desktop Plugin management changes Plugin state or credentials"
	case "runtime_mcp":
		facts.Management = true
		facts.MCP = true
		facts.Network = true
		facts.OneShotEligible = true
		facts.Reason = "runtime MCP management changes provider configuration or credentials"
	case "runtime_evolve":
		facts.Management = true
		facts.Other = true
		facts.OneShotEligible = true
		facts.Reason = "runtime evolution proposal mutates durable evolution state"
	case "acp_provider_continuation":
		facts.EffectsKnown = false
		facts.ReadOnly = false
		facts.OpaqueProviderExecution = true
		facts.Other = true
		facts.OneShotEligible = true
		facts.Reason = "ACP permission option selection resumes opaque provider execution"
	default:
		facts.EffectsKnown = false
		facts.Other = true
		facts.OneShotEligible = true
		facts.Reason = "host capability operation is not classified"
	}
	return facts
}

func hostOperationFingerprint(tool, action string, payload any) (string, error) {
	snapshot, err := snapshotValidatedToolArguments(map[string]any{"payload": payload})
	if err != nil {
		return "", err
	}
	return runtimePermissionFingerprint(tool, action, snapshot)
}

func trustedWorkspacePermissionID(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("agentdock-permission-workspace-v1"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(root))
	return "workspace:sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func hostPayloadHasNonBlankString(payload any, key string) bool {
	container, ok := payload.(map[string]any)
	if !ok {
		return false
	}
	value, _ := container[key].(string)
	return strings.TrimSpace(value) != ""
}

func computerObservationIsCoreReadOnly(payload any) bool {
	container, ok := payload.(map[string]any)
	if !ok {
		return false
	}
	request, ok := container["request"].(toolcomputer.ObservationRequest)
	if !ok || request.RestoreWindow || request.Action == "permissions" {
		return false
	}
	switch request.Action {
	case "capabilities", "list_apps", "list_windows", "get_app_state":
		return true
	default:
		return false
	}
}

func hostOperationScope(source, tool string) string {
	if strings.TrimSpace(tool) == "acp_provider_continuation" {
		return "AgentDock ACP provider continuation"
	}
	if strings.TrimSpace(source) == "acp_bridge" {
		return "AgentDock ACP host capability"
	}
	return "AgentDock Runtime management"
}
