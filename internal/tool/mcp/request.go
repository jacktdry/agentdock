package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strings"
)

// ManageRequest 是 mcp_manage 进入动态 MCP capability 后的稳定输入契约。
type ManageRequest struct {
	Action          string            `json:"action"`
	Name            string            `json:"name,omitempty"`
	Description     string            `json:"description,omitempty"`
	Transport       string            `json:"transport,omitempty"`
	ProtocolVersion string            `json:"protocol_version,omitempty"`
	URL             string            `json:"url,omitempty"`
	Command         string            `json:"command,omitempty"`
	Args            []string          `json:"args,omitempty"`
	CWD             string            `json:"cwd,omitempty"`
	HeaderEnv       map[string]string `json:"header_env,omitempty"`
	EnvFromEnv      map[string]string `json:"env_from_env,omitempty"`
	Key             string            `json:"key,omitempty"`
	Value           *string           `json:"value,omitempty"`
	Enabled         *bool             `json:"enabled,omitempty"`
	TimeoutMS       *int              `json:"timeout_ms,omitempty"`
	CallbackID      string            `json:"callback_id,omitempty"`
}

// DesktopManageRequest is internal Runtime API input. It is deliberately
// separate from model-facing mcp_manage so optimistic-concurrency fields and
// write-only values never become part of the public tool schema.
type DesktopManageRequest struct {
	Action                     string            `json:"action"`
	RequestID                  string            `json:"request_id,omitempty"`
	Name                       string            `json:"name,omitempty"`
	Description                string            `json:"description,omitempty"`
	Transport                  string            `json:"transport,omitempty"`
	ProtocolVersion            string            `json:"protocol_version,omitempty"`
	URL                        string            `json:"url,omitempty"`
	Command                    string            `json:"command,omitempty"`
	Args                       []string          `json:"args,omitempty"`
	CWD                        string            `json:"cwd,omitempty"`
	HeaderEnv                  map[string]string `json:"header_env,omitempty"`
	EnvFromEnv                 map[string]string `json:"env_from_env,omitempty"`
	Key                        string            `json:"key,omitempty"`
	Value                      *string           `json:"value,omitempty"`
	Enabled                    *bool             `json:"enabled,omitempty"`
	TimeoutMS                  *int              `json:"timeout_ms,omitempty"`
	CallbackID                 string            `json:"callback_id,omitempty"`
	FlowID                     string            `json:"flow_id,omitempty"`
	ExpectedRegistryRevision   string            `json:"expected_registry_revision,omitempty"`
	ExpectedGeneration         string            `json:"expected_generation,omitempty"`
	ExpectedEnvRevision        string            `json:"expected_env_revision,omitempty"`
	ReuseConfiguredEnvironment bool              `json:"reuse_configured_environment,omitempty"`
	argsPresent                bool
}

func (r DesktopManageRequest) ArgsProvided() bool { return r.argsPresent }

type SearchRequest struct {
	Query  string `json:"query"`
	Server string `json:"server,omitempty"`
	Limit  *int   `json:"limit,omitempty"`
}

type InspectRequest struct {
	Name string `json:"name"`
}

// CallRequest.Arguments 是第三方 MCP 工具 schema 决定的动态叶子，必须保持开放对象。
type CallRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func intValue(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

// DecodeDesktopRequest is shared by the transport and internal application boundary.
// Decoder errors are deliberately fixed text: unknown keys may contain secrets.
func DecodeDesktopRequest(body []byte) (DesktopManageRequest, error) {
	invalid := func() (DesktopManageRequest, error) {
		return DesktopManageRequest{}, toolErrorDetails("INVALID_MCP_REQUEST", "Invalid Desktop MCP request", "validation", nil)
	}
	if len(body) > 64*1024 {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request DesktopManageRequest
	if err := decoder.Decode(&request); err != nil {
		return invalid()
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return invalid()
	}
	for key := range raw {
		if strings.EqualFold(key, "args") {
			request.argsPresent = true
			break
		}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return invalid()
	}
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	if !slices.Contains(desktopActions, request.Action) {
		return invalid()
	}
	return request, nil
}
