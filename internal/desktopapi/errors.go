package desktopapi

import (
	"context"
	"errors"
	"strings"
)

type ErrorCategory string

const (
	ErrorCategoryValidation    ErrorCategory = "validation"
	ErrorCategoryUnavailable   ErrorCategory = "unavailable"
	ErrorCategoryOperation     ErrorCategory = "operation"
	ErrorCategoryTimeout       ErrorCategory = "timeout"
	ErrorCategoryCompatibility ErrorCategory = "compatibility"
	ErrorCategoryInternal      ErrorCategory = "internal"
)

type APIError struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Category  ErrorCategory     `json:"category"`
	Retryable bool              `json:"retryable"`
	Details   map[string]string `json:"details,omitempty"`
}

func NewError(code, message string, category ErrorCategory, retryable bool, details map[string]string) *APIError {
	return &APIError{
		Code:      strings.TrimSpace(code),
		Message:   strings.TrimSpace(message),
		Category:  category,
		Retryable: retryable,
		Details:   cloneStringMap(details),
	}
}

func ErrorFrom(code string, category ErrorCategory, retryable bool, err error) *APIError {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		category = ErrorCategoryTimeout
		retryable = true
	}
	return NewError(code, err.Error(), category, retryable, nil)
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return output
}
