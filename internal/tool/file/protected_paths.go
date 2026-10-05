package file

import (
	"path/filepath"
	"strings"
)

func (svc *Service) guardProtectedPath(absPath, displayPath string, includeAncestors bool) error {
	if svc == nil || svc.protected == nil {
		return nil
	}
	blocked := svc.protected.Contains(absPath)
	if includeAncestors {
		blocked = svc.protected.Intersects(absPath)
	}
	if !blocked {
		return nil
	}
	displayPath = strings.TrimSpace(displayPath)
	if displayPath == "" {
		displayPath = filepath.Clean(absPath)
	}
	return toolErrorDetails(
		"PROTECTED_CONTROL_PATH",
		"AgentDock control-plane paths are not accessible through file tools",
		"permission",
		map[string]any{"path": displayPath},
	)
}

func (svc *Service) protectedRelativeDescendants(root string) []string {
	if svc == nil || svc.protected == nil {
		return nil
	}
	result := make([]string, 0)
	for _, protected := range svc.protected.Descendants(root) {
		rel, err := filepath.Rel(root, protected)
		if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		result = append(result, filepath.ToSlash(rel))
	}
	return result
}
