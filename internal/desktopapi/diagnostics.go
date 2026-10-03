package desktopapi

import (
	"os"
	"path/filepath"
	"runtime"
)

type DiagnosticsSnapshot struct {
	Platform                  string `json:"platform"`
	Architecture              string `json:"architecture"`
	RuntimeRoot               string `json:"runtimeRoot"`
	RuntimeDirectoryAvailable bool   `json:"runtimeDirectoryAvailable"`
	ManifestAvailable         bool   `json:"manifestAvailable"`
}
type DiagnosticsResult struct {
	Snapshot DiagnosticsSnapshot `json:"snapshot"`
	Error    *APIError           `json:"error,omitempty"`
}
type DiagnosticsService struct {
	runtimeRoot string
	rootError   error
}

func NewDiagnosticsService(root string) *DiagnosticsService {
	root, err := resolveRuntimeRoot(root)
	return &DiagnosticsService{runtimeRoot: root, rootError: err}
}
func (s *DiagnosticsService) Snapshot() DiagnosticsResult {
	if s.rootError != nil {
		return DiagnosticsResult{Error: safeServiceError("diagnostics_root_unavailable", s.rootError)}
	}
	root, err := os.Lstat(s.runtimeRoot)
	if err != nil && !os.IsNotExist(err) {
		return DiagnosticsResult{Error: safeServiceError("diagnostics_stat_failed", err)}
	}
	snapshot := DiagnosticsSnapshot{Platform: runtime.GOOS, Architecture: runtime.GOARCH, RuntimeRoot: s.runtimeRoot, RuntimeDirectoryAvailable: err == nil && root.IsDir()}
	manifestName := "desktop-runtime.json"
	if runtime.GOOS == "windows" {
		manifestName = "runtime.json"
	}
	manifest, err := os.Lstat(filepath.Join(s.runtimeRoot, manifestName))
	if err != nil && !os.IsNotExist(err) {
		return DiagnosticsResult{Error: safeServiceError("diagnostics_stat_failed", err)}
	}
	snapshot.ManifestAvailable = err == nil && manifest.Mode().IsRegular()
	return DiagnosticsResult{Snapshot: snapshot}
}
