package desktopapi

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

// Command/args are explicit local Desktop configuration fields. Environment
// mappings and raw persistence documents never cross this API boundary.
type ACPManagedProfile struct {
	ID                string   `json:"id"`
	DisplayName       string   `json:"displayName,omitempty"`
	RuntimeKind       string   `json:"runtimeKind"`
	Preset            string   `json:"preset"`
	Source            string   `json:"source"`
	Enabled           bool     `json:"enabled"`
	ConfiguredCommand string   `json:"configuredCommand,omitempty"`
	ConfiguredArgs    []string `json:"configuredArgs"`
	DetectedCommand   string   `json:"detectedCommand,omitempty"`
	DetectedArgs      []string `json:"detectedArgs,omitempty"`
	Availability      string   `json:"availability"`
	InstalledVersion  string   `json:"installedVersion,omitempty"`
	LatestVersion     string   `json:"latestVersion,omitempty"`
	VersionState      string   `json:"versionState"`
	CanDetect         bool     `json:"canDetect"`
	CanUpdate         bool     `json:"canUpdate"`
	BlockedReason     string   `json:"blockedReason,omitempty"`
}

type ACPManagerSnapshot struct {
	Revision       string                `json:"revision"`
	Enabled        bool                  `json:"enabled"`
	DefaultProfile string                `json:"defaultProfile"`
	Profiles       []ACPManagedProfile   `json:"profiles"`
	Capabilities   []OperationCapability `json:"capabilities"`
	Error          *APIError             `json:"error,omitempty"`
}

type ACPSettingsMutationResult struct {
	Completed       bool                `json:"completed"`
	Persisted       bool                `json:"persisted"`
	Applied         bool                `json:"applied"`
	RestartRequired bool                `json:"restartRequired"`
	RuntimeImpact   string              `json:"runtimeImpact"`
	Snapshot        *ACPManagerSnapshot `json:"snapshot,omitempty"`
	Error           *APIError           `json:"error,omitempty"`
}

type ACPProfileProbeResult struct {
	Profile *ACPManagedProfile `json:"profile,omitempty"`
	Error   *APIError          `json:"error,omitempty"`
}

type ACPProfileUpdateMutationResult struct {
	Completed       bool               `json:"completed"`
	RestartRequired bool               `json:"restartRequired"`
	RuntimeImpact   string             `json:"runtimeImpact"`
	Profile         *ACPManagedProfile `json:"profile,omitempty"`
	Error           *APIError          `json:"error,omitempty"`
}

func acpSettingsOperations() []OperationCapability {
	return []OperationCapability{
		{Name: "settings", Access: AccessRead},
		{Name: "probeProfile", Access: AccessRead},
		{Name: "checkProfileUpdate", Access: AccessRead},
		{Name: "saveSettings", Access: AccessMutating},
		{Name: "updateProfileAdapter", Access: AccessMutating, RequiresConfirmation: true},
	}
}

// Settings reads configuration only; it never probes Core, Memory or adapters.
func (s *ACPService) Settings(ctx context.Context) ACPManagerSnapshot {
	if s.rootError != nil {
		return ACPManagerSnapshot{Profiles: []ACPManagedProfile{}, Error: safeServiceError("acp_root_unavailable", s.rootError)}
	}
	config, err := s.readConfiguration(ctx, s.runtimeRoot)
	if err != nil {
		return ACPManagerSnapshot{Profiles: []ACPManagedProfile{}, Error: acpSettingsError(ctx, "acp_settings_read_failed", err)}
	}
	return acpManagerSnapshot(config)
}

func (s *ACPService) ProbeProfile(ctx context.Context, profileID string) ACPProfileProbeResult {
	config, profile, failure := s.readACPManagedProfile(ctx, profileID)
	if failure != nil {
		return ACPProfileProbeResult{Error: failure}
	}
	_ = config
	managed := acpManagedProfile(profile)
	probe, err := desktopruntime.ProbeACPProfile(ctx, s.runtimeRoot, managed.Preset, profile)
	if err != nil {
		return ACPProfileProbeResult{Error: safeContextServiceError(ctx, "acp_profile_probe_failed", err)}
	}
	applyACPProbe(&managed, probe)
	return ACPProfileProbeResult{Profile: &managed}
}

func (s *ACPService) CheckProfileUpdate(ctx context.Context, profileID string) ACPProfileProbeResult {
	_, profile, failure := s.readACPManagedProfile(ctx, profileID)
	if failure != nil {
		return ACPProfileProbeResult{Error: failure}
	}
	managed := acpManagedProfile(profile)
	probe, err := desktopruntime.ProbeACPProfile(ctx, s.runtimeRoot, managed.Preset, profile)
	if err != nil {
		return ACPProfileProbeResult{Error: safeContextServiceError(ctx, "acp_profile_probe_failed", err)}
	}
	applyACPProbe(&managed, probe)
	update, err := desktopruntime.CheckACPProfileUpdate(ctx, s.updateClient, s.updateSource, s.runtimeRoot, managed.Preset, profile, probe)
	if err != nil {
		return ACPProfileProbeResult{Error: safeContextServiceError(ctx, "acp_profile_update_check_failed", err)}
	}
	applyACPUpdate(&managed, update)
	return ACPProfileProbeResult{Profile: &managed}
}

func (s *ACPService) UpdateProfileAdapter(ctx context.Context, profileID, expectedVersion string) ACPProfileUpdateMutationResult {
	_, profile, failure := s.readACPManagedProfile(ctx, profileID)
	if failure != nil {
		return ACPProfileUpdateMutationResult{Error: failure}
	}
	managed := acpManagedProfile(profile)
	probe, err := desktopruntime.ProbeACPProfile(ctx, s.runtimeRoot, managed.Preset, profile)
	if err != nil {
		return ACPProfileUpdateMutationResult{Error: safeContextServiceError(ctx, "acp_profile_probe_failed", err)}
	}
	update, err := desktopruntime.CheckACPProfileUpdate(ctx, s.updateClient, s.updateSource, s.runtimeRoot, managed.Preset, profile, probe)
	if err != nil {
		return ACPProfileUpdateMutationResult{Error: safeContextServiceError(ctx, "acp_profile_update_check_failed", err)}
	}
	if !update.Supported || !update.Available {
		return ACPProfileUpdateMutationResult{Error: NewError("acp_profile_update_unavailable", "No trusted ACP adapter update is available", ErrorCategoryUnavailable, true, nil)}
	}
	if strings.TrimSpace(expectedVersion) == "" || strings.TrimSpace(expectedVersion) != update.LatestVersion {
		return ACPProfileUpdateMutationResult{Error: NewError("acp_profile_update_conflict", "ACP adapter update changed; check again before applying", ErrorCategoryConflict, false, nil)}
	}
	if err := desktopruntime.ApplyACPProfileUpdate(ctx, s.updateClient, s.updateSource, s.runtimeRoot, update); err != nil {
		return ACPProfileUpdateMutationResult{Error: safeContextServiceError(ctx, "acp_profile_update_failed", err)}
	}
	probe, err = desktopruntime.ProbeACPProfile(ctx, s.runtimeRoot, managed.Preset, profile)
	if err != nil {
		return ACPProfileUpdateMutationResult{Error: safeContextServiceError(ctx, "acp_profile_probe_failed", err)}
	}
	applyACPProbe(&managed, probe)
	managed.LatestVersion = update.LatestVersion
	managed.VersionState = "current"
	managed.CanUpdate = false
	managed.BlockedReason = ""
	return ACPProfileUpdateMutationResult{
		Completed: true, RestartRequired: false, RuntimeImpact: "existing_sessions_unchanged", Profile: &managed,
	}
}

func (s *ACPService) readACPManagedProfile(ctx context.Context, profileID string) (desktopruntime.ACPConfiguration, desktopruntime.ACPProfileSettings, *APIError) {
	if s.rootError != nil {
		return desktopruntime.ACPConfiguration{}, desktopruntime.ACPProfileSettings{}, safeServiceError("acp_root_unavailable", s.rootError)
	}
	config, err := s.readConfiguration(ctx, s.runtimeRoot)
	if err != nil {
		return desktopruntime.ACPConfiguration{}, desktopruntime.ACPProfileSettings{}, acpSettingsError(ctx, "acp_settings_read_failed", err)
	}
	id := strings.TrimSpace(profileID)
	for _, profile := range config.Profiles {
		if profile.ID == id {
			return config, profile, nil
		}
	}
	return config, desktopruntime.ACPProfileSettings{}, NewError("acp_profile_not_found", "ACP profile was not found", ErrorCategoryValidation, false, nil)
}

func applyACPProbe(managed *ACPManagedProfile, probe desktopruntime.ACPAdapterProbe) {
	managed.DetectedCommand = probe.Command
	managed.DetectedArgs = append([]string(nil), probe.Args...)
	managed.Availability = probe.Availability
	managed.InstalledVersion = probe.InstalledVersion
	managed.VersionState = probe.VersionState
	managed.BlockedReason = probe.BlockedReason
}

func applyACPUpdate(managed *ACPManagedProfile, update desktopruntime.ACPAdapterUpdate) {
	managed.LatestVersion = update.LatestVersion
	managed.VersionState = update.VersionState
	managed.CanUpdate = update.Available
	managed.BlockedReason = update.BlockedReason
}

// Profiles use the existing simplified configuration DTO (id, displayName,
// kind, command, args, enabled). An empty new ID is assigned by the backend.
func (s *ACPService) SaveSettings(ctx context.Context, expectedRevision string, enabled bool, defaultProfile string, profiles []desktopruntime.ACPProfileSettings) ACPSettingsMutationResult {
	if s.rootError != nil {
		return ACPSettingsMutationResult{Error: safeServiceError("acp_root_unavailable", s.rootError)}
	}
	config, err := s.saveConfiguration(ctx, s.runtimeRoot, expectedRevision, desktopruntime.ACPSettings{Enabled: enabled, DefaultProfile: defaultProfile, Profiles: profiles})
	if err != nil {
		return ACPSettingsMutationResult{Error: acpSettingsError(ctx, "acp_settings_save_failed", err)}
	}
	snapshot := acpManagerSnapshot(config)
	return ACPSettingsMutationResult{Completed: true, Persisted: true, Applied: false, RestartRequired: true, RuntimeImpact: "existing_runtime_unchanged", Snapshot: &snapshot}
}

func acpManagerSnapshot(config desktopruntime.ACPConfiguration) ACPManagerSnapshot {
	result := ACPManagerSnapshot{Revision: config.Revision, Enabled: config.Enabled, DefaultProfile: config.DefaultProfile, Profiles: []ACPManagedProfile{}, Capabilities: acpSettingsOperations()}
	for _, p := range config.Profiles {
		result.Profiles = append(result.Profiles, acpManagedProfile(p))
	}
	return result
}

func acpManagedProfile(p desktopruntime.ACPProfileSettings) ACPManagedProfile {
	preset, source := "custom", "custom"
	switch p.Kind {
	case "codex":
		preset, source = "codex", "builtin"
	case "claude", "grok":
		preset, source = "legacy", "builtin"
	case "custom":
		if p.ID == "antigravity" || strings.Contains(strings.ToLower(filepath.Base(p.Command)), "antigravity-acp") {
			preset, source = "antigravity", "custom-fork"
		}
	}
	args := append([]string{}, p.Args...)
	return ACPManagedProfile{
		ID: p.ID, DisplayName: p.DisplayName, RuntimeKind: p.Kind, Preset: preset, Source: source,
		Enabled: p.Enabled, ConfiguredCommand: p.Command, ConfiguredArgs: args,
		Availability: "unknown", VersionState: "not_checked", CanDetect: true, CanUpdate: false,
		BlockedReason: "update_not_available",
	}
}

func acpSettingsError(ctx context.Context, code string, err error) *APIError {
	switch {
	case errors.Is(err, desktopruntime.ErrACPSettingsConflict):
		return NewError("acp_settings_conflict", "ACP settings changed; reload and review before saving", ErrorCategoryConflict, false, nil)
	case errors.Is(err, desktopruntime.ErrACPSettingsInvalid):
		return NewError("acp_settings_invalid", "ACP profiles, commands, arguments and default must form a valid configuration", ErrorCategoryValidation, false, nil)
	default:
		return safeContextServiceError(ctx, code, err)
	}
}
