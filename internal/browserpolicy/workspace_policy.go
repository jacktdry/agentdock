package browserpolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type WorkspaceRootPolicy struct {
	Root   string             `json:"root"`
	Policy BrowserRoutePolicy `json:"policy"`
}

// CanonicalWorkspaceRoot requires an existing absolute directory. Unresolvable
// paths fail closed; callers must not substitute a default classification.
func CanonicalWorkspaceRoot(root string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("workspace root must be absolute")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace root must be a directory")
	}
	return filepath.Clean(canonical), nil
}

func rootContains(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func NormalizeWorkspacePolicies(mappings []WorkspaceRootPolicy) ([]WorkspaceRootPolicy, error) {
	result := make([]WorkspaceRootPolicy, len(mappings))
	for i, m := range mappings {
		if err := ValidateRoutePolicy(m.Policy); err != nil {
			return nil, fmt.Errorf("workspace policy %d: %w", i, err)
		}
		root, err := CanonicalWorkspaceRoot(m.Root)
		if err != nil {
			return nil, fmt.Errorf("workspace policy %d: %w", i, err)
		}
		result[i] = WorkspaceRootPolicy{Root: root, Policy: m.Policy}
		for _, previous := range result[:i] {
			if root == previous.Root || ((rootContains(root, previous.Root) || rootContains(previous.Root, root)) && m.Policy != previous.Policy) {
				return nil, fmt.Errorf("duplicate or conflicting overlapping workspace policies")
			}
		}
	}
	return result, nil
}

// ClassifyWorkspace checks all mappings before applying the default. Nested
// mappings cannot weaken a parent's required route; conflicting overlaps fail.
func ClassifyWorkspace(root string, mappings []WorkspaceRootPolicy) (string, BrowserRoutePolicy, error) {
	normalized, err := NormalizeWorkspacePolicies(mappings)
	if err != nil {
		return "", BrowserRoutePolicy{}, err
	}
	canonical, err := CanonicalWorkspaceRoot(root)
	if err != nil {
		return "", BrowserRoutePolicy{}, err
	}
	policy := BrowserRoutePolicy{Class: WorkspaceDefault, Route: RouteManaged}
	for _, m := range normalized {
		if rootContains(m.Root, canonical) {
			policy = m.Policy
		}
	}
	return canonical, policy, nil
}
