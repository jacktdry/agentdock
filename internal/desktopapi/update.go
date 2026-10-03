package desktopapi

import (
	"context"
	"github.com/uvwt/agentdock/internal/selfupdate"
	"time"
)

type UpdateStatus struct {
	CurrentVersion         string `json:"currentVersion"`
	LatestVersion          string `json:"latestVersion"`
	DesktopCurrentVersion  string `json:"desktopCurrentVersion,omitempty"`
	UpdateAvailable        bool   `json:"updateAvailable"`
	DesktopUpdateAvailable bool   `json:"desktopUpdateAvailable"`
}
type UpdateCheckResult struct {
	Status UpdateStatus `json:"status"`
	Error  *APIError    `json:"error,omitempty"`
}
type UpdateService struct {
	runtimeRoot string
	rootError   error
	check       func(context.Context, string) (selfupdate.CheckResult, error)
}

func NewUpdateService(root string) *UpdateService {
	root, err := resolveRuntimeRoot(root)
	return &UpdateService{runtimeRoot: root, rootError: err, check: selfupdate.CheckForRuntime}
}
func (s *UpdateService) Check(ctx context.Context) UpdateCheckResult {
	if s.rootError != nil {
		return UpdateCheckResult{Error: safeServiceError("update_root_unavailable", s.rootError)}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := s.check(ctx, s.runtimeRoot)
	if err != nil {
		return UpdateCheckResult{Error: safeContextServiceError(ctx, "update_check_failed", err)}
	}
	return UpdateCheckResult{Status: UpdateStatus{CurrentVersion: result.CurrentVersion, LatestVersion: result.LatestVersion, DesktopCurrentVersion: result.DesktopCurrentVersion, UpdateAvailable: result.UpdateAvailable, DesktopUpdateAvailable: result.DesktopUpdateAvailable}}
}
