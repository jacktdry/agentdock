package desktopapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

func TestACPNativeCodexReadinessRedactedAndFailClosed(t *testing.T) {
	s := NewACPService(t.TempDir())
	s.nativeCodexReadiness = func(_ context.Context, root string) (desktopruntime.NativeCodexReadiness, error) {
		if root != s.runtimeRoot {
			t.Fatal("runtime root substitution")
		}
		return desktopruntime.NativeCodexReadiness{OK: true, State: "recovery_required"}, nil
	}
	result := s.NativeCodexReadiness(context.Background())
	if result.Error != nil || result.State != "recovery_required" {
		t.Fatalf("valid state %v", result)
	}
	for _, scenario := range []struct {
		reply desktopruntime.NativeCodexReadiness
		err   error
	}{
		{desktopruntime.NativeCodexReadiness{OK: true, State: "all_ready"}, nil},
		{desktopruntime.NativeCodexReadiness{OK: false, State: "operator_setup_required"}, nil},
		{desktopruntime.NativeCodexReadiness{}, errors.New("PRIVATE token=canary pid=123 path=/tmp/secret")},
	} {
		s.nativeCodexReadiness = func(context.Context, string) (desktopruntime.NativeCodexReadiness, error) {
			return scenario.reply, scenario.err
		}
		got := s.NativeCodexReadiness(context.Background())
		raw, _ := json.Marshal(got)
		if got.State != "unavailable" || got.Error == nil || strings.Contains(string(raw), "canary") || strings.Contains(string(raw), "pid=") {
			t.Fatalf("untrusted read was accepted or leaked: %s", raw)
		}
	}
}
