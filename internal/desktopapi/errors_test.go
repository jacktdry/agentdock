package desktopapi

import (
	"context"
	"testing"
)

func TestErrorFromMarksDeadlineAsRetryableTimeout(t *testing.T) {
	result := ErrorFrom("runtime_status_failed", ErrorCategoryOperation, false, context.DeadlineExceeded)
	if result == nil {
		t.Fatal("ErrorFrom returned nil")
	}
	if result.Category != ErrorCategoryTimeout || !result.Retryable {
		t.Fatalf("deadline error = %#v", result)
	}
}

func TestNewErrorClonesDetails(t *testing.T) {
	details := map[string]string{" domain ": " runtime "}
	result := NewError(" code ", " message ", ErrorCategoryValidation, false, details)
	details[" domain "] = "changed"

	if result.Code != "code" || result.Message != "message" {
		t.Fatalf("normalised error = %#v", result)
	}
	if result.Details["domain"] != "runtime" {
		t.Fatalf("cloned details = %#v", result.Details)
	}
}
