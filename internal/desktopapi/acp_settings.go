package desktopapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	ProtectedArgs     bool     `json:"protectedArgs,omitempty"`
	DetectedCommand   string   `json:"detectedCommand,omitempty"`
	DetectedArgs      []string `json:"detectedArgs,omitempty"`
	Availability      string   `json:"availability"`
	InstalledVersion  string   `json:"installedVersion,omitempty"`
	LatestVersion     string   `json:"latestVersion,omitempty"`
	UpdatePlan        string   `json:"updatePlan,omitempty"`
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
	config, profile, failure := s.readACPManagedProfile(ctx, profileID)
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
	if update.Supported && update.Available {
		managed.UpdatePlan = acpUpdatePlanToken(config.Revision, profile, update)
	}
	return ACPProfileProbeResult{Profile: &managed}
}

func (s *ACPService) UpdateProfileAdapter(ctx context.Context, profileID, expectedPlan string) ACPProfileUpdateMutationResult {
	if s.rootError != nil {
		return ACPProfileUpdateMutationResult{Error: safeServiceError("acp_root_unavailable", s.rootError)}
	}
	release, err := desktopruntime.AcquireDesktopMutation(ctx, s.runtimeRoot)
	if err != nil {
		return ACPProfileUpdateMutationResult{Error: safeContextServiceError(ctx, "acp_profile_update_lock_failed", err)}
	}
	defer release()

	config, profile, failure := s.readACPManagedProfile(ctx, profileID)
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
	plan := acpUpdatePlanToken(config.Revision, profile, update)
	if strings.TrimSpace(expectedPlan) == "" || strings.TrimSpace(expectedPlan) != plan {
		return ACPProfileUpdateMutationResult{Error: NewError("acp_profile_update_conflict", "ACP adapter update changed; check again before applying", ErrorCategoryConflict, false, nil)}
	}
	applyUpdate := s.applyUpdate
	if applyUpdate == nil {
		applyUpdate = desktopruntime.ApplyACPProfileUpdate
	}
	if err := applyUpdate(ctx, s.updateClient, s.updateSource, s.runtimeRoot, update); err != nil {
		if errors.Is(err, desktopruntime.ErrACPUpdateOutcomeUnknown) {
			reconciled := managed
			if observed, probeErr := desktopruntime.ProbeACPProfile(ctx, s.runtimeRoot, managed.Preset, profile); probeErr == nil {
				applyACPProbe(&reconciled, observed)
			} else {
				reconciled.Availability = "unknown"
				reconciled.InstalledVersion = ""
			}
			reconciled.LatestVersion = update.LatestVersion
			reconciled.UpdatePlan = ""
			reconciled.VersionState = "unavailable"
			reconciled.CanUpdate = false
			reconciled.BlockedReason = "update_outcome_unknown"
			return ACPProfileUpdateMutationResult{
				Completed: false, RestartRequired: false, RuntimeImpact: "adapter_update_outcome_unknown", Profile: &reconciled,
				Error: NewError("acp_profile_update_outcome_unknown", "ACP adapter promotion could not be durably confirmed; check the adapter state before retrying", ErrorCategoryConflict, false, nil),
			}
		}
		return ACPProfileUpdateMutationResult{Error: safeContextServiceError(ctx, "acp_profile_update_failed", err)}
	}
	applyACPProbe(&managed, probe)
	managed.InstalledVersion = update.LatestVersion
	managed.LatestVersion = update.LatestVersion
	managed.UpdatePlan = ""
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
	if managed.ProtectedArgs || containsSensitiveACPArguments(probe.Args) {
		managed.ProtectedArgs = true
		managed.DetectedArgs = nil
	} else {
		managed.DetectedArgs = append([]string(nil), probe.Args...)
	}
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
	protectedArgs := containsSensitiveACPArguments(args)
	if protectedArgs {
		args = []string{}
	}
	return ACPManagedProfile{
		ID: p.ID, DisplayName: p.DisplayName, RuntimeKind: p.Kind, Preset: preset, Source: source,
		Enabled: p.Enabled, ConfiguredCommand: p.Command, ConfiguredArgs: args, ProtectedArgs: protectedArgs,
		Availability: "unknown", VersionState: "not_checked", CanDetect: true, CanUpdate: false,
		BlockedReason: "update_not_available",
	}
}

func acpUpdatePlanToken(revision string, profile desktopruntime.ACPProfileSettings, update desktopruntime.ACPAdapterUpdate) string {
	payload, _ := json.Marshal(struct {
		Revision  string                          `json:"revision"`
		ProfileID string                          `json:"profileId"`
		Command   string                          `json:"command"`
		Update    desktopruntime.ACPAdapterUpdate `json:"update"`
	}{
		Revision:  revision,
		ProfileID: profile.ID,
		Command:   profile.Command,
		Update:    update,
	})
	sum := sha256.Sum256(append([]byte("acp-update-plan-v1\x00"), payload...))
	return hex.EncodeToString(sum[:])
}

func containsSensitiveACPArguments(args []string) bool {
	for _, arg := range args {
		value := strings.TrimSpace(arg)
		if value == "" {
			continue
		}
		lower := strings.ToLower(value)
		if strings.HasPrefix(lower, "bearer ") ||
			strings.Contains(lower, "authorization:") ||
			strings.Contains(lower, "proxy-authorization:") {
			return true
		}

		key := value
		if index := strings.IndexAny(key, "=:"); index >= 0 {
			key = key[:index]
		}
		key = strings.TrimLeft(strings.TrimSpace(key), "-/")
		normalized := strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(strings.ToLower(key))
		switch normalized {
		case "apikey", "accesstoken", "authtoken", "authorization", "credential", "credentials",
			"clientsecret", "password", "passwd", "secret", "token", "bearertoken",
			"refreshtoken", "idtoken", "privatekey", "secretkey", "signingkey", "accesskey":
			return true
		}
		if strings.HasSuffix(normalized, "token") && normalized != "token" {
			return true
		}
	}
	return false
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
