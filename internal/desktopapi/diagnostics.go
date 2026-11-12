package desktopapi

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/uvwt/agentdock/internal/desktopruntime"
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
	runtimeRoot      string
	rootError        error
	inspectDirectory func(context.Context, string, desktopruntime.NextDirectoryKind) error
	openDirectory    func(context.Context, string, desktopruntime.NextDirectoryKind) error
}

func NewDiagnosticsService(root string) *DiagnosticsService {
	root, err := resolveRuntimeRoot(root)
	return &DiagnosticsService{runtimeRoot: root, rootError: err, inspectDirectory: desktopruntime.InspectNextDirectory, openDirectory: desktopruntime.OpenNextDirectory}
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

// Directory actions accept semantic intent only; Snapshot remains compatible.
type NextDirectoryKind string

const (
	NextDirectoryLogs          NextDirectoryKind = "logs"
	NextDirectoryConfiguration NextDirectoryKind = "configuration"
)

type DirectoryCapability struct {
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason"`
}
type DiagnosticsDirectories struct {
	Logs          DirectoryCapability `json:"logs"`
	Configuration DirectoryCapability `json:"configuration"`
}
type DiagnosticsDirectoryResult struct {
	OK    bool      `json:"ok"`
	Error *APIError `json:"error,omitempty"`
}

func (s *DiagnosticsService) Directories(ctx context.Context) DiagnosticsDirectories {
	capability := func(kind NextDirectoryKind) DirectoryCapability {
		if s.rootError != nil {
			return DirectoryCapability{Reason: "unavailable"}
		}
		if err := s.inspectDirectory(ctx, s.runtimeRoot, desktopruntime.NextDirectoryKind(kind)); err != nil {
			if err == desktopruntime.ErrDirectoryRequiresNative {
				return DirectoryCapability{Reason: "requires_native"}
			}
			return DirectoryCapability{Reason: "unavailable"}
		}
		return DirectoryCapability{Enabled: true}
	}
	return DiagnosticsDirectories{Logs: capability(NextDirectoryLogs), Configuration: capability(NextDirectoryConfiguration)}
}
func (s *DiagnosticsService) OpenNextDirectory(ctx context.Context, kind NextDirectoryKind) DiagnosticsDirectoryResult {
	if kind != NextDirectoryLogs && kind != NextDirectoryConfiguration {
		return DiagnosticsDirectoryResult{Error: NewError("diagnostics_directory_invalid", "Choose a supported directory.", ErrorCategoryValidation, false, nil)}
	}
	if s.rootError != nil {
		return directoryFailure()
	}
	if err := s.openDirectory(ctx, s.runtimeRoot, desktopruntime.NextDirectoryKind(kind)); err != nil {
		return directoryFailure()
	}
	return DiagnosticsDirectoryResult{OK: true}
}
func directoryFailure() DiagnosticsDirectoryResult {
	return DiagnosticsDirectoryResult{Error: NewError("diagnostics_directory_unavailable", "Refresh directory availability and try again.", ErrorCategoryUnavailable, false, nil)}
}
