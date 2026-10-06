package desktopruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agentconfig "github.com/uvwt/agentdock/internal/config"
)

var ErrACPSettingsConflict = errors.New("ACP settings revision conflict")
var ErrACPSettingsInvalid = errors.New("invalid ACP settings")

type ACPConfiguration struct {
	Revision string `json:"revision"`
	ACPSettings
}

// The document and its hidden fields stay inside the persistence boundary.
type acpConfigurationDocument struct {
	settings ACPSettings
	profiles []json.RawMessage
	revision string
	persist  func(context.Context, ACPSettings, []json.RawMessage) (string, error)
}

func acpConfigurationRevision(data []byte) string {
	sum := sha256.Sum256(append([]byte("acp-settings-v1\x00"), data...))
	return hex.EncodeToString(sum[:])
}

func ReadACPConfiguration(ctx context.Context, root string) (ACPConfiguration, error) {
	if ctx == nil {
		return ACPConfiguration{}, errors.New("context is required")
	}
	if err := ValidateNextSettingsIdentity(ctx, root); err != nil {
		return ACPConfiguration{}, err
	}
	basicSettingsMu.Lock()
	defer basicSettingsMu.Unlock()
	if err := ctx.Err(); err != nil {
		return ACPConfiguration{}, err
	}
	doc, err := platformLoadACPConfiguration(ctx, root)
	if err != nil {
		return ACPConfiguration{}, err
	}
	return ACPConfiguration{Revision: doc.revision, ACPSettings: doc.settings}, nil
}

// SaveACPSettings is persistence-only: running Core/session state is untouched.
// Coordinate with other Desktop domains and processes before reading revision.
func SaveACPSettings(ctx context.Context, root, expectedRevision string, requested ACPSettings) (ACPConfiguration, error) {
	if ctx == nil {
		return ACPConfiguration{}, errors.New("context is required")
	}
	if err := ValidateNextSettingsIdentity(ctx, root); err != nil {
		return ACPConfiguration{}, err
	}
	release, err := AcquireDesktopMutation(ctx, root)
	if err != nil {
		return ACPConfiguration{}, err
	}
	defer release()
	basicSettingsMu.Lock()
	defer basicSettingsMu.Unlock()
	if err := ValidateNextSettingsIdentity(ctx, root); err != nil {
		return ACPConfiguration{}, err
	}
	doc, err := platformLoadACPConfiguration(ctx, root)
	if err != nil {
		return ACPConfiguration{}, err
	}
	if expectedRevision == "" || expectedRevision != doc.revision {
		return ACPConfiguration{}, ErrACPSettingsConflict
	}
	candidate, raw, err := mergeACPConfiguration(doc, requested)
	if err != nil {
		return ACPConfiguration{}, ErrACPSettingsInvalid
	}
	if err := ctx.Err(); err != nil {
		return ACPConfiguration{}, err
	}
	revision, err := doc.persist(ctx, candidate, raw)
	if err != nil {
		return ACPConfiguration{}, err
	}
	return ACPConfiguration{Revision: revision, ACPSettings: candidate}, nil
}

func mergeACPConfiguration(doc acpConfigurationDocument, requested ACPSettings) (ACPSettings, []json.RawMessage, error) {
	existing := map[string]map[string]json.RawMessage{}
	for _, raw := range doc.profiles {
		var fields map[string]json.RawMessage
		var p agentconfig.ACPProfile
		if json.Unmarshal(raw, &fields) != nil || fields == nil || json.Unmarshal(raw, &p) != nil {
			return ACPSettings{}, nil, ErrACPSettingsInvalid
		}
		existing[strings.TrimSpace(p.ID)] = fields
	}
	used := map[string]bool{}
	for _, p := range requested.Profiles {
		used[strings.TrimSpace(p.ID)] = true
	}
	var profiles []agentconfig.ACPProfile
	rawProfiles := make([]json.RawMessage, 0, len(requested.Profiles))
	for _, input := range requested.Profiles {
		input.ID = strings.TrimSpace(input.ID)
		input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
		input.Command = strings.TrimSpace(input.Command)
		if input.ID == "" {
			if input.Kind == "custom" {
				for n := 1; ; n++ {
					id := fmt.Sprintf("custom-%d", n)
					if !used[id] && existing[id] == nil {
						input.ID = id
						break
					}
				}
			} else {
				input.ID = input.Kind
			}
			used[input.ID] = true
		}
		fields := existing[input.ID]
		if fields == nil {
			fields = map[string]json.RawMessage{}
		} else {
			var oldKind string
			_ = json.Unmarshal(fields["kind"], &oldKind)
			if strings.ToLower(strings.TrimSpace(oldKind)) != input.Kind {
				return ACPSettings{}, nil, ErrACPSettingsInvalid
			}
		}
		for key, value := range map[string]any{"id": input.ID, "display_name": input.DisplayName, "kind": input.Kind, "command": input.Command, "enabled": input.Enabled} {
			fields[key], _ = json.Marshal(value)
		}
		if input.PreserveArgs {
			if existing[input.ID] == nil {
				return ACPSettings{}, nil, ErrACPSettingsInvalid
			}
		} else {
			fields["args"], _ = json.Marshal(input.Args)
		}
		raw, err := json.Marshal(fields)
		if err != nil {
			return ACPSettings{}, nil, err
		}
		var p agentconfig.ACPProfile
		if err := json.Unmarshal(raw, &p); err != nil {
			return ACPSettings{}, nil, err
		}
		profiles = append(profiles, p)
		rawProfiles = append(rawProfiles, raw)
	}
	requested.DefaultProfile = strings.TrimSpace(requested.DefaultProfile)
	if err := agentconfig.ValidateACPSettings(requested.Enabled, requested.DefaultProfile, profiles); err != nil {
		return ACPSettings{}, nil, err
	}
	return acpSettingsFromProfiles(requested.Enabled, profiles, requested.DefaultProfile), rawProfiles, nil
}

func acpDocumentFromEnvironment(values map[string]string) (acpConfigurationDocument, error) {
	settings, err := acpSettingsFromEnvironment(values)
	if err != nil {
		return acpConfigurationDocument{}, err
	}
	doc := acpConfigurationDocument{settings: settings}
	if raw := strings.TrimSpace(values["AGENTDOCK_ACP_PROFILES_JSON"]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &doc.profiles); err != nil {
			return acpConfigurationDocument{}, err
		}
	} else {
		for _, p := range settings.Profiles {
			profile := agentconfig.ACPProfile{ID: p.ID, DisplayName: p.DisplayName, Kind: p.Kind, Command: p.Command, Args: p.Args, Enabled: p.Enabled}
			if raw := values["AGENTDOCK_ACP_ENV_FROM_ENV_JSON"]; strings.TrimSpace(raw) != "" {
				if err := json.Unmarshal([]byte(raw), &profile.EnvFromEnv); err != nil {
					return acpConfigurationDocument{}, ErrACPSettingsInvalid
				}
			}
			raw, _ := json.Marshal(profile)
			doc.profiles = append(doc.profiles, raw)
		}
	}
	return doc, nil
}
