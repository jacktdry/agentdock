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

const browserCatalogEnv = "AGENTDOCK_BROWSER_CATALOG_JSON"

func browserCatalogFromEnv() (browserpolicy.CatalogDefinition, error) {
	value := strings.TrimSpace(os.Getenv(browserCatalogEnv))
	if value == "" {
		return browserpolicy.CatalogDefinition{}, nil
	}
	if !strings.HasPrefix(value, "{") {
		return browserpolicy.CatalogDefinition{}, fmt.Errorf("%s: expected JSON object", browserCatalogEnv)
	}
	var def browserpolicy.CatalogDefinition
	decoder := json.NewDecoder(bytes.NewBufferString(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&def); err != nil {
		return def, fmt.Errorf("%s: %w", browserCatalogEnv, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return def, fmt.Errorf("%s: trailing JSON data", browserCatalogEnv)
	}
	catalog, err := normalizeBrowserCatalog(def)
	if err != nil {
		return def, err
	}
	return catalog.Definition(), nil
}

func normalizeBrowserCatalog(def browserpolicy.CatalogDefinition) (browserpolicy.Catalog, error) {
	catalog, err := browserpolicy.NewCatalog(def)
	if err != nil {
		return browserpolicy.Catalog{}, fmt.Errorf("%s: %w", browserCatalogEnv, err)
	}
	return catalog, nil
}

func validateBrowserPolicyCatalog(policies []browserpolicy.WorkspaceRootPolicy, catalog browserpolicy.Catalog) error {
	if err := browserpolicy.ValidateWorkspacePolicies(policies, catalog); err != nil {
		return fmt.Errorf("%s: %w", browserWorkspacePoliciesEnv, err)
	}
	return nil
}

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
