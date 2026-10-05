package file

import (
	pathpkg "path"
	"strings"
)

// DrvFS may be case insensitive. Conservatively fold Windows-drive-backed
// paths only; Linux-native paths retain case-sensitive semantics.
func wslProtectionKey(value string) string {
	value = pathpkg.Clean(value)
	if len(value) >= 6 && strings.EqualFold(value[:5], "/mnt/") && ((value[5] >= 'a' && value[5] <= 'z') || (value[5] >= 'A' && value[5] <= 'Z')) && (len(value) == 6 || value[6] == '/') {
		return strings.ToLower(value)
	}
	return value
}
func wslPathWithin(root, target string) bool {
	root = wslProtectionKey(root)
	target = wslProtectionKey(target)
	if root == target {
		return true
	}
	return strings.HasPrefix(target, strings.TrimSuffix(root, "/")+"/")
}
func wslRelativePath(root, target string) string {
	root = pathpkg.Clean(root)
	target = pathpkg.Clean(target)
	if !wslPathWithin(root, target) || wslProtectionKey(root) == wslProtectionKey(target) {
		return ""
	}
	return target[len(strings.TrimSuffix(root, "/"))+1:]
}
