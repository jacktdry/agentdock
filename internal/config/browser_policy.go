package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

// Workspace mappings are stored only in this slice; runtime routing is deferred.
const browserWorkspacePoliciesEnv = "AGENTDOCK_BROWSER_WORKSPACE_POLICIES_JSON"

func browserWorkspacePoliciesFromEnv() ([]browserpolicy.WorkspaceRootPolicy, error) {
	value := strings.TrimSpace(os.Getenv(browserWorkspacePoliciesEnv))
	if value == "" {
		return nil, nil
	}
	var mappings []browserpolicy.WorkspaceRootPolicy
	decoder := json.NewDecoder(bytes.NewBufferString(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&mappings); err != nil {
		return nil, fmt.Errorf("%s: %w", browserWorkspacePoliciesEnv, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("%s: trailing JSON data", browserWorkspacePoliciesEnv)
	}
	return normalizeBrowserWorkspacePolicies(mappings)
}

func normalizeBrowserWorkspacePolicies(mappings []browserpolicy.WorkspaceRootPolicy) ([]browserpolicy.WorkspaceRootPolicy, error) {
	result, err := browserpolicy.NormalizeWorkspacePolicies(mappings)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", browserWorkspacePoliciesEnv, err)
	}
	return result, nil
}
