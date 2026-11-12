package desktopapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/uvwt/agentdock/internal/app"
	"github.com/uvwt/agentdock/internal/desktopcontrol"
	"github.com/uvwt/agentdock/internal/desktopruntime"
	"github.com/uvwt/agentdock/internal/permission"
	"github.com/uvwt/agentdock/internal/runtimeapi"
)

// PermissionResult projects Core state only; approving never runs the original
// operation. The caller must retry it through normal Core admission.
type PermissionResult struct {
	OK            bool                              `json:"ok"`
	SchemaVersion int                               `json:"schema_version"`
	StateRevision uint64                            `json:"state_revision,omitempty"`
	RuntimeEpoch  string                            `json:"runtime_epoch,omitempty"`
	Policy        *permission.Policy                `json:"policy,omitempty"`
	Approval      *permission.ApprovalRecord        `json:"approval,omitempty"`
	Approvals     []permission.ApprovalRecord       `json:"approvals,omitempty"`
	Count         int                               `json:"count,omitempty"`
	Confirmation  *permission.ConfirmationChallenge `json:"confirmation,omitempty"`
	Error         *APIError                         `json:"error,omitempty"`
}

type PermissionService struct {
	runtimeRoot    string
	rootError      error
	client         *http.Client
	readConnection func(string) (desktopruntime.CoreConnection, error)
	bootstrap      func(context.Context, string) (string, error)
}

func NewPermissionService(root string) *PermissionService {
	root, err := resolveRuntimeRoot(root)
	return &PermissionService{runtimeRoot: root, rootError: err,
		client: &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil, ResponseHeaderTimeout: 5 * time.Second},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		readConnection: desktopruntime.ReadCoreConnection, bootstrap: bootstrapPermissionCredential}
}

func bootstrapPermissionCredential(ctx context.Context, root string) (string, error) {
	var response struct {
		Credential string `json:"credential"`
	}
	err := desktopcontrol.Call(ctx, root, "permission.bootstrap", nil, &response)
	return response.Credential, err
}

func (s *PermissionService) Status(ctx context.Context) PermissionResult {
	return s.request(ctx, http.MethodGet, "/internal/runtime/permissions", nil)
}
func (s *PermissionService) History(ctx context.Context, status string, limit int) PermissionResult {
	query := url.Values{"status": {status}, "limit": {strconv.Itoa(limit)}}
	return s.request(ctx, http.MethodGet, "/internal/runtime/approvals?"+query.Encode(), nil)
}
func (s *PermissionService) Approval(ctx context.Context, id string) PermissionResult {
	if id == "" {
		return permissionServiceFailure("INVALID_PERMISSION_MUTATION", ErrorCategoryValidation)
	}
	return s.request(ctx, http.MethodGet, "/internal/runtime/approvals?"+url.Values{"approval_id": {id}}.Encode(), nil)
}
func (s *PermissionService) BeginConfirmation(ctx context.Context, mutation permission.ControlMutationRequest) PermissionResult {
	return s.request(ctx, http.MethodPost, "/internal/desktop-control/permission-confirmations", runtimeapi.PermissionControlRequest{ControlMutationRequest: mutation})
}
func (s *PermissionService) UpdatePolicy(ctx context.Context, confirmationID string, revision uint64, policy permission.Policy) PermissionResult {
	return s.mutate(ctx, confirmationID, permission.ControlMutationRequest{Kind: permission.ControlMutationUpdatePolicy, PolicyRevision: revision, Policy: &policy})
}
func (s *PermissionService) ApproveOnce(ctx context.Context, confirmationID, approvalID string, version, revision uint64) PermissionResult {
	return s.decide(ctx, permission.ControlMutationApproveOnce, confirmationID, approvalID, version, revision)
}
func (s *PermissionService) ApproveWorkspace(ctx context.Context, confirmationID, approvalID string, version, revision uint64) PermissionResult {
	return s.decide(ctx, permission.ControlMutationApproveWorkspace, confirmationID, approvalID, version, revision)
}
func (s *PermissionService) Reject(ctx context.Context, confirmationID, approvalID string, version, revision uint64) PermissionResult {
	return s.decide(ctx, permission.ControlMutationReject, confirmationID, approvalID, version, revision)
}
func (s *PermissionService) decide(ctx context.Context, kind, confirmationID, approvalID string, version, revision uint64) PermissionResult {
	return s.mutate(ctx, confirmationID, permission.ControlMutationRequest{Kind: kind, ApprovalID: approvalID, ApprovalVersion: version, PolicyRevision: revision})
}
func (s *PermissionService) mutate(ctx context.Context, confirmationID string, mutation permission.ControlMutationRequest) PermissionResult {
	path := "/internal/desktop-control/approvals"
	if mutation.Kind == permission.ControlMutationUpdatePolicy {
		path = "/internal/desktop-control/permissions"
	}
	return s.request(ctx, http.MethodPost, path, runtimeapi.PermissionControlRequest{ControlMutationRequest: mutation, ConfirmationID: confirmationID})
}
func (s *PermissionService) request(ctx context.Context, method, path string, payload any) PermissionResult {
	if s.rootError != nil {
		return permissionServiceFailure("PERMISSION_CORE_UNAVAILABLE", ErrorCategoryUnavailable)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	connection, err := s.readConnection(s.runtimeRoot)
	if err != nil {
		return permissionServiceFailure("PERMISSION_CORE_UNAVAILABLE", ErrorCategoryUnavailable)
	}
	endpoint, err := url.Parse(connection.Endpoint())
	if err != nil || endpoint.Scheme != "http" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") {
		return permissionServiceFailure("PERMISSION_CORE_UNAVAILABLE", ErrorCategoryUnavailable)
	}
	ip := net.ParseIP(endpoint.Hostname())
	if endpoint.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return permissionServiceFailure("PERMISSION_CORE_UNAVAILABLE", ErrorCategoryUnavailable)
	}
	token := connection.AuthToken()
	var encoded []byte
	if method == http.MethodPost {
		encoded, err = json.Marshal(payload)
		if err != nil || len(encoded) > runtimeapi.MaxPermissionRequestBytes {
			return permissionServiceFailure("INVALID_PERMISSION_MUTATION", ErrorCategoryValidation)
		}
		// Only native IPC supplies the distinct secret. No normal bearer fallback,
		// automatic challenge issuance, retry or approval occurs here.
		token, err = s.bootstrap(ctx, s.runtimeRoot)
		if err != nil || token == "" {
			return permissionServiceFailure("DESKTOP_CONTROL_UNAUTHORIZED", ErrorCategoryUnavailable)
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, connection.Endpoint()+path, bytes.NewReader(encoded))
	if err != nil {
		return permissionServiceFailure("PERMISSION_CORE_UNAVAILABLE", ErrorCategoryUnavailable)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return permissionServiceFailure("PERMISSION_CORE_UNAVAILABLE", ErrorCategoryUnavailable)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, permission.MaxStateBytes+1))
	if err != nil || len(data) > permission.MaxStateBytes {
		return permissionServiceFailure("PERMISSION_RESPONSE_INVALID", ErrorCategoryInternal)
	}
	if response.StatusCode != http.StatusOK {
		var envelope struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(data, &envelope)
		// Preserve only known Core error codes; discard remote strings and details.
		for _, cause := range []error{permission.ErrControlUnauthorized, permission.ErrControlMutation, permission.ErrInvalidPolicy, permission.ErrRevision, permission.ErrVersion, permission.ErrNotFound, permission.ErrConfirmationNotFound, permission.ErrConfirmationExpired, permission.ErrConfirmationMismatch, permission.ErrExpired, permission.ErrNotEligible, permission.ErrLimit} {
			mapped := runtimeapi.PermissionControlError(cause).(*app.ToolError)
			if envelope.Code == mapped.Code {
				return PermissionResult{Error: NewError(mapped.Code, mapped.Message, ErrorCategory(mapped.Category), false, nil)}
			}
		}
		return permissionServiceFailure("PERMISSION_REQUEST_FAILED", ErrorCategoryOperation)
	}
	var result PermissionResult
	if json.Unmarshal(data, &result) != nil || !result.OK || result.SchemaVersion != permission.SchemaVersion {
		return permissionServiceFailure("PERMISSION_RESPONSE_INVALID", ErrorCategoryInternal)
	}
	return result
}
func permissionServiceFailure(code string, category ErrorCategory) PermissionResult {
	return PermissionResult{Error: NewError(code, "Permission control request could not be completed", category, false, nil)}
}
