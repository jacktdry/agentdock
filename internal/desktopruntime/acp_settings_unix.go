//go:build darwin || linux

package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/uvwt/agentdock/internal/envstore"
)

func platformReadACPSettings(_ context.Context, runtimeRoot string) (ACPSettings, error) {
	_, _, values, err := readBasicEnvironment(runtimeRoot)
	if err != nil {
		return ACPSettings{}, errors.New("protected ACP environment could not be read")
	}
	return acpSettingsFromEnvironment(values)
}

func platformLoadACPConfiguration(_ context.Context, root string) (acpConfigurationDocument, error) {
	path, original, values, err := readBasicEnvironment(root)
	if err != nil {
		return acpConfigurationDocument{}, err
	}
	doc, err := acpDocumentFromEnvironment(values)
	if err != nil {
		return acpConfigurationDocument{}, err
	}
	doc.revision = acpConfigurationRevision(original)
	doc.persist = func(ctx context.Context, settings ACPSettings, profiles []json.RawMessage) (string, error) {
		raw, err := json.Marshal(profiles)
		if err != nil {
			return "", err
		}
		values["AGENTDOCK_ACP_ENABLED"] = strconv.FormatBool(settings.Enabled)
		values["AGENTDOCK_ACP_DEFAULT_PROFILE"] = settings.DefaultProfile
		values["AGENTDOCK_ACP_PROFILES_JSON"] = string(raw)
		updated := envstore.Marshal(values)
		if err := replaceBasicEnvironments(ctx, []basicEnvironmentChange{{path, original, updated}}, nil, nil); err != nil {
			return "", err
		}
		return acpConfigurationRevision(updated), nil
	}
	return doc, nil
}
