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
	PortMutable          bool          `json:"portMutable"`
	PortDisabledReason   string        `json:"portDisabledReason,omitempty"`
	Error                *APIError     `json:"error,omitempty"`
}
type BasicSettingsSaveResult struct {
	Completed bool      `json:"completed"`
	Error     *APIError `json:"error,omitempty"`
}
type BasicSettingsService struct {
	runtimeRoot      string
	rootError        error
	read             func(context.Context, string) (desktopruntime.BasicSettings, error)
	update           func(context.Context, string, desktopruntime.BasicSettings) error
	validateIdentity func(context.Context, string) error
}

func NewBasicSettingsService(root string) *BasicSettingsService {
	root, err := resolveRuntimeRoot(root)
	return &BasicSettingsService{runtimeRoot: root, rootError: err, read: desktopruntime.ReadBasicSettings, update: desktopruntime.UpdateBasicSettings, validateIdentity: desktopruntime.ValidateNextSettingsIdentity}
}
func (s *BasicSettingsService) Read(ctx context.Context) BasicSettingsResult {
	if s.rootError != nil {
		return BasicSettingsResult{Error: safeServiceError("settings_root_unavailable", s.rootError)}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.validateIdentity(ctx, s.runtimeRoot); err != nil {
		return BasicSettingsResult{Error: connectionFailure("next_identity_unavailable")}
	}
	settings, err := s.read(ctx, s.runtimeRoot)
	if err != nil {
		return BasicSettingsResult{Error: safeContextServiceError(ctx, "settings_read_failed", err)}
	}
	reason := ""
	if s.connectionManagedPort() {
		reason = "port_managed_by_connection"
	}
	return BasicSettingsResult{
		PortMutable: reason == "", PortDisabledReason: reason,
		Settings:             BasicSettings{Port: settings.Port, LogLevel: settings.LogLevel, CoreAutostart: settings.CoreAutostart},
		CoreAutostartMutable: desktopruntime.BasicAutostartMutable(),
	}
}
func (s *BasicSettingsService) Save(ctx context.Context, settings BasicSettings) BasicSettingsSaveResult {
	if s.rootError != nil {
		return BasicSettingsSaveResult{Error: safeServiceError("settings_root_unavailable", s.rootError)}
	}
	if err := s.validateIdentity(ctx, s.runtimeRoot); err != nil {
		return BasicSettingsSaveResult{Error: connectionFailure("next_identity_unavailable")}
	}
	operationCtx, finish, err := beginRuntimeMutation(ctx, s.runtimeRoot)
	if err != nil {
		return BasicSettingsSaveResult{Error: safeContextServiceError(ctx, "settings_mutation_busy", err)}
	}
	defer finish()
	if err := s.validateIdentity(operationCtx, s.runtimeRoot); err != nil {
		return BasicSettingsSaveResult{Error: connectionFailure("next_identity_unavailable")}
	}
	if s.connectionManagedPort() {
		current, err := s.read(operationCtx, s.runtimeRoot)
		if err != nil {
			return BasicSettingsSaveResult{Error: safeContextServiceError(operationCtx, "settings_read_failed", err)}
		}
		if current.Port != settings.Port {
			return BasicSettingsSaveResult{Error: NewError("port_managed_by_connection", "Change the Next Core port through Connection", ErrorCategoryUnavailable, false, nil)}
		}
	}
	err = s.update(operationCtx, s.runtimeRoot, desktopruntime.BasicSettings{Port: settings.Port, LogLevel: settings.LogLevel, CoreAutostart: settings.CoreAutostart})
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
	if code == "connection_root_unavailable" || code == "settings_root_unavailable" || code == "diagnostics_root_unavailable" || code == "runtime_root_unavailable" || code == "update_root_unavailable" || code == "acp_root_unavailable" {
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

// Explicit Next launch selection and Next manifests both close the old Shared
// port-write path. Stable selection retains the existing settings semantics.
func (s *BasicSettingsService) connectionManagedPort() bool {
	return desktopruntime.NextManagedRoot(s.runtimeRoot)
}
