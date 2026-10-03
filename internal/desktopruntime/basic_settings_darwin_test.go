//go:build darwin

package desktopruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uvwt/agentdock/internal/envstore"
)

func TestBasicSettingsLaunchdRegisteredIsNotRunning(t *testing.T) {
	for _, output := range []string{"state = waiting", "state = not running", "active count = 0\n", "properties = keepalive"} {
		if launchdJobRunning(output) {
			t.Fatal("stopped registered job treated as running")
		}
	}
	if !launchdJobRunning("job = {\n\tstate = running\n\tpid = 123\n}") {
		t.Fatal("running job missed")
	}
}

func TestUpdateBasicSettingsDarwinPreservesNativeBoundary(t *testing.T) {
	t.Setenv("AGENTDOCK_LAUNCHCTL_BIN", "/usr/bin/false")
	root := t.TempDir()
	path := filepath.Join(root, "agentdock.env")
	original := []byte("AGENTDOCK_PORT=8765\nAGENTDOCK_LOG_LEVEL=info\nAGENTDOCK_AUTH_TOKEN='secret'\nCUSTOM='advanced'\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	old, err := ReadBasicSettings(context.Background(), root)
	if err != nil || old.Port != 8765 || old.LogLevel != "info" || old.CoreAutostart {
		t.Fatalf("read: %#v %v", old, err)
	}
	requested := BasicSettings{Port: 8877, LogLevel: "debug", CoreAutostart: true}
	if err := UpdateBasicSettings(context.Background(), root, requested); !errors.Is(err, ErrBasicSettingsUnavailable) {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != string(original) {
		t.Fatal("autostart failure changed configuration")
	}
	requested.CoreAutostart = false
	if err := UpdateBasicSettings(context.Background(), root, requested); err != nil {
		t.Fatal(err)
	}
	values, err := envstore.ParseFile(path)
	if err != nil || values["AGENTDOCK_PORT"] != "8877" || values["AGENTDOCK_LOG_LEVEL"] != "debug" || values["AGENTDOCK_AUTH_TOKEN"] != "secret" || values["CUSTOM"] != "advanced" {
		t.Fatal("settings were not preserved")
	}
	before, _ := os.ReadFile(path)
	if err := UpdateBasicSettings(context.Background(), root, requested); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("no-op rewrote config")
	}
}
