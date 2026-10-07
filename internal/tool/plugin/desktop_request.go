package plugin

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/uvwt/agentdock/internal/envstore"
	pluginruntime "github.com/uvwt/agentdock/internal/plugin"

	toolcore "github.com/uvwt/agentdock/internal/tool/core"
)

// DesktopManageRequest is distinct from the model contract. Paths, review
// tokens, storage keys, and candidates are deliberately absent in Phase A.
type DesktopManageRequest struct {
	Action                   string  `json:"action"`
	RequestID                string  `json:"request_id,omitempty"`
	CandidateID              string  `json:"candidate_id,omitempty"`
	Name                     string  `json:"name,omitempty"`
	Component                string  `json:"component,omitempty"`
	Enabled                  *bool   `json:"enabled,omitempty"`
	Key                      string  `json:"key,omitempty"`
	Value                    *string `json:"value,omitempty"`
	ExpectedRegistryRevision string  `json:"expected_registry_revision,omitempty"`
	ExpectedGeneration       string  `json:"expected_generation,omitempty"`
	ExpectedEnvRevision      string  `json:"expected_env_revision,omitempty"`
}

func DecodeDesktopRequest(body []byte) (DesktopManageRequest, error) {
	var request DesktopManageRequest
	invalid := func() (DesktopManageRequest, error) {
		return DesktopManageRequest{}, DesktopError("INVALID_PLUGIN_REQUEST", "validation")
	}
	if len(body) > 64<<10 {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil {
		return invalid()
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return invalid()
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return invalid()
	}
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	switch request.Action {
	case "desktop_snapshot", "desktop_inspect", "desktop_operation_status", "desktop_install_candidate", "desktop_update_candidate", "desktop_set_enabled", "desktop_remove_keep", "desktop_remove_purge", "desktop_env_snapshot", "desktop_env_set", "desktop_env_unset":
	default:
		return invalid()
	}
	allowed := map[string]map[string]bool{
		"desktop_snapshot":          {"action": true},
		"desktop_inspect":           {"action": true, "name": true},
		"desktop_operation_status":  {"action": true, "request_id": true},
		"desktop_install_candidate": {"action": true, "request_id": true, "candidate_id": true, "name": true, "expected_registry_revision": true},
		"desktop_update_candidate":  {"action": true, "request_id": true, "candidate_id": true, "name": true, "expected_registry_revision": true, "expected_generation": true},
		"desktop_set_enabled":       {"action": true, "request_id": true, "name": true, "enabled": true, "expected_registry_revision": true, "expected_generation": true},
		"desktop_remove_keep":       {"action": true, "request_id": true, "name": true, "expected_registry_revision": true, "expected_generation": true},
		"desktop_remove_purge":      {"action": true, "request_id": true, "name": true, "expected_registry_revision": true, "expected_generation": true},
		"desktop_env_snapshot":      {"action": true, "name": true, "component": true},
		"desktop_env_set":           {"action": true, "request_id": true, "name": true, "component": true, "key": true, "value": true, "expected_registry_revision": true, "expected_generation": true, "expected_env_revision": true},
		"desktop_env_unset":         {"action": true, "request_id": true, "name": true, "component": true, "key": true, "expected_registry_revision": true, "expected_generation": true, "expected_env_revision": true},
	}[request.Action]
	for key := range raw {
		if !allowed[key] {
			return invalid()
		}
	}
	if request.Action != "desktop_snapshot" && request.Action != "desktop_operation_status" && pluginruntime.ValidateName(request.Name) != nil {
		return invalid()
	}
	if request.Component != "" && !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`).MatchString(request.Component) {
		return invalid()
	}
	if request.Key != "" && envstore.ValidateKey(request.Key) != nil {
		return invalid()
	}
	for _, token := range []string{request.ExpectedRegistryRevision, request.ExpectedGeneration, request.ExpectedEnvRevision} {
		if token != "" {
			if len(token) != 64 {
				return invalid()
			}
			if _, err := hex.DecodeString(token); err != nil {
				return invalid()
			}
		}
	}
	validRequestID := func(value string) bool {
		if len(value) != 32 {
			return false
		}
		_, err := hex.DecodeString(value)
		return err == nil
	}
	mutation := request.Action == "desktop_install_candidate" || request.Action == "desktop_update_candidate" || request.Action == "desktop_set_enabled" || request.Action == "desktop_remove_keep" || request.Action == "desktop_remove_purge" || request.Action == "desktop_env_set" || request.Action == "desktop_env_unset"
	if (mutation || request.Action == "desktop_operation_status") && !validRequestID(request.RequestID) {
		return invalid()
	}
	if mutation && request.ExpectedRegistryRevision == "" {
		return invalid()
	}
	if request.Action != "desktop_install_candidate" && mutation && request.ExpectedGeneration == "" {
		return invalid()
	}
	if (request.Action == "desktop_install_candidate" || request.Action == "desktop_update_candidate") && !validDesktopCandidateID(request.CandidateID) {
		return invalid()
	}
	if request.Action == "desktop_set_enabled" && request.Enabled == nil {
		return invalid()
	}
	if request.Action == "desktop_env_snapshot" && request.Component == "" {
		return invalid()
	}
	if (request.Action == "desktop_env_set" || request.Action == "desktop_env_unset") && (request.Component == "" || request.Key == "" || request.ExpectedEnvRevision == "") {
		return invalid()
	}
	if request.Action == "desktop_env_set" && request.Value == nil {
		return invalid()
	}
	return request, nil
}

func DesktopError(code, category string) *toolcore.ToolError {
	return toolcore.NewErrorDetails(code, "Plugin management request could not be completed", category, nil)
}
