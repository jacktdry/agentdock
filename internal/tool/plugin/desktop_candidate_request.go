package plugin

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"

	pluginruntime "github.com/uvwt/agentdock/internal/plugin"
)

// DesktopCandidateRequest is a private local-Desktop transport contract.
// Source may contain an absolute native picker path, so this type must never be
// exposed as a Wails/renderer binding or model-facing tool schema.
type DesktopCandidateRequest struct {
	Action           string `json:"action"`
	Source           string `json:"source,omitempty"`
	Kind             string `json:"kind,omitempty"`
	TargetName       string `json:"target_name,omitempty"`
	TargetGeneration string `json:"target_generation,omitempty"`
	CandidateID      string `json:"candidate_id,omitempty"`
}

func DecodeDesktopCandidateRequest(body []byte) (DesktopCandidateRequest, error) {
	invalid := func() (DesktopCandidateRequest, error) {
		return DesktopCandidateRequest{}, DesktopError("PLUGIN_CANDIDATE_INVALID", "validation")
	}
	if len(body) > 64<<10 {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request DesktopCandidateRequest
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
	case "prepare":
		allowed := map[string]bool{"action": true, "source": true, "kind": true, "target_name": true, "target_generation": true}
		for key := range raw {
			if !allowed[key] {
				return invalid()
			}
		}
		request.Source = strings.TrimSpace(request.Source)
		request.Kind = strings.ToLower(strings.TrimSpace(request.Kind))
		if request.Source == "" || !filepath.IsAbs(request.Source) || strings.Contains(request.Source, "://") || (request.Kind != "install" && request.Kind != "update") {
			return invalid()
		}
		if request.Kind == "install" {
			if request.TargetName != "" || request.TargetGeneration != "" {
				return invalid()
			}
			return request, nil
		}
		request.TargetName = strings.TrimSpace(request.TargetName)
		if pluginruntime.ValidateName(request.TargetName) != nil || len(request.TargetGeneration) != 64 {
			return invalid()
		}
		if _, err := hex.DecodeString(request.TargetGeneration); err != nil {
			return invalid()
		}
		return request, nil
	case "discard":
		if len(raw) != 2 || raw["action"] == nil || raw["candidate_id"] == nil || !validDesktopCandidateID(request.CandidateID) {
			return invalid()
		}
		return request, nil
	default:
		return invalid()
	}
}
