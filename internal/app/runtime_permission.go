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

	"github.com/uvwt/agentdock/internal/execution"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	"github.com/uvwt/agentdock/internal/observability"
	"github.com/uvwt/agentdock/internal/permission"
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
	case "acp_session", "acp_prompt", "acp_interaction":
		facts.EffectsKnown = false
		facts.ReadOnly = false
		facts.OpaqueProviderExecution = true
		facts.Other = true
		facts.OneShotEligible = true
		facts.Reason = "ACP provider continuation can have opaque downstream effects"
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
