package desktopapi

import (
	"context"
	"errors"
	"github.com/uvwt/agentdock/internal/desktopruntime"
	"time"
)

type BasicSettings struct {
	Port          int    `json:"port"`
	LogLevel      string `json:"logLevel"`
	CoreAutostart bool   `json:"coreAutostart"`
}
type BasicSettingsResult struct {
	Settings             BasicSettings `json:"settings"`
	CoreAutostartMutable bool          `json:"coreAutostartMutable"`
	Error                *APIError     `json:"error,omitempty"`
}
type BasicSettingsSaveResult struct {
	Completed bool      `json:"completed"`
	Error     *APIError `json:"error,omitempty"`
}
type BasicSettingsService struct {
	runtimeRoot string
	rootError   error
}

func NewBasicSettingsService(root string) *BasicSettingsService {
	root, err := resolveRuntimeRoot(root)
	return &BasicSettingsService{runtimeRoot: root, rootError: err}
}
func (s *BasicSettingsService) Read(ctx context.Context) BasicSettingsResult {
	if s.rootError != nil {
		return BasicSettingsResult{Error: safeServiceError("settings_root_unavailable", s.rootError)}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	settings, err := desktopruntime.ReadBasicSettings(ctx, s.runtimeRoot)
	if err != nil {
		return BasicSettingsResult{Error: safeContextServiceError(ctx, "settings_read_failed", err)}
	}
	return BasicSettingsResult{
		Settings:             BasicSettings{Port: settings.Port, LogLevel: settings.LogLevel, CoreAutostart: settings.CoreAutostart},
		CoreAutostartMutable: desktopruntime.BasicAutostartMutable(),
	}
}
func (s *BasicSettingsService) Save(ctx context.Context, settings BasicSettings) BasicSettingsSaveResult {
	if s.rootError != nil {
		return BasicSettingsSaveResult{Error: safeServiceError("settings_root_unavailable", s.rootError)}
	}
	operationCtx, finish, err := beginRuntimeMutation(ctx, s.runtimeRoot)
	if err != nil {
		return BasicSettingsSaveResult{Error: safeContextServiceError(ctx, "settings_mutation_busy", err)}
	}
	defer finish()
	err = desktopruntime.UpdateBasicSettings(operationCtx, s.runtimeRoot, desktopruntime.BasicSettings{Port: settings.Port, LogLevel: settings.LogLevel, CoreAutostart: settings.CoreAutostart})
	if err != nil {
		return BasicSettingsSaveResult{Error: safeContextServiceError(operationCtx, "settings_save_failed", err)}
	}
	return BasicSettingsSaveResult{Completed: true}
}

// Platform command output and parser errors can contain secrets. Never forward
// raw errors or stderr through the shared services.
func safeServiceError(code string, err error) *APIError {
	category, message := ErrorCategoryOperation, "Desktop operation failed"
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		category, message = ErrorCategoryTimeout, "Desktop operation timed out or was canceled"
	case errors.Is(err, desktopruntime.ErrBasicSettingsInvalid):
		category, message = ErrorCategoryValidation, "Port must be 1–65535 and log level must be debug, info, warn or error"
	case errors.Is(err, desktopruntime.ErrBasicSettingsUnavailable):
		category, message = ErrorCategoryUnavailable, "This setting requires the native desktop service adapter"
	}
	if code == "connection_root_unavailable" || code == "settings_root_unavailable" || code == "diagnostics_root_unavailable" || code == "runtime_root_unavailable" || code == "update_root_unavailable" {
		category, message = ErrorCategoryUnavailable, "Runtime root unavailable"
	}
	return NewError(code, message, category, category == ErrorCategoryTimeout, nil)
}

func safeContextServiceError(ctx context.Context, code string, err error) *APIError {
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return safeServiceError(code, err)
}
