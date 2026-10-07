package desktopapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

const (
	mcpDesktopPath             = "/internal/runtime/mcp/desktop"
	maxMCPDesktopRequestBytes  = 64 << 10
	maxMCPDesktopResponseBytes = 4 << 20
	mcpDesktopRequestTimeout   = 310 * time.Second
)

type MCPObservation struct {
	Connection    string `json:"connection"`
	Status        string `json:"status"`
	AuthStatus    string `json:"authStatus"`
	ToolCount     *int   `json:"toolCount,omitempty"`
	ObservedAt    string `json:"observedAt,omitempty"`
	LastErrorCode string `json:"lastErrorCode,omitempty"`
	Stale         bool   `json:"stale"`
}

type MCPManagedServer struct {
	Name            string            `json:"name"`
	DisplayName     string            `json:"displayName"`
	Description     string            `json:"description"`
	SourceType      string            `json:"sourceType"`
	PluginName      string            `json:"pluginName,omitempty"`
	Transport       string            `json:"transport"`
	ProtocolVersion string            `json:"protocolVersion,omitempty"`
	URL             string            `json:"url,omitempty"`
	URLProtected    bool              `json:"urlProtected"`
	Command         string            `json:"command,omitempty"`
	CWD             string            `json:"cwd,omitempty"`
	Args            []string          `json:"args,omitempty"`
	ArgsProtected   bool              `json:"argsProtected"`
	Enabled         bool              `json:"enabled"`
	TimeoutMS       int               `json:"timeoutMs"`
	HeaderEnv       map[string]string `json:"headerEnv,omitempty"`
	EnvFromEnv      map[string]string `json:"envFromEnv,omitempty"`
	Generation      string            `json:"generation"`
	Observation     MCPObservation    `json:"observation"`
}

type MCPSnapshot struct {
	RegistryRevision string             `json:"registryRevision"`
	Authoritative    bool               `json:"authoritative"`
	Servers          []MCPManagedServer `json:"servers"`
}

type MCPSnapshotResult struct {
	Snapshot MCPSnapshot `json:"snapshot"`
	Error    *APIError   `json:"error,omitempty"`
}

type MCPServerResult struct {
	RegistryRevision string            `json:"registryRevision,omitempty"`
	Server           *MCPManagedServer `json:"server,omitempty"`
	Error            *APIError         `json:"error,omitempty"`
}

type MCPConfigInput struct {
	Name            string            `json:"name,omitempty"`
	Description     string            `json:"description,omitempty"`
	Transport       string            `json:"transport,omitempty"`
	ProtocolVersion string            `json:"protocolVersion,omitempty"`
	URL             string            `json:"url,omitempty"`
	Command         string            `json:"command,omitempty"`
	Args            []string          `json:"args,omitempty"`
	CWD             string            `json:"cwd,omitempty"`
	HeaderEnv       map[string]string `json:"headerEnv,omitempty"`
	EnvFromEnv      map[string]string `json:"envFromEnv,omitempty"`
	Enabled         *bool             `json:"enabled,omitempty"`
	TimeoutMS       *int              `json:"timeoutMs,omitempty"`
}

type MCPMutationResult struct {
	Completed        bool              `json:"completed"`
	Persisted        bool              `json:"persisted"`
	RuntimeApplied   bool              `json:"runtimeApplied"`
	RecoveryRequired bool              `json:"recoveryRequired"`
	RegistryRevision string            `json:"registryRevision,omitempty"`
	Server           *MCPManagedServer `json:"server,omitempty"`
	Error            *APIError         `json:"error,omitempty"`
}

type MCPEnvironmentEntry struct {
	Key        string `json:"key"`
	Configured bool   `json:"configured"`
}

type MCPEnvironmentResult struct {
	Revision string                `json:"revision,omitempty"`
	Items    []MCPEnvironmentEntry `json:"items"`
	Error    *APIError             `json:"error,omitempty"`
}

type MCPToolSummary struct {
	Name          string `json:"name"`
	QualifiedName string `json:"qualifiedName"`
	Server        string `json:"server"`
	SourceType    string `json:"sourceType"`
	PluginName    string `json:"pluginName,omitempty"`
}

type MCPReconnectResult struct {
	Server    *MCPManagedServer `json:"server,omitempty"`
	Tools     []MCPToolSummary  `json:"tools"`
	ToolCount int               `json:"toolCount"`
	Error     *APIError         `json:"error,omitempty"`
}

type MCPAuthCallback struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type MCPAuthorizationStatus struct {
	Status          string            `json:"status"`
	CallbackOptions []MCPAuthCallback `json:"callbackOptions"`
}

type MCPAuthorizationStatusResult struct {
	Authorization MCPAuthorizationStatus `json:"authorization"`
	Error         *APIError              `json:"error,omitempty"`
}

type MCPAuthorizationResult struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty"`
	CallbackID       string            `json:"callbackId,omitempty"`
	ExpiresAt        string            `json:"expiresAt,omitempty"`
	CallbackOptions  []MCPAuthCallback `json:"callbackOptions"`
	Error            *APIError         `json:"error,omitempty"`
}

type MCPActionResult struct {
	Completed bool      `json:"completed"`
	Error     *APIError `json:"error,omitempty"`
}

type mcpCoreAccess struct {
	Endpoint string
	Token    string
}

type MCPService struct {
	runtimeRoot string
	rootError   error
	client      *http.Client
	readAccess  func(string) (mcpCoreAccess, error)
}

func NewMCPService(root string) *MCPService {
	root, err := resolveRuntimeRoot(root)
	transport := &http.Transport{Proxy: nil, ResponseHeaderTimeout: 8 * time.Second}
	return &MCPService{
		runtimeRoot: root,
		rootError:   err,
		client: &http.Client{
			Transport: transport,
			Timeout:   mcpDesktopRequestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		readAccess: func(root string) (mcpCoreAccess, error) {
			connection, err := desktopruntime.ReadCoreConnection(root)
			if err != nil {
				return mcpCoreAccess{}, err
			}
			return mcpCoreAccess{Endpoint: connection.Endpoint(), Token: connection.AuthToken()}, nil
		},
	}
}

func mcpOperations() []OperationCapability {
	return []OperationCapability{
		{Name: "snapshot", Access: AccessRead, Availability: AvailabilityAvailable},
		{Name: "inspect", Access: AccessRead, Availability: AvailabilityAvailable},
		{Name: "environment", Access: AccessRead, Availability: AvailabilityAvailable},
		{Name: "authorizationStatus", Access: AccessRead, Availability: AvailabilityAvailable},
		{Name: "create", Access: AccessMutating, RequiresConfirmation: true, Availability: AvailabilityAvailable},
		{Name: "update", Access: AccessMutating, RequiresConfirmation: true, Availability: AvailabilityAvailable},
		{Name: "remove", Access: AccessPrivileged, RequiresConfirmation: true, Availability: AvailabilityAvailable},
		{Name: "setEnabled", Access: AccessMutating, RequiresConfirmation: true, Availability: AvailabilityAvailable},
		{Name: "setEnvironment", Access: AccessPrivileged, RequiresConfirmation: true, Availability: AvailabilityAvailable},
		{Name: "unsetEnvironment", Access: AccessPrivileged, RequiresConfirmation: true, Availability: AvailabilityAvailable},
		{Name: "purgeEnvironment", Access: AccessPrivileged, RequiresConfirmation: true, Availability: AvailabilityAvailable},
		{Name: "reconnect", Access: AccessPrivileged, RequiresConfirmation: true, Availability: AvailabilityAvailable},
		{Name: "authorize", Access: AccessPrivileged, RequiresConfirmation: true, Availability: AvailabilityAvailable},
		{Name: "clearAuthorization", Access: AccessPrivileged, RequiresConfirmation: true, Availability: AvailabilityAvailable},
	}
}

func (s *MCPService) Snapshot(ctx context.Context) MCPSnapshotResult {
	var wire mcpWireResponse
	if apiErr := s.request(ctx, http.MethodGet, nil, &wire); apiErr != nil {
		return MCPSnapshotResult{Snapshot: MCPSnapshot{Servers: []MCPManagedServer{}}, Error: apiErr}
	}
	return MCPSnapshotResult{Snapshot: snapshotFromWire(wire)}
}

func (s *MCPService) Inspect(ctx context.Context, name string) MCPServerResult {
	var wire mcpWireResponse
	apiErr := s.request(ctx, http.MethodPost, mcpWireRequest{Action: "desktop_inspect", Name: strings.TrimSpace(name)}, &wire)
	if apiErr != nil {
		return MCPServerResult{Error: apiErr}
	}
	server := serverFromWire(wire.Server)
	return MCPServerResult{RegistryRevision: wire.RegistryRevision, Server: &server}
}

func (s *MCPService) Create(ctx context.Context, expectedRevision string, input MCPConfigInput) MCPMutationResult {
	request := mcpConfigRequest("desktop_create", input)
	request.ExpectedRegistryRevision = strings.TrimSpace(expectedRevision)
	return s.mutation(ctx, request)
}

func (s *MCPService) Update(ctx context.Context, name, expectedRevision, expectedGeneration string, input MCPConfigInput) MCPMutationResult {
	request := mcpConfigRequest("desktop_update", input)
	request.Name = strings.TrimSpace(name)
	request.ExpectedRegistryRevision = strings.TrimSpace(expectedRevision)
	request.ExpectedGeneration = strings.TrimSpace(expectedGeneration)
	return s.mutation(ctx, request)
}

func (s *MCPService) Remove(ctx context.Context, name, expectedRevision, expectedGeneration string) MCPMutationResult {
	return s.mutation(ctx, mcpWireRequest{Action: "desktop_remove", Name: strings.TrimSpace(name), ExpectedRegistryRevision: strings.TrimSpace(expectedRevision), ExpectedGeneration: strings.TrimSpace(expectedGeneration)})
}

func (s *MCPService) SetEnabled(ctx context.Context, name string, enabled bool, expectedRevision, expectedGeneration string) MCPMutationResult {
	return s.mutation(ctx, mcpWireRequest{Action: "desktop_set_enabled", Name: strings.TrimSpace(name), Enabled: &enabled, ExpectedRegistryRevision: strings.TrimSpace(expectedRevision), ExpectedGeneration: strings.TrimSpace(expectedGeneration)})
}

func (s *MCPService) Environment(ctx context.Context, name, expectedRevision, expectedGeneration string) MCPEnvironmentResult {
	return s.environment(ctx, mcpWireRequest{Action: "desktop_env_snapshot", Name: strings.TrimSpace(name), ExpectedRegistryRevision: strings.TrimSpace(expectedRevision), ExpectedGeneration: strings.TrimSpace(expectedGeneration)})
}

func (s *MCPService) SetEnvironment(ctx context.Context, name, key, value, expectedRevision, expectedGeneration, expectedEnvRevision string) MCPEnvironmentResult {
	return s.environment(ctx, mcpWireRequest{Action: "desktop_env_set", Name: strings.TrimSpace(name), Key: strings.TrimSpace(key), Value: &value, ExpectedRegistryRevision: strings.TrimSpace(expectedRevision), ExpectedGeneration: strings.TrimSpace(expectedGeneration), ExpectedEnvRevision: strings.TrimSpace(expectedEnvRevision)})
}

func (s *MCPService) UnsetEnvironment(ctx context.Context, name, key, expectedRevision, expectedGeneration, expectedEnvRevision string) MCPEnvironmentResult {
	return s.environment(ctx, mcpWireRequest{Action: "desktop_env_unset", Name: strings.TrimSpace(name), Key: strings.TrimSpace(key), ExpectedRegistryRevision: strings.TrimSpace(expectedRevision), ExpectedGeneration: strings.TrimSpace(expectedGeneration), ExpectedEnvRevision: strings.TrimSpace(expectedEnvRevision)})
}

func (s *MCPService) PurgeEnvironment(ctx context.Context, name, expectedRevision, expectedGeneration, expectedEnvRevision string) MCPEnvironmentResult {
	return s.environment(ctx, mcpWireRequest{Action: "desktop_env_purge", Name: strings.TrimSpace(name), ExpectedRegistryRevision: strings.TrimSpace(expectedRevision), ExpectedGeneration: strings.TrimSpace(expectedGeneration), ExpectedEnvRevision: strings.TrimSpace(expectedEnvRevision)})
}

func (s *MCPService) Reconnect(ctx context.Context, name, expectedRevision, expectedGeneration string) MCPReconnectResult {
	var wire mcpWireResponse
	apiErr := s.request(ctx, http.MethodPost, mcpWireRequest{Action: "desktop_reconnect", Name: strings.TrimSpace(name), ExpectedRegistryRevision: strings.TrimSpace(expectedRevision), ExpectedGeneration: strings.TrimSpace(expectedGeneration)}, &wire)
	if apiErr != nil {
		return MCPReconnectResult{Tools: []MCPToolSummary{}, Error: apiErr}
	}
	server := serverFromWire(wire.Server)
	tools := make([]MCPToolSummary, 0, len(wire.Tools))
	for _, item := range wire.Tools {
		tools = append(tools, MCPToolSummary{Name: item.Name, QualifiedName: item.QualifiedName, Server: item.Server, SourceType: item.SourceType, PluginName: item.PluginName})
	}
	return MCPReconnectResult{Server: &server, Tools: tools, ToolCount: wire.ToolCount}
}

func (s *MCPService) AuthorizationStatus(ctx context.Context, name string) MCPAuthorizationStatusResult {
	var wire mcpWireResponse
	apiErr := s.request(ctx, http.MethodPost, mcpWireRequest{Action: "desktop_auth_status", Name: strings.TrimSpace(name)}, &wire)
	if apiErr != nil {
		return MCPAuthorizationStatusResult{Authorization: MCPAuthorizationStatus{Status: "unknown", CallbackOptions: []MCPAuthCallback{}}, Error: apiErr}
	}
	return MCPAuthorizationStatusResult{Authorization: MCPAuthorizationStatus{Status: wire.Status, CallbackOptions: callbacksFromWire(wire.CallbackOptions)}}
}

func (s *MCPService) Authorize(ctx context.Context, name, callbackID, expectedRevision, expectedGeneration string) MCPAuthorizationResult {
	var wire mcpWireResponse
	apiErr := s.request(ctx, http.MethodPost, mcpWireRequest{Action: "desktop_authorize", Name: strings.TrimSpace(name), CallbackID: strings.TrimSpace(callbackID), ExpectedRegistryRevision: strings.TrimSpace(expectedRevision), ExpectedGeneration: strings.TrimSpace(expectedGeneration)}, &wire)
	if apiErr != nil {
		return MCPAuthorizationResult{CallbackOptions: []MCPAuthCallback{}, Error: apiErr}
	}
	return MCPAuthorizationResult{AuthorizationURL: wire.AuthorizationURL, CallbackID: wire.CallbackID, ExpiresAt: wire.ExpiresAt, CallbackOptions: callbacksFromWire(wire.CallbackOptions)}
}

func (s *MCPService) ClearAuthorization(ctx context.Context, name, expectedRevision, expectedGeneration string) MCPActionResult {
	var wire mcpWireResponse
	apiErr := s.request(ctx, http.MethodPost, mcpWireRequest{Action: "desktop_auth_clear", Name: strings.TrimSpace(name), ExpectedRegistryRevision: strings.TrimSpace(expectedRevision), ExpectedGeneration: strings.TrimSpace(expectedGeneration)}, &wire)
	return MCPActionResult{Completed: apiErr == nil && wire.Removed, Error: apiErr}
}

func (s *MCPService) mutation(ctx context.Context, request mcpWireRequest) MCPMutationResult {
	var wire mcpWireResponse
	apiErr := s.request(ctx, http.MethodPost, request, &wire)
	result := MCPMutationResult{Completed: wire.Completed, Persisted: wire.Persisted, RuntimeApplied: wire.RuntimeApplied, RecoveryRequired: wire.RecoveryRequired, RegistryRevision: wire.RegistryRevision, Error: apiErr}
	if wire.Server.Name != "" {
		server := serverFromWire(wire.Server)
		result.Server = &server
	}
	if result.Error == nil && wire.SafeError != nil {
		code := safeMCPCode(wire.SafeError.Code)
		if code == "" {
			code = "MCP_REQUEST_FAILED"
		}
		result.Error = safeMCPAPIError(code, mcpErrorCategory(code), wire.SafeError.Retryable, nil)
	}
	return result
}

func (s *MCPService) environment(ctx context.Context, request mcpWireRequest) MCPEnvironmentResult {
	var wire mcpWireResponse
	apiErr := s.request(ctx, http.MethodPost, request, &wire)
	items := make([]MCPEnvironmentEntry, 0, len(wire.Items))
	for _, item := range wire.Items {
		items = append(items, MCPEnvironmentEntry{Key: item.Key, Configured: item.Configured})
	}
	return MCPEnvironmentResult{Revision: wire.EnvRevision, Items: items, Error: apiErr}
}

func (s *MCPService) request(ctx context.Context, method string, payload any, target *mcpWireResponse) *APIError {
	if s == nil || s.rootError != nil {
		return mcpServiceFailure("MCP_CORE_UNAVAILABLE", ErrorCategoryUnavailable, true)
	}
	access, err := s.readAccess(s.runtimeRoot)
	if err != nil {
		return mcpServiceFailure("MCP_CORE_UNAVAILABLE", ErrorCategoryUnavailable, true)
	}
	endpoint, err := safeLocalCoreEndpoint(access.Endpoint)
	if err != nil {
		return mcpServiceFailure("MCP_CORE_UNAVAILABLE", ErrorCategoryUnavailable, false)
	}
	var encoded []byte
	if method == http.MethodPost {
		encoded, err = json.Marshal(payload)
		if err != nil || len(encoded) > maxMCPDesktopRequestBytes {
			return mcpServiceFailure("INVALID_MCP_REQUEST", ErrorCategoryValidation, false)
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint+mcpDesktopPath, bytes.NewReader(encoded))
	if err != nil {
		return mcpServiceFailure("MCP_REQUEST_FAILED", ErrorCategoryOperation, false)
	}
	request.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	if access.Token != "" {
		request.Header.Set("Authorization", "Bearer "+access.Token)
	}
	response, err := s.client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return mcpServiceFailure("MCP_TIMEOUT", ErrorCategoryTimeout, true)
		}
		return mcpServiceFailure("MCP_CORE_UNAVAILABLE", ErrorCategoryUnavailable, true)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxMCPDesktopResponseBytes+1))
	if err != nil || len(data) > maxMCPDesktopResponseBytes {
		return mcpServiceFailure("MCP_RESPONSE_INVALID", ErrorCategoryInternal, false)
	}
	if response.StatusCode != http.StatusOK {
		return decodeMCPHTTPError(response.StatusCode, data)
	}
	if json.Unmarshal(data, target) != nil || !target.OK {
		return mcpServiceFailure("MCP_RESPONSE_INVALID", ErrorCategoryInternal, false)
	}
	return nil
}

func safeLocalCoreEndpoint(raw string) (string, error) {
	endpoint, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || endpoint.Scheme != "http" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") {
		return "", errors.New("invalid local Core endpoint")
	}
	host := endpoint.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", errors.New("Core endpoint is not loopback")
	}
	endpoint.Path, endpoint.RawPath = "", ""
	return strings.TrimSuffix(endpoint.String(), "/"), nil
}

func decodeMCPHTTPError(status int, data []byte) *APIError {
	var envelope struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	_ = json.Unmarshal(data, &envelope)
	code := safeMCPCode(envelope.Code)
	details := map[string]string{}
	if code == "APPROVAL_REQUIRED" {
		for _, key := range []string{"approval_id", "approval_version", "policy_revision", "executed", "retry"} {
			if value, ok := envelope.Details[key]; ok {
				if encoded, err := json.Marshal(value); err == nil && len(encoded) <= 256 {
					details[key] = strings.Trim(string(encoded), "\"")
				}
			}
		}
	}
	category := mcpErrorCategory(code)
	if status == http.StatusConflict && code == "MCP_REQUEST_FAILED" {
		category = ErrorCategoryConflict
	}
	return safeMCPAPIError(code, category, category == ErrorCategoryTimeout || category == ErrorCategoryUnavailable, details)
}

func safeMCPCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	switch code {
	case "APPROVAL_REQUIRED", "INVALID_MCP_REQUEST",
		"MCP_CORE_UNAVAILABLE", "MCP_REQUEST_FAILED", "MCP_RESPONSE_INVALID",
		"MCP_REGISTRY_CONFLICT", "MCP_SERVER_GENERATION_CONFLICT", "MCP_ENV_CONFLICT",
		"MCP_OWNED_BY_PLUGIN", "MCP_NAME_IMMUTABLE", "MCP_CONFIG_INVALID",
		"MCP_SERVER_EXISTS", "MCP_SERVER_NOT_FOUND", "MCP_SERVER_DISABLED", "MCP_SERVER_COLLISION",
		"MCP_MANAGER_CLOSED", "MCP_REGISTRY_READ_FAILED", "MCP_REGISTRY_WRITE_FAILED",
		"MCP_CLIENT_CLOSE_FAILED", "MCP_ENV_ERROR", "MCP_AUTH_REQUIRED", "MCP_AUTH_UNSUPPORTED",
		"MCP_AUTH_CLEAR_FAILED", "MCP_AUTH_CANCELLED", "MCP_AUTH_FAILED", "MCP_AUTH_CALLBACK_REQUIRED",
		"MCP_AUTH_CALLBACK_INVALID", "MCP_AUTH_CALLBACK_UNAVAILABLE", "MCP_AUTH_DENIED", "MCP_AUTH_EXPIRED",
		"MCP_CREDENTIAL_REQUIRED", "MCP_CONNECTION_FAILED", "MCP_START_FAILED",
		"MCP_TRANSPORT_ERROR", "MCP_TRANSPORT_REJECTED", "MCP_TRANSPORT_UNSUPPORTED",
		"MCP_PROTOCOL_ERROR", "MCP_INVALID_RESPONSE", "MCP_SCHEMA_INVALID", "MCP_TIMEOUT":
		return code
	default:
		return "MCP_REQUEST_FAILED"
	}
}

func mcpErrorCategory(code string) ErrorCategory {
	switch code {
	case "APPROVAL_REQUIRED":
		return ErrorCategoryPermission
	case "MCP_REGISTRY_CONFLICT", "MCP_SERVER_GENERATION_CONFLICT", "MCP_ENV_CONFLICT":
		return ErrorCategoryConflict
	case "MCP_SERVER_NOT_FOUND":
		return ErrorCategoryNotFound
	case "MCP_AUTH_REQUIRED", "MCP_AUTH_CLEAR_FAILED", "MCP_AUTH_CANCELLED", "MCP_AUTH_FAILED", "MCP_AUTH_CALLBACK_REQUIRED", "MCP_AUTH_CALLBACK_INVALID", "MCP_AUTH_CALLBACK_UNAVAILABLE", "MCP_AUTH_DENIED", "MCP_AUTH_EXPIRED":
		return ErrorCategoryAuthentication
	case "MCP_TIMEOUT":
		return ErrorCategoryTimeout
	case "MCP_MANAGER_CLOSED", "MCP_REGISTRY_READ_FAILED", "MCP_CORE_UNAVAILABLE":
		return ErrorCategoryUnavailable
	case "MCP_RESPONSE_INVALID":
		return ErrorCategoryInternal
	case "INVALID_MCP_REQUEST", "MCP_OWNED_BY_PLUGIN", "MCP_NAME_IMMUTABLE", "MCP_CONFIG_INVALID",
		"MCP_SERVER_EXISTS", "MCP_SERVER_DISABLED", "MCP_SERVER_COLLISION", "MCP_AUTH_UNSUPPORTED",
		"MCP_CREDENTIAL_REQUIRED", "MCP_TRANSPORT_UNSUPPORTED":
		return ErrorCategoryValidation
	default:
		return ErrorCategoryOperation
	}
}

func safeMCPAPIError(code string, category ErrorCategory, retryable bool, details map[string]string) *APIError {
	code = safeMCPCode(code)
	if code == "" {
		code = "MCP_REQUEST_FAILED"
	}
	return NewError(code, "MCP management request could not be completed", category, retryable, details)
}

func mcpServiceFailure(code string, category ErrorCategory, retryable bool) *APIError {
	return safeMCPAPIError(code, category, retryable, nil)
}

type mcpWireRequest struct {
	Action                   string            `json:"action"`
	Name                     string            `json:"name,omitempty"`
	Description              string            `json:"description,omitempty"`
	Transport                string            `json:"transport,omitempty"`
	ProtocolVersion          string            `json:"protocol_version,omitempty"`
	URL                      string            `json:"url,omitempty"`
	Command                  string            `json:"command,omitempty"`
	Args                     []string          `json:"args,omitempty"`
	CWD                      string            `json:"cwd,omitempty"`
	HeaderEnv                map[string]string `json:"header_env,omitempty"`
	EnvFromEnv               map[string]string `json:"env_from_env,omitempty"`
	Key                      string            `json:"key,omitempty"`
	Value                    *string           `json:"value,omitempty"`
	Enabled                  *bool             `json:"enabled,omitempty"`
	TimeoutMS                *int              `json:"timeout_ms,omitempty"`
	CallbackID               string            `json:"callback_id,omitempty"`
	ExpectedRegistryRevision string            `json:"expected_registry_revision,omitempty"`
	ExpectedGeneration       string            `json:"expected_generation,omitempty"`
	ExpectedEnvRevision      string            `json:"expected_env_revision,omitempty"`
}

type mcpWireObservation struct {
	Connection    string `json:"connection"`
	Status        string `json:"status"`
	AuthStatus    string `json:"auth_status"`
	ToolCount     *int   `json:"tool_count"`
	ObservedAt    string `json:"observed_at"`
	LastErrorCode string `json:"last_error_code"`
	Stale         bool   `json:"stale"`
}

type mcpWireServer struct {
	Name            string             `json:"name"`
	DisplayName     string             `json:"display_name"`
	Description     string             `json:"description"`
	SourceType      string             `json:"source_type"`
	PluginName      string             `json:"plugin_name"`
	Transport       string             `json:"transport"`
	ProtocolVersion string             `json:"protocol_version"`
	URL             string             `json:"url"`
	URLProtected    bool               `json:"url_protected"`
	Command         string             `json:"command"`
	CWD             string             `json:"cwd"`
	Args            []string           `json:"args"`
	ArgsProtected   bool               `json:"args_protected"`
	Enabled         bool               `json:"enabled"`
	TimeoutMS       int                `json:"timeout_ms"`
	HeaderEnv       map[string]string  `json:"header_env"`
	EnvFromEnv      map[string]string  `json:"env_from_env"`
	Generation      string             `json:"generation"`
	Observation     mcpWireObservation `json:"observation"`
}

type mcpWireEnvEntry struct {
	Key        string `json:"key"`
	Configured bool   `json:"configured"`
}

type mcpWireTool struct {
	Name          string `json:"name"`
	QualifiedName string `json:"qualified_name"`
	Server        string `json:"server"`
	SourceType    string `json:"source_type"`
	PluginName    string `json:"plugin_name"`
}

type mcpWireCallback struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type mcpWireSafeError struct {
	Code      string `json:"code"`
	Category  string `json:"category"`
	Retryable bool   `json:"retryable"`
}

type mcpWireResponse struct {
	OK               bool              `json:"ok"`
	RegistryRevision string            `json:"registry_revision"`
	Authoritative    bool              `json:"authoritative"`
	Servers          []mcpWireServer   `json:"servers"`
	Server           mcpWireServer     `json:"server"`
	Completed        bool              `json:"completed"`
	Persisted        bool              `json:"persisted"`
	RuntimeApplied   bool              `json:"runtime_applied"`
	RecoveryRequired bool              `json:"recovery_required"`
	SafeError        *mcpWireSafeError `json:"safe_error"`
	EnvRevision      string            `json:"env_revision"`
	Items            []mcpWireEnvEntry `json:"items"`
	Tools            []mcpWireTool     `json:"tools"`
	ToolCount        int               `json:"tool_count"`
	Status           string            `json:"status"`
	AuthorizationURL string            `json:"authorization_url"`
	CallbackID       string            `json:"callback_id"`
	ExpiresAt        string            `json:"expires_at"`
	CallbackOptions  []mcpWireCallback `json:"callback_options"`
	Removed          bool              `json:"removed"`
}

func snapshotFromWire(w mcpWireResponse) MCPSnapshot {
	servers := make([]MCPManagedServer, 0, len(w.Servers))
	for _, item := range w.Servers {
		servers = append(servers, serverFromWire(item))
	}
	return MCPSnapshot{RegistryRevision: w.RegistryRevision, Authoritative: w.Authoritative, Servers: servers}
}

func serverFromWire(w mcpWireServer) MCPManagedServer {
	return MCPManagedServer{
		Name: w.Name, DisplayName: w.DisplayName, Description: w.Description, SourceType: w.SourceType, PluginName: w.PluginName,
		Transport: w.Transport, ProtocolVersion: w.ProtocolVersion, URL: w.URL, URLProtected: w.URLProtected,
		Command: w.Command, CWD: w.CWD, Args: append([]string(nil), w.Args...), ArgsProtected: w.ArgsProtected,
		Enabled: w.Enabled, TimeoutMS: w.TimeoutMS, HeaderEnv: cloneStringMap(w.HeaderEnv), EnvFromEnv: cloneStringMap(w.EnvFromEnv),
		Generation:  w.Generation,
		Observation: MCPObservation{Connection: w.Observation.Connection, Status: w.Observation.Status, AuthStatus: w.Observation.AuthStatus, ToolCount: w.Observation.ToolCount, ObservedAt: w.Observation.ObservedAt, LastErrorCode: safeMCPCode(w.Observation.LastErrorCode), Stale: w.Observation.Stale},
	}
}

func callbacksFromWire(items []mcpWireCallback) []MCPAuthCallback {
	out := make([]MCPAuthCallback, 0, len(items))
	for _, item := range items {
		out = append(out, MCPAuthCallback{ID: item.ID, Label: item.Label})
	}
	return out
}

func mcpConfigRequest(action string, input MCPConfigInput) mcpWireRequest {
	return mcpWireRequest{Action: action, Name: strings.TrimSpace(input.Name), Description: input.Description, Transport: input.Transport, ProtocolVersion: input.ProtocolVersion, URL: input.URL, Command: input.Command, Args: append([]string(nil), input.Args...), CWD: input.CWD, HeaderEnv: cloneStringMap(input.HeaderEnv), EnvFromEnv: cloneStringMap(input.EnvFromEnv), Enabled: input.Enabled, TimeoutMS: input.TimeoutMS}
}
