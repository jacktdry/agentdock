package runtimeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/uvwt/agentdock/internal/app"
	"github.com/uvwt/agentdock/internal/permission"
	toolcore "github.com/uvwt/agentdock/internal/tool/core"
)

const MaxPermissionRequestBytes = 64 << 10

// PermissionControlRequest never contains credentials or a caller-supplied actor.
type PermissionControlRequest struct {
	permission.ControlMutationRequest
	ConfirmationID string `json:"confirmation_id,omitempty"`
}

func DesktopPermissionControlRoute(path string) bool {
	switch path {
	case "/internal/desktop-control/permission-confirmations", "/internal/desktop-control/permissions", "/internal/desktop-control/approvals":
		return true
	default:
		return false
	}
}

// DispatchDesktopPermissionControl is separate from normal Runtime/Nexus dispatch.
// Its caller must enforce direct-loopback transport before passing the credential.
func DispatchDesktopPermissionControl(ctx context.Context, runtime DesktopPermissionControlRuntime, credential string, request Request) (map[string]any, error) {
	if runtime == nil || !runtime.AuthenticateDesktopPermissionControl(credential) {
		return nil, PermissionControlError(permission.ErrControlUnauthorized)
	}
	if !DesktopPermissionControlRoute(request.Path) || request.Method != http.MethodPost {
		return nil, &app.ToolError{Code: "NOT_FOUND", Message: "desktop permission control route not found", Category: "not_found"}
	}
	var input PermissionControlRequest
	if err := DecodePermissionControlRequest(request.Body, &input); err != nil {
		return nil, err
	}
	confirmation := request.Path == "/internal/desktop-control/permission-confirmations"
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	if confirmation {
		if input.ConfirmationID != "" {
			return nil, PermissionControlError(permission.ErrControlMutation)
		}
		challenge, err := runtime.BeginPermissionConfirmation(credential, input.ControlMutationRequest)
		if err != nil {
			return nil, PermissionControlError(err)
		}
		return map[string]any{"ok": true, "schema_version": permission.SchemaVersion, "confirmation": challenge}, nil
	}
	if (request.Path == "/internal/desktop-control/permissions" && kind != permission.ControlMutationUpdatePolicy) ||
		(request.Path == "/internal/desktop-control/approvals" && kind != permission.ControlMutationApproveOnce && kind != permission.ControlMutationApproveWorkspace && kind != permission.ControlMutationReject) {
		return nil, PermissionControlError(permission.ErrControlMutation)
	}
	result, err := runtime.ApplyPermissionControlMutation(ctx, credential, input.ConfirmationID, input.ControlMutationRequest)
	if err != nil {
		return nil, PermissionControlError(err)
	}
	result["ok"], result["schema_version"] = true, permission.SchemaVersion
	return map[string]any(result), nil
}

func DecodePermissionControlRequest(body []byte, target *PermissionControlRequest) error {
	if len(body) > MaxPermissionRequestBytes || !bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) {
		return PermissionControlError(permission.ErrControlMutation)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return PermissionControlError(permission.ErrControlMutation)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return PermissionControlError(permission.ErrControlMutation)
	}
	return nil
}

// PermissionControlError returns bounded errors without leaking state paths,
// policy contents, credentials or raw storage errors.
func PermissionControlError(err error) error {
	if err == nil {
		return nil
	}
	code, message, category := "PERMISSION_STATE_ERROR", "AgentDock permission state operation failed", "internal"
	switch {
	case errors.Is(err, permission.ErrControlUnauthorized):
		code, message, category = "DESKTOP_CONTROL_UNAUTHORIZED", "Desktop control authentication is required", "authentication"
	case errors.Is(err, permission.ErrControlMutation), errors.Is(err, permission.ErrInvalidPolicy):
		code, message, category = "INVALID_PERMISSION_MUTATION", "invalid permission mutation payload", "validation"
	case errors.Is(err, permission.ErrRevision):
		code, message, category = "POLICY_REVISION_CONFLICT", "permission policy changed; refresh before confirming again", "conflict"
	case errors.Is(err, permission.ErrVersion):
		code, message, category = "APPROVAL_VERSION_CONFLICT", "approval changed; refresh before confirming again", "conflict"
	case errors.Is(err, permission.ErrNotFound):
		code, message, category = "APPROVAL_NOT_FOUND", "permission approval was not found", "not_found"
	case errors.Is(err, permission.ErrConfirmationNotFound):
		code, message, category = "CONFIRMATION_NOT_FOUND", "confirmation is missing or already consumed; confirm again", "conflict"
	case errors.Is(err, permission.ErrConfirmationExpired):
		code, message, category = "CONFIRMATION_EXPIRED", "confirmation expired; confirm again", "conflict"
	case errors.Is(err, permission.ErrConfirmationMismatch):
		code, message, category = "CONFIRMATION_MISMATCH", "confirmation does not match this mutation; confirm again", "conflict"
	case errors.Is(err, permission.ErrExpired):
		code, message, category = "APPROVAL_EXPIRED", "permission approval expired", "conflict"
	case errors.Is(err, permission.ErrNotEligible):
		code, message, category = "APPROVAL_NOT_ELIGIBLE", "permission approval is not eligible for this decision", "permission"
	case errors.Is(err, permission.ErrConfirmationLimit), errors.Is(err, permission.ErrLimit):
		code, message, category = "PERMISSION_LIMIT_REACHED", "permission control capacity reached", "capacity"
	}
	return toolcore.NewErrorCause(code, message, category, nil, err)
}
