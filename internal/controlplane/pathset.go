package controlplane

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PathSet contains canonical host paths that normal AgentDock file/media
// surfaces must not expose or mutate.
type PathSet struct {
	roots []string
}

func NewPathSet(paths ...string) (*PathSet, error) {
	roots := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, raw := range paths {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		resolved, err := canonicalPath(raw)
		if err != nil {
			return nil, err
		}
		key := comparisonKey(resolved)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		roots = append(roots, resolved)
	}
	sort.Strings(roots)
	return &PathSet{roots: roots}, nil
}

func (s *PathSet) Roots() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.roots...)
}

func (s *PathSet) Contains(target string) bool {
	if s == nil || strings.TrimSpace(target) == "" {
		return false
	}
	for _, candidate := range pathCandidates(target) {
		for _, root := range s.roots {
			if within(root, candidate) {
				return true
			}
		}
	}
	return false
}

func (s *PathSet) Intersects(target string) bool {
	if s == nil || strings.TrimSpace(target) == "" {
		return false
	}
	for _, candidate := range pathCandidates(target) {
		for _, root := range s.roots {
			if within(root, candidate) || within(candidate, root) {
				return true
			}
		}
	}
	return false
}

func (s *PathSet) Descendants(root string) []string {
	if s == nil || strings.TrimSpace(root) == "" {
		return nil
	}
	result := make([]string, 0)
	for _, protected := range s.roots {
		for _, candidate := range pathCandidates(root) {
			if within(candidate, protected) {
				result = append(result, protected)
				break
			}
		}
	}
	return result
}

// Check both lexical and resolved paths. Protecting only the canonical root
// misses macOS /var -> /private/var aliases and symlinks into a protected
// directory; dropping the lexical path would instead allow links *out* of one.
// If resolution fails, retain the existing lexical deny behavior.
func pathCandidates(raw string) []string {
	clean := filepath.Clean(raw)
	resolved, err := canonicalPath(raw)
	if err != nil || resolved == clean {
		return []string{clean}
	}
	return []string{clean, resolved}
}

func canonicalPath(raw string) (string, error) {
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve protected control path: %w", err)
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved), nil
	}
	ancestor := abs
	for {
		info, statErr := os.Stat(ancestor)
		if statErr == nil && info.IsDir() {
			break
		}
		next := filepath.Dir(ancestor)
		if next == ancestor {
			return abs, nil
		}
		ancestor = next
	}
	realAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", fmt.Errorf("resolve protected control path ancestor: %w", err)
	}
	rel, err := filepath.Rel(ancestor, abs)
	if err != nil {
		return "", fmt.Errorf("resolve protected control path suffix: %w", err)
	}
	return filepath.Clean(filepath.Join(realAncestor, rel)), nil
}

func within(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func comparisonKey(path string) string {
	clean := filepath.Clean(path)
	if filepath.Separator == '\\' {
		return strings.ToLower(clean)
	}
	return clean
}
