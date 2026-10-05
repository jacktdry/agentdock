//go:build windows

package file

import (
	pathpkg "path"
	"strings"

	"github.com/uvwt/agentdock/internal/workspace"
)

func (svc *Service) protectedWSLRoots() []string {
	if svc == nil || svc.protected == nil {
		return nil
	}
	roots := make([]string, 0)
	for _, root := range svc.protected.Roots() {
		converted, ok := workspace.WindowsPathToWSL(root)
		if !ok {
			continue
		}
		roots = append(roots, pathpkg.Clean(converted))
	}
	return roots
}

func (svc *Service) guardProtectedWSLPath(target string, includeAncestors bool) error {
	target = pathpkg.Clean(strings.TrimSpace(target))
	if target == "." || target == "" {
		return nil
	}
	for _, root := range svc.protectedWSLRoots() {
		blocked := wslPathWithin(root, target)
		if includeAncestors {
			blocked = blocked || wslPathWithin(target, root)
		}
		if blocked {
			return toolErrorDetails(
				"PROTECTED_CONTROL_PATH",
				"AgentDock control-plane paths are not accessible through WSL file tools",
				"permission",
				map[string]any{"path": target},
			)
		}
	}
	return nil
}

func (svc *Service) protectedWSLRelativeDescendants(root string) []string {
	root = pathpkg.Clean(root)
	result := make([]string, 0)
	for _, protected := range svc.protectedWSLRoots() {
		if !wslPathWithin(root, protected) {
			continue
		}
		rel := wslRelativePath(root, protected)
		if rel != "" {
			result = append(result, rel)
		}
	}
	return result
}

func wslPathWithin(root, target string) bool {
	root = pathpkg.Clean(root)
	target = pathpkg.Clean(target)
	if root == target {
		return true
	}
	prefix := strings.TrimSuffix(root, "/") + "/"
	if root == "/" {
		prefix = "/"
	}
	return strings.HasPrefix(target, prefix)
}

func wslRelativePath(root, target string) string {
	root = pathpkg.Clean(root)
	target = pathpkg.Clean(target)
	if root == target {
		return ""
	}
	prefix := strings.TrimSuffix(root, "/") + "/"
	if root == "/" {
		prefix = "/"
	}
	if !strings.HasPrefix(target, prefix) {
		return ""
	}
	return strings.TrimPrefix(target, prefix)
}
