package desktopruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	agentconfig "github.com/uvwt/agentdock/internal/config"
)

type ACPProfileSettings struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"displayName,omitempty"`
	Kind        string   `json:"kind"`
	Command     string   `json:"command,omitempty"`
	Args        []string `json:"args,omitempty"`
	Enabled     bool     `json:"enabled"`
}

type ACPSettings struct {
	Enabled        bool                 `json:"enabled"`
	Profiles       []ACPProfileSettings `json:"profiles"`
	DefaultProfile string               `json:"defaultProfile,omitempty"`
}

func ReadACPSettings(ctx context.Context, runtimeRoot string) (ACPSettings, error) {
	if ctx == nil {
		return ACPSettings{}, fmt.Errorf("context is required")
	}
	if err := ctx.Err(); err != nil {
		return ACPSettings{}, err
	}
	return platformReadACPSettings(ctx, runtimeRoot)
}

func acpSettingsFromProfiles(enabled bool, profiles []agentconfig.ACPProfile, defaultProfile string) ACPSettings {
	result := ACPSettings{Enabled: enabled, DefaultProfile: strings.TrimSpace(defaultProfile), Profiles: make([]ACPProfileSettings, 0, len(profiles))}
	for _, profile := range profiles {
		result.Profiles = append(result.Profiles, ACPProfileSettings{
			ID: strings.TrimSpace(profile.ID), DisplayName: strings.TrimSpace(profile.DisplayName), Kind: strings.ToLower(strings.TrimSpace(profile.Kind)),
			Command: strings.TrimSpace(profile.Command), Args: append([]string(nil), profile.Args...), Enabled: profile.Enabled,
		})
	}
	return result
}

func acpSettingsFromEnvironment(values map[string]string) (ACPSettings, error) {
	enabled := false
	if raw := strings.TrimSpace(values["AGENTDOCK_ACP_ENABLED"]); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return ACPSettings{}, fmt.Errorf("invalid AGENTDOCK_ACP_ENABLED")
		}
		enabled = parsed
	}
	if raw := strings.TrimSpace(values["AGENTDOCK_ACP_PROFILES_JSON"]); raw != "" {
		var profiles []agentconfig.ACPProfile
		if err := json.Unmarshal([]byte(raw), &profiles); err != nil {
			return ACPSettings{}, fmt.Errorf("invalid AGENTDOCK_ACP_PROFILES_JSON")
		}
		return acpSettingsFromProfiles(enabled, profiles, values["AGENTDOCK_ACP_DEFAULT_PROFILE"]), nil
	}
	if !enabled && strings.TrimSpace(values["AGENTDOCK_ACP_AGENT"]) == "" && strings.TrimSpace(values["AGENTDOCK_ACP_COMMAND"]) == "" {
		return ACPSettings{Enabled: false, Profiles: []ACPProfileSettings{}}, nil
	}
	agent := strings.TrimSpace(values["AGENTDOCK_ACP_AGENT"])
	if agent == "" {
		agent = "claude"
	}
	kind := strings.ToLower(agent)
	switch kind {
	case "codex", "claude", "grok":
	default:
		kind = "custom"
	}
	var args []string
	if raw := strings.TrimSpace(values["AGENTDOCK_ACP_ARGS_JSON"]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			return ACPSettings{}, fmt.Errorf("invalid AGENTDOCK_ACP_ARGS_JSON")
		}
	}
	profiles := []agentconfig.ACPProfile{{ID: agent, Kind: kind, Command: strings.TrimSpace(values["AGENTDOCK_ACP_COMMAND"]), Args: args, Enabled: true}}
	return acpSettingsFromProfiles(enabled, profiles, agent), nil
}
