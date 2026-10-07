package desktopapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const pluginDesktopPath = "/internal/runtime/plugin/desktop"

type PluginManagedItem struct {
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Version            string            `json:"version"`
	Format             string            `json:"format"`
	Enabled            bool              `json:"enabled"`
	Generation         string            `json:"generation"`
	InstalledAt        string            `json:"installedAt"`
	SkillsCount        int               `json:"skillsCount"`
	MCPCount           int               `json:"mcpCount"`
	WarningCount       int               `json:"warningCount"`
	PackageFingerprint string            `json:"packageFingerprint"`
	Provenance         *PluginProvenance `json:"provenance,omitempty"`
	RecoveryState      string            `json:"recoveryState,omitempty"`
}
type PluginProvenance struct {
	Origin   string `json:"origin"`
	Ref      string `json:"ref,omitempty"`
	Revision string `json:"revision,omitempty"`
}
type PluginRecoveryItem struct {
	Name       string `json:"name"`
	Generation string `json:"generation"`
	State      string `json:"state"`
}
type PluginManagerSnapshot struct {
	RegistryRevision string               `json:"registryRevision"`
	Authoritative    bool                 `json:"authoritative"`
	Plugins          []PluginManagedItem  `json:"plugins"`
	RecoveryItems    []PluginRecoveryItem `json:"recoveryItems"`
}
type PluginSnapshotResult struct {
	Snapshot PluginManagerSnapshot `json:"snapshot"`
	Error    *APIError             `json:"error,omitempty"`
}
type PluginMutationInput struct {
	RequestID                string `json:"requestId"`
	Name                     string `json:"name"`
	ExpectedRegistryRevision string `json:"expectedRegistryRevision"`
	ExpectedGeneration       string `json:"expectedGeneration"`
}
type PluginEnvironmentInput struct {
	PluginMutationInput
	Component           string  `json:"component"`
	Key                 string  `json:"key"`
	Value               *string `json:"value,omitempty"`
	ExpectedEnvRevision string  `json:"expectedEnvRevision"`
}
type PluginOperationResult struct {
	RequestID         string                `json:"requestId,omitempty"`
	Outcome           string                `json:"outcome,omitempty"`
	OutcomeUnknown    bool                  `json:"outcomeUnknown"`
	Completed         bool                  `json:"completed"`
	Persisted         bool                  `json:"persisted"`
	RuntimeApplied    bool                  `json:"runtimeApplied"`
	RecoveryRequired  bool                  `json:"recoveryRequired"`
	RuntimeImpact     string                `json:"runtimeImpact,omitempty"`
	ReconnectRequired bool                  `json:"reconnectRequired"`
	RegistryRevision  string                `json:"registryRevision,omitempty"`
	Plugin            *PluginManagedItem    `json:"plugin,omitempty"`
	DataPolicy        string                `json:"dataPolicy,omitempty"`
	DataPreserved     bool                  `json:"dataPreserved"`
	EnvRevision       string                `json:"envRevision,omitempty"`
	Items             []MCPEnvironmentEntry `json:"items,omitempty"`
	Found             bool                  `json:"found"`
	Pending           bool                  `json:"pending"`
	OperationAction   string                `json:"operationAction,omitempty"`
	Error             *APIError             `json:"error,omitempty"`
}

// Phase A service is intentionally not registered with Shared frontend bindings.
type PluginService struct{ core *MCPService }

func NewPluginService(root string) *PluginService { return &PluginService{core: NewMCPService(root)} }
func (s *PluginService) Snapshot(ctx context.Context) PluginSnapshotResult {
	var wire pluginWireResponse
	err := s.request(ctx, http.MethodGet, pluginWireRequest{}, &wire)
	result := PluginSnapshotResult{Snapshot: pluginSnapshotFromWire(wire), Error: err}
	if result.Error == nil && wire.SafeError != nil {
		result.Error = pluginWireError(wire.SafeError)
	}
	return result
}
func (s *PluginService) Inspect(ctx context.Context, name string) PluginOperationResult {
	return s.operation(ctx, pluginWireRequest{Action: "desktop_inspect", Name: name})
}
func (s *PluginService) Environment(ctx context.Context, name, component string) PluginOperationResult {
	return s.operation(ctx, pluginWireRequest{Action: "desktop_env_snapshot", Name: name, Component: component})
}
func (s *PluginService) OperationStatus(ctx context.Context, id string) PluginOperationResult {
	return s.operation(ctx, pluginWireRequest{Action: "desktop_operation_status", RequestID: id})
}
func pluginMutationRequest(action string, input PluginMutationInput) pluginWireRequest {
	return pluginWireRequest{Action: action, RequestID: input.RequestID, Name: input.Name, ExpectedRegistryRevision: input.ExpectedRegistryRevision, ExpectedGeneration: input.ExpectedGeneration}
}
func (s *PluginService) SetEnabled(ctx context.Context, input PluginMutationInput, enabled bool) PluginOperationResult {
	request := pluginMutationRequest("desktop_set_enabled", input)
	request.Enabled = &enabled
	return s.operation(ctx, request)
}
func (s *PluginService) RemoveKeep(ctx context.Context, input PluginMutationInput) PluginOperationResult {
	return s.operation(ctx, pluginMutationRequest("desktop_remove_keep", input))
}
func (s *PluginService) RemovePurge(ctx context.Context, input PluginMutationInput) PluginOperationResult {
	return s.operation(ctx, pluginMutationRequest("desktop_remove_purge", input))
}
func (s *PluginService) SetEnvironment(ctx context.Context, input PluginEnvironmentInput) PluginOperationResult {
	return s.environmentMutation(ctx, "desktop_env_set", input)
}
func (s *PluginService) UnsetEnvironment(ctx context.Context, input PluginEnvironmentInput) PluginOperationResult {
	input.Value = nil
	return s.environmentMutation(ctx, "desktop_env_unset", input)
}
func (s *PluginService) environmentMutation(ctx context.Context, action string, input PluginEnvironmentInput) PluginOperationResult {
	request := pluginMutationRequest(action, input.PluginMutationInput)
	request.Component, request.Key, request.Value, request.ExpectedEnvRevision = input.Component, input.Key, input.Value, input.ExpectedEnvRevision
	return s.operation(ctx, request)
}
func (s *PluginService) operation(ctx context.Context, request pluginWireRequest) PluginOperationResult {
	var wire pluginWireResponse
	err := s.request(ctx, http.MethodPost, request, &wire)
	result := PluginOperationResult{RequestID: firstNonEmpty(wire.RequestID, request.RequestID), Outcome: wire.Outcome, OutcomeUnknown: wire.OutcomeUnknown,
		Completed: wire.Completed, Persisted: wire.Persisted, RuntimeApplied: wire.RuntimeApplied, RecoveryRequired: wire.RecoveryRequired, RuntimeImpact: wire.RuntimeImpact, ReconnectRequired: wire.ReconnectRequired,
		RegistryRevision: wire.RegistryRevision, DataPolicy: wire.DataPolicy, DataPreserved: wire.DataPreserved, EnvRevision: wire.EnvRevision, Items: wire.Items, Found: wire.Found, Pending: wire.Pending, OperationAction: wire.OperationAction, Error: err}
	if wire.Plugin.Name != "" {
		item := pluginItemFromWire(wire.Plugin)
		result.Plugin = &item
	}
	if result.Error == nil && wire.SafeError != nil {
		result.Error = pluginWireError(wire.SafeError)
	}
	if request.RequestID != "" && isMCPOutcomeUnknown(result.Error) || request.Action == "desktop_operation_status" && !wire.Found || wire.Outcome == "outcome_unknown" {
		result.OutcomeUnknown = true
		result.Outcome = "outcome_unknown"
	}
	return result
}
func pluginWireError(w *mcpWireSafeError) *APIError { return safePluginError(w.Code, w.Retryable, nil) }
func safePluginError(code string, retryable bool, details map[string]string) *APIError {
	category := ErrorCategoryOperation
	switch code {
	case "APPROVAL_REQUIRED":
		category = ErrorCategoryPermission
	case "INVALID_PLUGIN_REQUEST", "PLUGIN_OPERATION_ID_INVALID", "PLUGIN_ENV_OWNERSHIP_INVALID", "PLUGIN_CREDENTIAL_REQUIRED":
		category = ErrorCategoryValidation
	case "PLUGIN_REGISTRY_CONFLICT", "PLUGIN_GENERATION_CONFLICT", "PLUGIN_ENV_CONFLICT", "PLUGIN_RECOVERY_REQUIRED", "PLUGIN_OPERATION_ID_CONFLICT", "PLUGIN_OPERATION_IN_PROGRESS":
		category = ErrorCategoryConflict
	case "PLUGIN_NOT_FOUND":
		category = ErrorCategoryNotFound
	case "PLUGIN_TIMEOUT":
		category = ErrorCategoryTimeout
	case "PLUGIN_CORE_UNAVAILABLE", "PLUGIN_REGISTRY_READ_FAILED", "PLUGIN_ENV_UNAVAILABLE", "PLUGIN_PACKAGE_UNAVAILABLE", "PLUGIN_OPERATION_UNAVAILABLE":
		category = ErrorCategoryUnavailable
	case "PLUGIN_OPERATION_LIMIT":
		category = ErrorCategoryCapacity
	case "PLUGIN_RESPONSE_INVALID":
		category = ErrorCategoryInternal
	case "PLUGIN_OPERATION_FAILED":
	default:
		code = "PLUGIN_OPERATION_FAILED"
	}
	return NewError(code, "Plugin management request could not be completed", category, retryable, details)
}
func (s *PluginService) request(ctx context.Context, method string, payload pluginWireRequest, target *pluginWireResponse) *APIError {
	if s == nil || s.core == nil || s.core.rootError != nil {
		return safePluginError("PLUGIN_CORE_UNAVAILABLE", true, nil)
	}
	access, err := s.core.readAccess(s.core.runtimeRoot)
	if err != nil {
		return safePluginError("PLUGIN_CORE_UNAVAILABLE", true, nil)
	}
	endpoint, err := safeLocalCoreEndpoint(access.Endpoint)
	if err != nil {
		return safePluginError("PLUGIN_CORE_UNAVAILABLE", false, nil)
	}
	var data []byte
	if method == http.MethodPost {
		data, err = json.Marshal(payload)
		if err != nil || len(data) > maxMCPDesktopRequestBytes {
			return safePluginError("INVALID_PLUGIN_REQUEST", false, nil)
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint+pluginDesktopPath, bytes.NewReader(data))
	if err != nil {
		return safePluginError("INVALID_PLUGIN_REQUEST", false, nil)
	}
	request.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	if access.Token != "" {
		request.Header.Set("Authorization", "Bearer "+access.Token)
	}
	response, err := s.core.client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return safePluginError("PLUGIN_TIMEOUT", true, nil)
		}
		return safePluginError("PLUGIN_CORE_UNAVAILABLE", true, nil)
	}
	defer response.Body.Close()
	data, err = io.ReadAll(io.LimitReader(response.Body, maxMCPDesktopResponseBytes+1))
	if err != nil || len(data) > maxMCPDesktopResponseBytes {
		return safePluginError("PLUGIN_RESPONSE_INVALID", false, nil)
	}
	if response.StatusCode != http.StatusOK {
		var envelope struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		}
		_ = json.Unmarshal(data, &envelope)
		details := map[string]string{}
		if envelope.Code == "APPROVAL_REQUIRED" {
			for _, key := range []string{"approval_id", "approval_version", "policy_revision", "executed", "retry"} {
				if value, ok := envelope.Details[key]; ok {
					if encoded, err := json.Marshal(value); err == nil && len(encoded) <= 256 {
						details[key] = strings.Trim(string(encoded), "\"")
					}
				}
			}
		}
		return safePluginError(envelope.Code, false, details)
	}
	if json.Unmarshal(data, target) != nil || !target.OK {
		return safePluginError("PLUGIN_RESPONSE_INVALID", false, nil)
	}
	return nil
}

type pluginWireRequest struct {
	Action                   string  `json:"action"`
	RequestID                string  `json:"request_id,omitempty"`
	Name                     string  `json:"name,omitempty"`
	Component                string  `json:"component,omitempty"`
	Enabled                  *bool   `json:"enabled,omitempty"`
	Key                      string  `json:"key,omitempty"`
	Value                    *string `json:"value,omitempty"`
	ExpectedRegistryRevision string  `json:"expected_registry_revision,omitempty"`
	ExpectedGeneration       string  `json:"expected_generation,omitempty"`
	ExpectedEnvRevision      string  `json:"expected_env_revision,omitempty"`
}
type pluginWireItem struct {
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Version            string            `json:"version"`
	Format             string            `json:"format"`
	Enabled            bool              `json:"enabled"`
	Generation         string            `json:"generation"`
	InstalledAt        string            `json:"installed_at"`
	SkillsCount        int               `json:"skills_count"`
	MCPCount           int               `json:"mcp_count"`
	WarningCount       int               `json:"warning_count"`
	PackageFingerprint string            `json:"package_fingerprint"`
	Provenance         *PluginProvenance `json:"provenance"`
	RecoveryState      string            `json:"recovery_state"`
}
type pluginWireResponse struct {
	OK                bool                  `json:"ok"`
	RegistryRevision  string                `json:"registry_revision"`
	Authoritative     bool                  `json:"authoritative"`
	Plugins           []pluginWireItem      `json:"plugins"`
	Plugin            pluginWireItem        `json:"plugin"`
	RecoveryItems     []PluginRecoveryItem  `json:"recovery_items"`
	RequestID         string                `json:"request_id"`
	Outcome           string                `json:"outcome"`
	OutcomeUnknown    bool                  `json:"outcome_unknown"`
	Completed         bool                  `json:"completed"`
	Persisted         bool                  `json:"persisted"`
	RuntimeApplied    bool                  `json:"runtime_applied"`
	RecoveryRequired  bool                  `json:"recovery_required"`
	RuntimeImpact     string                `json:"runtime_impact"`
	ReconnectRequired bool                  `json:"reconnect_required"`
	DataPolicy        string                `json:"data_policy"`
	DataPreserved     bool                  `json:"data_preserved"`
	EnvRevision       string                `json:"env_revision"`
	Items             []MCPEnvironmentEntry `json:"items"`
	Found             bool                  `json:"found"`
	Pending           bool                  `json:"pending"`
	OperationAction   string                `json:"operation_action"`
	SafeError         *mcpWireSafeError     `json:"safe_error"`
}

func pluginItemFromWire(w pluginWireItem) PluginManagedItem {
	return PluginManagedItem{Name: w.Name, Description: w.Description, Version: w.Version, Format: w.Format, Enabled: w.Enabled, Generation: w.Generation, InstalledAt: w.InstalledAt, SkillsCount: w.SkillsCount, MCPCount: w.MCPCount, WarningCount: w.WarningCount, PackageFingerprint: w.PackageFingerprint, Provenance: w.Provenance, RecoveryState: w.RecoveryState}
}
func pluginSnapshotFromWire(w pluginWireResponse) PluginManagerSnapshot {
	items := make([]PluginManagedItem, 0, len(w.Plugins))
	for _, item := range w.Plugins {
		items = append(items, pluginItemFromWire(item))
	}
	recoveries := append([]PluginRecoveryItem{}, w.RecoveryItems...)
	return PluginManagerSnapshot{RegistryRevision: w.RegistryRevision, Authoritative: w.Authoritative, Plugins: items, RecoveryItems: recoveries}
}
