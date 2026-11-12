package wslfilehelper

import (
	"os"
	"path/filepath"
	"strings"
)

func controlPathKey(value string) string {
	value = filepath.ToSlash(filepath.Clean(value))
	if len(value) >= 6 && strings.EqualFold(value[:5], "/mnt/") && ((value[5] >= 'a' && value[5] <= 'z') || (value[5] >= 'A' && value[5] <= 'Z')) && (len(value) == 6 || value[6] == '/') {
		return strings.ToLower(value)
	}
	return value
}
func controlPathWithin(root, target string) bool {
	root = controlPathKey(root)
	target = controlPathKey(target)
	return root == target || strings.HasPrefix(target, strings.TrimSuffix(root, "/")+"/")
}

// Resolve existing ancestors so read/list paths through a symlink cannot alias
// protected state. Missing new-file suffixes remain attached to the real parent.
func controlCanonicalPath(value string) (string, error) {
	value = filepath.Clean(value)
	suffix := []string{}
	for {
		real, err := filepath.EvalSymlinks(value)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				real = filepath.Join(real, suffix[i])
			}
			return real, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(value)
		if parent == value {
			return value, nil
		}
		suffix = append(suffix, filepath.Base(value))
		value = parent
	}
}
func controlPathBlocked(roots []string, target string, includeAncestors bool) bool {
	if len(roots) == 0 {
		return false
	}
	canonical, err := controlCanonicalPath(target)
	if err != nil {
		return true
	}
	for _, root := range roots {
		realRoot, err := controlCanonicalPath(root)
		if err != nil {
			return true
		}
		for _, pair := range [][2]string{{root, target}, {realRoot, canonical}} {
			if controlPathWithin(pair[0], pair[1]) || (includeAncestors && controlPathWithin(pair[1], pair[0])) {
				return true
			}
		}
	}
	return false
}
