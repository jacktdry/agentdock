//go:build !darwin

package config

import "github.com/uvwt/agentdock/internal/browserpolicy"

// The private route file is a Next macOS service feature. No other platform
// silently loads an owner's local browser authority through this path.
func nextBrowserRouteFile() (browserpolicy.CatalogDefinition, []browserpolicy.WorkspaceRootPolicy, bool, error) {
	return browserpolicy.CatalogDefinition{}, nil, false, nil
}
