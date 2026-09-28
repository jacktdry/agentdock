package desktopruntime

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestRunTunnelCommandRejectsUnknownAction(t *testing.T) {
	err := RunTunnelCommand(context.Background(), []string{"unknown"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "agentdock tunnel") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTunnelCommandRequiresRuntimeRoot(t *testing.T) {
	err := RunTunnelCommand(context.Background(), []string{"start"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--runtime-root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTunnelCommandValidatesConfigureModeBeforePlatformAccess(t *testing.T) {
	err := RunTunnelCommand(
		context.Background(),
		[]string{"configure", "--runtime-root", t.TempDir(), "--mode", "invalid"},
		&bytes.Buffer{},
		&bytes.Buffer{},
	)
	if err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSupportedTunnelConfigureMode(t *testing.T) {
	for _, mode := range []string{"none", "quick", "named"} {
		if !supportedTunnelConfigureMode(mode) {
			t.Fatalf("expected %q to be supported", mode)
		}
	}
	if supportedTunnelConfigureMode("invalid") {
		t.Fatal("invalid tunnel mode was accepted")
	}

	wantTailscale := runtime.GOOS == "darwin" || runtime.GOOS == "linux"
	if got := supportedTunnelConfigureMode("tailscale"); got != wantTailscale {
		t.Fatalf("tailscale support=%v want=%v on %s", got, wantTailscale, runtime.GOOS)
	}
}

func TestRunTunnelCommandValidatesAutostartBooleanBeforePlatformAccess(t *testing.T) {
	err := RunTunnelCommand(
		context.Background(),
		[]string{"autostart", "--runtime-root", t.TempDir(), "--enabled", "yes"},
		&bytes.Buffer{},
		&bytes.Buffer{},
	)
	if err == nil || !strings.Contains(err.Error(), "enabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}
