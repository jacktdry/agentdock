package desktopapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

func TestDiagnosticsDirectorySafeBoundary(t *testing.T) {
	s := NewDiagnosticsService(t.TempDir())
	calls := 0
	s.inspectDirectory = func(context.Context, string, desktopruntime.NextDirectoryKind) error {
		return desktopruntime.ErrDirectoryRequiresNative
	}
	s.openDirectory = func(context.Context, string, desktopruntime.NextDirectoryKind) error {
		calls++
		return errors.New("secret /arbitrary/path command output")
	}
	caps := s.Directories(context.Background())
	if caps.Logs.Enabled || caps.Configuration.Enabled || caps.Logs.Reason != "requires_native" {
		t.Fatal(caps)
	}
	for _, kind := range []NextDirectoryKind{"", "../logs", "/arbitrary/path", "configuration/secret"} {
		r := s.OpenNextDirectory(context.Background(), kind)
		if r.OK || r.Error == nil || r.Error.Category != ErrorCategoryValidation {
			t.Fatal(r)
		}
	}
	if calls != 0 {
		t.Fatal("invalid intent reached adapter")
	}
	result := s.OpenNextDirectory(context.Background(), NextDirectoryLogs)
	data, _ := json.Marshal(result)
	for _, forbidden := range []string{"secret", "/arbitrary", "command", "output", s.runtimeRoot} {
		if strings.Contains(string(data), forbidden) {
			t.Fatal("leaked", string(data))
		}
	}
	if result.OK || result.Error == nil {
		t.Fatal(result)
	}
	s.openDirectory = func(context.Context, string, desktopruntime.NextDirectoryKind) error { return nil }
	if !s.OpenNextDirectory(context.Background(), NextDirectoryConfiguration).OK {
		t.Fatal("success")
	}
}
