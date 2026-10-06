//go:build windows

package desktopruntime

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"

	"github.com/uvwt/agentdock/internal/fs/atomicfile"
)

func platformReadACPSettings(_ context.Context, runtimeRoot string) (ACPSettings, error) {
	manifest, root, err := loadDesktopManifest(runtimeRoot)
	if err != nil {
		return ACPSettings{}, err
	}
	settings, err := loadControlPanelSettings(root, manifest.Port)
	if err != nil {
		return ACPSettings{}, err
	}
	return acpSettingsFromProfiles(settings.ACPEnabled, settings.ACPProfiles, settings.ACPDefaultProfile), nil
}

func platformLoadACPConfiguration(_ context.Context, runtimeRoot string) (acpConfigurationDocument, error) {
	_, root, err := loadDesktopManifest(runtimeRoot)
	if err != nil {
		return acpConfigurationDocument{}, err
	}
	snapshot, err := snapshotFile(filepath.Join(root, "control-panel-settings.json"))
	if err != nil {
		return acpConfigurationDocument{}, err
	}
	fields := map[string]json.RawMessage{}
	if snapshot.exists {
		if err := json.Unmarshal(snapshot.data, &fields); err != nil || fields == nil {
			return acpConfigurationDocument{}, ErrACPSettingsInvalid
		}
	}
	var enabled bool
	var defaultProfile, agent, command string
	var args []string
	for key, target := range map[string]any{"acp_enabled": &enabled, "acp_default_profile": &defaultProfile, "acp_agent": &agent, "acp_command": &command, "acp_args": &args} {
		if raw, ok := fields[key]; ok {
			if err := json.Unmarshal(raw, target); err != nil {
				return acpConfigurationDocument{}, ErrACPSettingsInvalid
			}
		}
	}
	argsJSON, _ := json.Marshal(args)
	values := map[string]string{"AGENTDOCK_ACP_ENABLED": strconv.FormatBool(enabled), "AGENTDOCK_ACP_DEFAULT_PROFILE": defaultProfile, "AGENTDOCK_ACP_PROFILES_JSON": string(fields["acp_profiles"]), "AGENTDOCK_ACP_AGENT": agent, "AGENTDOCK_ACP_COMMAND": command, "AGENTDOCK_ACP_ARGS_JSON": string(argsJSON)}
	doc, err := acpDocumentFromEnvironment(values)
	if err != nil {
		return acpConfigurationDocument{}, err
	}
	doc.revision = acpConfigurationRevision(snapshot.data)
	doc.persist = func(_ context.Context, settings ACPSettings, profiles []json.RawMessage) (string, error) {
		fields["acp_enabled"], _ = json.Marshal(settings.Enabled)
		fields["acp_default_profile"], _ = json.Marshal(settings.DefaultProfile)
		fields["acp_profiles"], _ = json.Marshal(profiles)
		data, err := json.MarshalIndent(fields, "", "  ")
		if err != nil {
			return "", err
		}
		data = append(data, '\n')
		if err := atomicfile.Write(snapshot.path, data, 0o600); err != nil {
			return "", err
		}
		return acpConfigurationRevision(data), nil
	}
	return doc, nil
}
