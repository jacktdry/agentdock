package main

import (
	"context"
	"os"
	"testing"
)

func TestRuntimeStatusReadOnlyWhenRuntimeRootExists(t *testing.T) {
	service := NewRuntimeService("")
	if service.rootError != nil {
		t.Skipf("runtime root unavailable on this machine: %v", service.rootError)
	}
	if _, err := os.Stat(service.runtimeRoot); err != nil {
		t.Skipf("runtime root not installed on this machine: %v", err)
	}

	result := service.Status(context.Background())
	if result.Error != nil {
		t.Fatalf("Status() returned %s: %s", result.Error.Code, result.Error.Message)
	}
	if result.RuntimeRoot != service.runtimeRoot {
		t.Fatalf("RuntimeRoot = %q, want %q", result.RuntimeRoot, service.runtimeRoot)
	}
}
