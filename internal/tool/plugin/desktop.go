package plugin

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/envstore"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	pluginruntime "github.com/uvwt/agentdock/internal/plugin"
	toolcore "github.com/uvwt/agentdock/internal/tool/core"
)

// These DTOs contain only renderer-safe projections; never embed State/Installed.
type DesktopItem struct {
	Name               string                    `json:"name"`
	Description        string                    `json:"description"`
	Version            string                    `json:"version"`
	Format             string                    `json:"format"`
	Enabled            bool                      `json:"enabled"`
	Generation         string                    `json:"generation"`
	InstalledAt        string                    `json:"installed_at"`
	SkillsCount        int                       `json:"skills_count"`
	MCPCount           int                       `json:"mcp_count"`
	WarningCount       int                       `json:"warning_count"`
	PackageFingerprint string                    `json:"package_fingerprint"`
	Provenance         *pluginruntime.Provenance `json:"provenance,omitempty"`
	RecoveryState      string                    `json:"recovery_state,omitempty"`
}
type DesktopMCPComponent struct {
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Transport        string   `json:"transport"`
	Endpoint         string   `json:"endpoint,omitempty"`
	Command          string   `json:"command,omitempty"`
	EnvironmentNames []string `json:"environment_names"`
	HeaderNames      []string `json:"header_names"`
}
type DesktopDetail struct {
	Plugin   DesktopItem           `json:"plugin"`
	MCP      []DesktopMCPComponent `json:"mcp"`
	Warnings []string              `json:"warnings"`
}
type DesktopRecoveryItem struct {
	Name       string `json:"name"`
	Generation string `json:"generation"`
	State      string `json:"state"`
}

func uniqueDesktopStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := values[:0]
	for _, value := range values {
		if value == "" || (len(result) > 0 && result[len(result)-1] == value) {
			continue
		}
		result = append(result, value)
	}
	return result
}

func desktopText(value string) string {
	value = strings.TrimSpace(value)
	for _, char := range value {
		if unicode.IsControl(char) {
			return ""
		}
	}
	runes := []rune(value)
	if len(runes) > 512 {
		runes = runes[:512]
	}
	return redactAbsolutePathTokens(string(runes))
}
func desktopItem(state pluginruntime.State) DesktopItem {
	item := DesktopItem{Name: state.Name, Description: desktopText(state.Description), Version: desktopText(state.Version), Format: state.Format,
		Enabled: state.Enabled, Generation: pluginruntime.DesktopGeneration(state), InstalledAt: state.InstalledAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		SkillsCount: len(state.Components.Skills), MCPCount: len(state.Components.MCP), WarningCount: len(state.Warnings), PackageFingerprint: state.PackageDigest}
	switch item.Format {
	case "portable", "openai", "claude":
	default:
		item.Format = "unknown"
	}
	if state.Provenance != nil {
		p := state.Provenance
		safe := &pluginruntime.Provenance{
			Ref:      safeCandidateOpaqueLabel(p.Ref, "", ""),
			Revision: safeCandidateOpaqueLabel(p.Revision, "", ""),
		}
		if endpoint, err := url.Parse(p.Origin); err == nil && endpoint.Scheme == "https" && endpoint.Host != "" && endpoint.User == nil {
			safe.Origin = "https://" + endpoint.Host
		}
		item.Provenance = safe
	}
	return item
}

func desktopDetail(state pluginruntime.State) DesktopDetail {
	mcp := make([]DesktopMCPComponent, 0, len(state.Components.MCP))
	for _, component := range state.Components.MCP {
		envNames := make([]string, 0, len(component.Environment)+len(component.EnvBindings)+len(component.RequiredEnv))
		for key := range component.Environment {
			envNames = append(envNames, key)
		}
		for _, key := range component.EnvBindings {
			envNames = append(envNames, key)
		}
		envNames = append(envNames, component.RequiredEnv...)
		sort.Strings(envNames)
		envNames = uniqueDesktopStrings(envNames)

		headerNames := make([]string, 0, len(component.Headers)+len(component.HeaderEnv))
		for key := range component.Headers {
			headerNames = append(headerNames, key)
		}
		for key := range component.HeaderEnv {
			headerNames = append(headerNames, key)
		}
		sort.Strings(headerNames)
		headerNames = uniqueDesktopStrings(headerNames)

		command := ""
		if component.Command != "" {
			command = safeCandidateText(filepath.Base(component.Command), 128)
		}
		mcp = append(mcp, DesktopMCPComponent{
			Name: safeCandidateText(component.Name, 128), Description: safeCandidateText(component.Description, 512),
			Transport: safeCandidateText(component.Transport, 32), Endpoint: safeCandidateOrigin(component.URL), Command: command,
			EnvironmentNames: envNames, HeaderNames: headerNames,
		})
	}
	warnings := make([]string, 0, len(state.Warnings))
	for _, warning := range state.Warnings {
		if safe := safeCandidateText(warning, 512); safe != "" {
			warnings = append(warnings, safe)
		}
	}
	return DesktopDetail{Plugin: desktopItem(state), MCP: mcp, Warnings: warnings}
}

func desktopSnapshot(reg pluginruntime.DesktopRegistry) Result {
	items := []DesktopItem{}
	recoveries := []DesktopRecoveryItem{}
	for _, state := range reg.States {
		item := desktopItem(state)
		purging := false
		for _, recovery := range reg.Recoveries {
			if recovery.Name == state.Name {
				item.RecoveryState = recovery.State
				purging = recovery.State == "cleanup_required"
			}
		}
		if !purging {
			items = append(items, item)
		}
	}
	for _, recovery := range reg.Recoveries {
		recoveries = append(recoveries, DesktopRecoveryItem{recovery.Name, recovery.Generation, recovery.State})
	}
	return Result{"registry_revision": reg.Revision, "authoritative": true, "plugins": items, "recovery_items": recoveries}
}

func (s *Service) desktopBasis(r DesktopManageRequest) (pluginruntime.DesktopRegistry, pluginruntime.State, error) {
	reg, err := s.manager.Store().DesktopRegistrySnapshot()
	if err != nil {
		return reg, pluginruntime.State{}, DesktopError("PLUGIN_REGISTRY_READ_FAILED", "unavailable")
	}
	if r.ExpectedRegistryRevision == "" || r.ExpectedRegistryRevision != reg.Revision {
		return reg, pluginruntime.State{}, DesktopError("PLUGIN_REGISTRY_CONFLICT", "conflict")
	}
	for _, recovery := range reg.Recoveries {
		if recovery.Name == r.Name {
			if r.ExpectedGeneration == "" || r.ExpectedGeneration != recovery.Generation {
				return reg, pluginruntime.State{}, DesktopError("PLUGIN_GENERATION_CONFLICT", "conflict")
			}
			if recovery.State == "cleanup_required" && r.Action == "desktop_remove_purge" {
				return reg, pluginruntime.State{Name: r.Name}, nil
			}
			return reg, pluginruntime.State{}, DesktopError("PLUGIN_RECOVERY_REQUIRED", "conflict")
		}
	}
	for _, state := range reg.States {
		if state.Name == r.Name {
			if r.ExpectedGeneration == "" || r.ExpectedGeneration != pluginruntime.DesktopGeneration(state) {
				return reg, state, DesktopError("PLUGIN_GENERATION_CONFLICT", "conflict")
			}
			return reg, state, nil
		}
	}
	return reg, pluginruntime.State{}, DesktopError("PLUGIN_NOT_FOUND", "not_found")
}

func (s *Service) DesktopManage(ctx context.Context, r DesktopManageRequest) (Result, error) {
	release, err := s.manager.Store().AcquireManagement(ctx)
	if err != nil {
		return nil, DesktopError("PLUGIN_CORE_UNAVAILABLE", "unavailable")
	}
	defer release()
	reg, err := s.manager.Store().DesktopRegistrySnapshot()
	if err != nil {
		if r.Action == "desktop_snapshot" {
			return Result{"authoritative": false, "plugins": []DesktopItem{}, "recovery_items": []DesktopRecoveryItem{}, "safe_error": DesktopError("PLUGIN_REGISTRY_READ_FAILED", "unavailable")}, nil
		}
		return nil, DesktopError("PLUGIN_REGISTRY_READ_FAILED", "unavailable")
	}
	if r.Action == "desktop_snapshot" {
		return desktopSnapshot(reg), nil
	}
	if pluginruntime.ValidateName(r.Name) != nil {
		return nil, DesktopError("INVALID_PLUGIN_REQUEST", "validation")
	}
	if r.Action == "desktop_inspect" {
		for _, state := range reg.States {
			if state.Name == r.Name {
				for _, recovery := range reg.Recoveries {
					if recovery.Name == r.Name {
						return nil, DesktopError("PLUGIN_RECOVERY_REQUIRED", "conflict")
					}
				}
				detail := desktopDetail(state)
				return Result{"registry_revision": reg.Revision, "authoritative": true, "plugin": detail.Plugin, "detail": detail}, nil
			}
		}
		return nil, DesktopError("PLUGIN_NOT_FOUND", "not_found")
	}
	if r.Action == "desktop_env_snapshot" {
		for _, state := range reg.States {
			if state.Name == r.Name {
				for _, recovery := range reg.Recoveries {
					if recovery.Name == r.Name {
						return nil, DesktopError("PLUGIN_RECOVERY_REQUIRED", "conflict")
					}
				}
				return s.desktopEnvironment(ctx, r, state)
			}
		}
		return nil, DesktopError("PLUGIN_NOT_FOUND", "not_found")
	}
	var state pluginruntime.State
	if r.Action == "desktop_install_candidate" {
		if r.ExpectedRegistryRevision == "" || r.ExpectedRegistryRevision != reg.Revision {
			return nil, DesktopError("PLUGIN_REGISTRY_CONFLICT", "conflict")
		}
		for _, recovery := range reg.Recoveries {
			if recovery.Name == r.Name {
				return nil, DesktopError("PLUGIN_RECOVERY_REQUIRED", "conflict")
			}
		}
		for _, installed := range reg.States {
			if installed.Name == r.Name {
				return nil, DesktopError("PLUGIN_ALREADY_INSTALLED", "conflict")
			}
		}
	} else {
		_, state, err = s.desktopBasis(r)
		if err != nil {
			return nil, err
		}
	}
	if r.Action == "desktop_env_set" || r.Action == "desktop_env_unset" {
		return s.desktopEnvironment(ctx, r, state)
	}
	check := func() error { _, _, err := s.desktopBasis(r); return err }
	var changed pluginruntime.ChangeResult
	switch r.Action {
	case "desktop_install_candidate":
		candidate, takeErr := s.takeDesktopCandidate(r.CandidateID, "install", r.Name, "")
		if takeErr != nil {
			return nil, takeErr
		}
		changed, err = s.desktopInstallPreparedCandidate(ctx, candidate)
	case "desktop_update_candidate":
		candidate, takeErr := s.takeDesktopCandidate(r.CandidateID, "update", r.Name, r.ExpectedGeneration)
		if takeErr != nil {
			return nil, takeErr
		}
		changed, err = s.desktopUpdatePreparedCandidate(ctx, candidate, r.ExpectedGeneration)
	case "desktop_set_enabled":
		if r.Enabled == nil {
			return nil, DesktopError("INVALID_PLUGIN_REQUEST", "validation")
		}
		changed, err = s.manager.SetEnabledWithLifecycleChecked(ctx, r.Name, *r.Enabled, func(target pluginruntime.State) error {
			if target.Enabled {
				if err := s.desktopRequiredEnvironment(target); err != nil {
					return err
				}
			}
			if !target.Enabled {
				return s.reconcileMCPWithGrantPolicy(target.Name, "", nil, true)
			}
			return s.reconcileMCPState(target)
		}, check)
	case "desktop_remove_keep", "desktop_remove_purge":
		policy := "keep"
		if r.Action == "desktop_remove_purge" {
			policy = "purge"
		}
		changed, err = s.manager.RemoveWithLifecycleChecked(ctx, r.Name, policy, pluginruntime.RemoveLifecycle{
			BeforeDelete: func(pluginruntime.State) error {
				return s.reconcileMCPWithGrantPolicy(r.Name, "", nil, policy == "keep")
			},
			Restore: func(state pluginruntime.State) error { return s.reconcileMCPState(state) },
			Purge:   s.desktopPurgeOwnedState,
		}, check)
	default:
		return nil, DesktopError("INVALID_PLUGIN_REQUEST", "validation")
	}
	runtimeImpact := "applied"
	if r.Action == "desktop_install_candidate" {
		runtimeImpact = "installed_disabled"
	}
	result := Result{"action": r.Action, "name": r.Name, "completed": err == nil, "persisted": err == nil, "runtime_applied": err == nil, "recovery_required": err != nil, "runtime_impact": runtimeImpact, "changed": changed.Changed}
	if strings.HasPrefix(r.Action, "desktop_remove_") {
		result["data_policy"] = "keep"
		result["data_preserved"] = true
		if r.Action == "desktop_remove_purge" {
			result["data_policy"] = "purge"
			result["data_preserved"] = false
		}
	}
	latest, readErr := s.manager.Store().DesktopRegistrySnapshot()
	if readErr == nil {
		result["registry_revision"] = latest.Revision
		result["snapshot"] = desktopSnapshot(latest)
	}
	if err != nil {
		safe := safeDesktopFailure(err)
		// A lifecycle error may follow runtime changes or the purge commit point.
		// Never claim unchanged state; the journal retains this recovery result.
		result["runtime_impact"] = "unknown"
		result["safe_error"] = safe
		if safe.Category == "validation" || safe.Category == "conflict" {
			return nil, safe
		}
		result["outcome_unknown"] = true
		if readErr == nil && r.Action == "desktop_remove_purge" {
			for _, recovery := range latest.Recoveries {
				if recovery.Name == r.Name && recovery.State == "cleanup_required" {
					result["persisted"] = true
					result["outcome_unknown"] = false
					result["runtime_impact"] = "cleanup_pending"
				}
			}
		}
	}
	if readErr != nil {
		result["recovery_required"] = true
		result["outcome_unknown"] = true
		result["safe_error"] = DesktopError("PLUGIN_REGISTRY_READ_FAILED", "unavailable")
	}
	return result, nil
}

func (s *Service) desktopInstallPreparedCandidate(ctx context.Context, candidate *pluginruntime.PreparedCandidate) (pluginruntime.ChangeResult, error) {
	result, err := s.manager.InstallPreparedCandidate(ctx, candidate, false)
	if err != nil {
		return pluginruntime.ChangeResult{}, pluginToolError(err)
	}
	if !result.Changed {
		return result, nil
	}
	if err := s.reconcileMCPActivation(result.Name); err != nil {
		abortErr := s.manager.AbortActivation(ctx, result.Name)
		reconcileErr := s.ReconcileMCP()
		return pluginruntime.ChangeResult{}, toolcore.NewErrorCause(
			"PLUGIN_RUNTIME_ACTIVATION_FAILED",
			"Plugin install candidate could not activate its runtime; installation was aborted",
			"runtime",
			nil,
			errors.Join(err, abortErr, reconcileErr),
		)
	}
	if err := s.manager.FinalizeActivation(result.Name); err != nil {
		deactivateErr := s.reconcileMCPExcluding(result.Name)
		abortErr := s.manager.AbortActivation(ctx, result.Name)
		reconcileErr := s.ReconcileMCP()
		return pluginruntime.ChangeResult{}, toolcore.NewErrorCause(
			"PLUGIN_INSTALL_FINALIZE_FAILED",
			"Plugin install could not be finalized",
			"runtime",
			nil,
			errors.Join(err, deactivateErr, abortErr, reconcileErr),
		)
	}
	return result, nil
}

func (s *Service) desktopUpdatePreparedCandidate(ctx context.Context, candidate *pluginruntime.PreparedCandidate, expectedGeneration string) (pluginruntime.ChangeResult, error) {
	deactivated := false
	var deactivationErr error
	result, err := s.manager.UpdatePreparedCandidate(ctx, candidate, func(state pluginruntime.State) error {
		if pluginruntime.DesktopGeneration(state) != expectedGeneration {
			return DesktopError("PLUGIN_GENERATION_CONFLICT", "conflict")
		}
		if !state.Enabled {
			return nil
		}
		if reconcileErr := s.reconcileMCPExcluding(state.Name); reconcileErr != nil {
			deactivationErr = toolcore.NewErrorCause(
				"PLUGIN_RUNTIME_DEACTIVATION_FAILED",
				"Plugin runtime could not stop before update",
				"runtime",
				nil,
				reconcileErr,
			)
			return deactivationErr
		}
		deactivated = true
		return nil
	})
	if err != nil {
		if deactivationErr != nil {
			return pluginruntime.ChangeResult{}, deactivationErr
		}
		if deactivated {
			reconcileErr := s.ReconcileMCP()
			if reconcileErr != nil {
				return pluginruntime.ChangeResult{}, toolcore.NewErrorCause(
					"PLUGIN_UPDATE_FAILED",
					"Plugin update failed and the previous runtime could not be fully restored",
					"runtime",
					nil,
					errors.Join(err, reconcileErr),
				)
			}
		}
		return pluginruntime.ChangeResult{}, pluginToolError(err)
	}
	if err := s.reconcileMCPActivation(result.Name); err != nil {
		restoreErr := s.manager.AbortActivation(ctx, result.Name)
		reconcileErr := s.ReconcileMCP()
		return pluginruntime.ChangeResult{}, toolcore.NewErrorCause(
			"PLUGIN_RUNTIME_ACTIVATION_FAILED",
			"Plugin update could not activate its runtime; the previous Plugin state was restored",
			"runtime",
			nil,
			errors.Join(err, restoreErr, reconcileErr),
		)
	}
	if err := s.manager.FinalizeActivation(result.Name); err != nil {
		deactivateErr := s.reconcileMCPExcluding(result.Name)
		restoreErr := s.manager.AbortActivation(ctx, result.Name)
		reconcileErr := s.ReconcileMCP()
		return pluginruntime.ChangeResult{}, toolcore.NewErrorCause(
			"PLUGIN_UPDATE_FINALIZE_FAILED",
			"Plugin update could not be finalized; the previous Plugin state was restored",
			"runtime",
			nil,
			errors.Join(err, deactivateErr, restoreErr, reconcileErr),
		)
	}
	return result, nil
}

func safeDesktopFailure(err error) *toolcore.ToolError {
	var typed *toolcore.ToolError
	if errors.As(err, &typed) {
		switch typed.Code {
		case "PLUGIN_REGISTRY_CONFLICT", "PLUGIN_GENERATION_CONFLICT", "PLUGIN_RECOVERY_REQUIRED", "PLUGIN_ENV_CONFLICT", "PLUGIN_CREDENTIAL_REQUIRED", "PLUGIN_ENV_OWNERSHIP_INVALID", "PLUGIN_CANDIDATE_INVALID", "PLUGIN_CANDIDATE_UNAVAILABLE", "PLUGIN_CANDIDATE_TARGET_MISMATCH", "PLUGIN_ALREADY_INSTALLED", "PLUGIN_NOT_FOUND":
			return DesktopError(typed.Code, typed.Category)
		}
	}
	return DesktopError("PLUGIN_OPERATION_FAILED", "operation")
}

func (s *Service) desktopComponent(state pluginruntime.State, name string) (pluginruntime.MCPComponent, error) {
	// Persisted component ownership is the authority; package declarations only
	// supply optional binding names after content identity has been verified.
	for _, component := range state.Components.MCP {
		if component.Name == name {
			canonical := pluginruntime.RuntimeMCPName(state.Name, name)
			if component.StorageKey != canonical || component.RuntimeName != canonical {
				break
			}
			owned := false
			for _, key := range state.MCPStorageKeys {
				if key == canonical {
					owned = true
				}
			}
			if !owned {
				break
			}
			root, err := s.manager.Store().PackagePath(state.Name, state.Version)
			if err != nil {
				return pluginruntime.MCPComponent{}, DesktopError("PLUGIN_PACKAGE_UNAVAILABLE", "unavailable")
			}
			pkg, err := pluginruntime.LoadPackage(root)
			if err != nil || pkg.Manifest.Name != state.Name || pkg.Manifest.Version != state.Version || pkg.PackageDigest != state.PackageDigest {
				return pluginruntime.MCPComponent{}, DesktopError("PLUGIN_PACKAGE_UNAVAILABLE", "unavailable")
			}
			for _, declared := range pkg.Components.MCP {
				if declared.Name == name && declared.StorageKey == canonical && declared.RuntimeName == canonical {
					return declared, nil
				}
			}
		}
	}
	return pluginruntime.MCPComponent{}, DesktopError("PLUGIN_ENV_OWNERSHIP_INVALID", "validation")
}

func envKeys(component pluginruntime.MCPComponent, snapshot envstore.Snapshot) []envstore.Entry {
	keys := map[string]bool{}
	for _, key := range component.RequiredEnv {
		keys[key] = false
	}
	for _, key := range component.EnvBindings {
		keys[key] = false
	}
	for _, key := range component.HeaderEnv {
		keys[key] = false
	}
	for _, entry := range snapshot.Entries {
		keys[entry.Key] = entry.Configured
	}
	result := []envstore.Entry{}
	for key, configured := range keys {
		if envstore.ValidateKey(key) == nil && !config.IsReservedPluginEnvironmentKey(key) {
			result = append(result, envstore.Entry{Key: key, Configured: configured})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}

func (s *Service) desktopEnvironment(ctx context.Context, r DesktopManageRequest, state pluginruntime.State) (Result, error) {
	release, err := s.manager.Store().AcquireBinding(ctx, state.Name)
	if err != nil {
		return nil, DesktopError("PLUGIN_CORE_UNAVAILABLE", "unavailable")
	}
	defer release()
	if r.Action != "desktop_env_snapshot" {
		_, state, err = s.desktopBasis(r)
		if err != nil {
			return nil, err
		}
	} else {
		current, loadErr := s.manager.Store().Load(state.Name)
		if loadErr != nil || pluginruntime.DesktopGeneration(current) != pluginruntime.DesktopGeneration(state) {
			return nil, DesktopError("PLUGIN_GENERATION_CONFLICT", "conflict")
		}
		state = current
	}
	component, err := s.desktopComponent(state, r.Component)
	if err != nil {
		return nil, err
	}
	if s.envs == nil {
		return nil, DesktopError("PLUGIN_ENV_UNAVAILABLE", "unavailable")
	}
	if s.mcpClients == nil {
		return nil, DesktopError("PLUGIN_ENV_UNAVAILABLE", "unavailable")
	}
	var result Result
	err = s.mcpClients.WithPluginEnvironmentScope(state.Name, component.RuntimeName, component.StorageKey, func() error {
		var err error
		result, err = s.desktopScopedEnvironment(r, state, component)
		return err
	})
	if err != nil {
		var mcpErr *mcpclient.Error
		if errors.As(err, &mcpErr) {
			if mcpErr.Code != "MCP_SERVER_COLLISION" {
				return nil, DesktopError("PLUGIN_ENV_UNAVAILABLE", "unavailable")
			}
			return nil, DesktopError("PLUGIN_ENV_OWNERSHIP_INVALID", "validation")
		}
		return nil, err
	}
	return result, nil
}

func (s *Service) desktopScopedEnvironment(r DesktopManageRequest, state pluginruntime.State, component pluginruntime.MCPComponent) (Result, error) {
	scope := envstore.Scope{Kind: envstore.ScopeMCP, Name: component.StorageKey}
	snapshot, err := s.envs.Snapshot(scope)
	if err != nil {
		return nil, DesktopError("PLUGIN_ENV_UNAVAILABLE", "unavailable")
	}
	if r.Action != "desktop_env_snapshot" {
		allowed := false
		for _, entry := range envKeys(component, snapshot) {
			if entry.Key == r.Key {
				allowed = true
			}
		}
		if !allowed {
			return nil, DesktopError("PLUGIN_ENV_OWNERSHIP_INVALID", "validation")
		}
		switch r.Action {
		case "desktop_env_set":
			if r.Value == nil {
				return nil, DesktopError("INVALID_PLUGIN_REQUEST", "validation")
			}
			snapshot, err = s.envs.SetChecked(scope, r.Key, *r.Value, r.ExpectedEnvRevision)
		case "desktop_env_unset":
			_, snapshot, err = s.envs.UnsetChecked(scope, r.Key, r.ExpectedEnvRevision)
		}
		if errors.Is(err, envstore.ErrRevisionConflict) {
			return nil, DesktopError("PLUGIN_ENV_CONFLICT", "conflict")
		}
		if err != nil {
			return nil, DesktopError("PLUGIN_ENV_UNAVAILABLE", "unavailable")
		}
	}
	result := Result{"action": r.Action, "name": state.Name, "component": component.Name, "generation": pluginruntime.DesktopGeneration(state), "env_revision": snapshot.Revision, "items": envKeys(component, snapshot)}
	if r.Action != "desktop_env_snapshot" {
		result["completed"] = true
		result["persisted"] = true
		result["runtime_applied"] = false
		result["recovery_required"] = false
		result["runtime_impact"] = "next_connection"
		result["reconnect_required"] = state.Enabled
	}
	return result, nil
}

func (s *Service) desktopRequiredEnvironment(state pluginruntime.State) error {
	for _, component := range state.Components.MCP {
		declared, err := s.desktopComponent(state, component.Name)
		if err != nil {
			return err
		}
		if len(declared.RequiredEnv) == 0 {
			continue
		}
		if s.envs == nil {
			return DesktopError("PLUGIN_ENV_UNAVAILABLE", "unavailable")
		}
		// Plugin credentials are scoped; do not inherit unrelated host secrets.
		values, err := s.envs.Load(envstore.Scope{Kind: envstore.ScopeMCP, Name: component.StorageKey})
		if err != nil {
			return DesktopError("PLUGIN_ENV_UNAVAILABLE", "unavailable")
		}
		for _, key := range declared.RequiredEnv {
			if values[key] == "" {
				return DesktopError("PLUGIN_CREDENTIAL_REQUIRED", "validation")
			}
		}
	}
	return nil
}

func (s *Service) desktopPurgeOwnedState(ownership pluginruntime.PurgeOwnership) error {
	var result error
	for _, key := range ownership.MCPStorageKeys {
		err := s.mcpClients.WithPluginEnvironmentScope(ownership.Name, key, key, func() error {
			return errors.Join(s.purgeMCPEnvironment([]string{key}), s.mcpClients.RemoveOAuthGrant(key))
		})
		result = errors.Join(result, err)
	}
	// Skill settings/data have their own explicit Plugin scope.
	result = errors.Join(result, s.purgePluginOwnedState(pluginruntime.PurgeOwnership{Name: ownership.Name}))
	return result
}
