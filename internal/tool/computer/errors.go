package computer

import "fmt"

const (
	ErrProviderUnavailable = "COMPUTER_PROVIDER_UNAVAILABLE"
	ErrProviderFailed      = "COMPUTER_PROVIDER_FAILED"
	ErrSessionNotFound     = "COMPUTER_SESSION_NOT_FOUND"
	ErrOwnerMismatch       = "COMPUTER_SESSION_OWNER_MISMATCH"
	ErrForegroundRequired  = "COMPUTER_FOREGROUND_REQUIRED"
	ErrFocusViolation      = "COMPUTER_FOCUS_VIOLATION"
	ErrCapabilityDenied    = "COMPUTER_CAPABILITY_DENIED"
	ErrInvalidArgument     = "COMPUTER_INVALID_ARGUMENT"
	ErrUnsupportedAction   = "COMPUTER_UNSUPPORTED_ACTION"
	ErrTimeout             = "COMPUTER_TIMEOUT"
)

type ErrorDetails struct {
	SessionID         string           `json:"computer_session_id,omitempty"`
	Provider          ProviderID       `json:"computer_provider,omitempty"`
	Action            string           `json:"action,omitempty"`
	ForegroundPolicy  ForegroundPolicy `json:"foreground_policy,omitempty"`
	OwnerACPSessionID string           `json:"owner_acp_session_id,omitempty"`
	OwnerTaskID       string           `json:"owner_task_id,omitempty"`
	ProviderErrorCode string           `json:"provider_error_code,omitempty"`
	Reason            string           `json:"reason,omitempty"`
}

type Error struct {
	Code    string
	Message string
	Phase   string
	Details *ErrorDetails
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func computerError(code, message, phase string, details *ErrorDetails, cause error) *Error {
	return &Error{Code: code, Message: message, Phase: phase, Details: details, Cause: cause}
}
