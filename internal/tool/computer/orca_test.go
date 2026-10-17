package computer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func newTestOrcaProvider(t *testing.T, handler func([]string, string) ([]byte, []byte, error)) *OrcaProvider {
	t.Helper()
	p := &OrcaProvider{command: "orca", timeout: defaultProviderTimeout}
	p.run = func(_ context.Context, _ string, args []string, stdin string) ([]byte, []byte, error) {
		return handler(args, stdin)
	}
	return p
}

func successEnvelope(result map[string]any) []byte {
	data, _ := json.Marshal(map[string]any{"id": "test", "ok": true, "result": result})
	return data
}

func TestOrcaProviderUsesStdinForSensitiveText(t *testing.T) {
	var gotArgs []string
	var gotStdin string
	provider := newTestOrcaProvider(t, func(args []string, stdin string) ([]byte, []byte, error) {
		gotArgs = append([]string(nil), args...)
		gotStdin = stdin
		return successEnvelope(map[string]any{"verification": "verified"}), nil, nil
	})
	idx := 7
	if _, err := provider.Act(context.Background(), ActionRequest{Action: "set_value", App: "Test", ElementIndex: &idx, Value: "super-secret"}); err != nil {
		t.Fatal(err)
	}
	if gotStdin != "super-secret" {
		t.Fatalf("stdin=%q", gotStdin)
	}
	joined := strings.Join(gotArgs, " ")
	if strings.Contains(joined, "super-secret") {
		t.Fatalf("secret leaked into args: %v", gotArgs)
	}
	want := []string{"computer", "set-value", "--app", "Test", "--element-index", "7", "--value-stdin", "--json"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("args=%v want=%v", gotArgs, want)
	}
}

func TestOrcaObservationBuildsBackgroundSafeArgs(t *testing.T) {
	var got []string
	provider := newTestOrcaProvider(t, func(args []string, _ string) ([]byte, []byte, error) {
		got = append([]string(nil), args...)
		return successEnvelope(map[string]any{"snapshot": map[string]any{}}), nil, nil
	})
	window := int64(42)
	if _, err := provider.Observe(context.Background(), ObservationRequest{Action: "get_app_state", App: "com.openai.codex", WindowID: &window, NoScreenshot: true}); err != nil {
		t.Fatal(err)
	}
	want := []string{"computer", "get-app-state", "--app", "com.openai.codex", "--window-id", "42", "--no-screenshot", "--json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%v want=%v", got, want)
	}
}

func TestOrcaProviderPreservesStructuredProviderFailure(t *testing.T) {
	provider := newTestOrcaProvider(t, func([]string, string) ([]byte, []byte, error) {
		body, _ := json.Marshal(map[string]any{"id": "x", "ok": false, "error": map[string]any{"code": "app_not_found", "message": "missing"}})
		return body, nil, errors.New("exit status 1")
	})
	_, err := provider.Observe(context.Background(), ObservationRequest{Action: "get_app_state", App: "missing"})
	var computerErr *Error
	if !errors.As(err, &computerErr) || computerErr.Code != ErrProviderFailed || computerErr.Details.ProviderErrorCode != "app_not_found" || !strings.Contains(computerErr.Details.Reason, "app_not_found") {
		t.Fatalf("err=%#v", err)
	}
}

func TestOrcaListWindowsDoesNotInventUnsupportedWindowSelector(t *testing.T) {
	var got []string
	provider := newTestOrcaProvider(t, func(args []string, _ string) ([]byte, []byte, error) {
		got = append([]string(nil), args...)
		return successEnvelope(map[string]any{"windows": []any{}}), nil, nil
	})
	window := int64(42)
	if _, err := provider.Observe(context.Background(), ObservationRequest{Action: "list_windows", App: "Test", WindowID: &window}); err != nil {
		t.Fatal(err)
	}
	want := []string{"computer", "list-windows", "--app", "Test", "--json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%v want=%v", got, want)
	}
}

func TestOrcaActionCanExplicitlyRestoreWindow(t *testing.T) {
	var got []string
	provider := newTestOrcaProvider(t, func(args []string, _ string) ([]byte, []byte, error) {
		got = append([]string(nil), args...)
		return successEnvelope(map[string]any{"verification": "verified"}), nil, nil
	})
	idx := 3
	if _, err := provider.Act(context.Background(), ActionRequest{Action: "click", App: "Test", ElementIndex: &idx, RestoreWindow: true, NoScreenshot: true}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--restore-window") || !strings.Contains(joined, "--no-screenshot") {
		t.Fatalf("args=%v", got)
	}
}

// This reproduces Next launchd's missing /usr/local/bin in PATH; Orca is
// already installed in a known CLI directory but exec.LookPath("orca") fails.
func TestOrcaProviderDiscoversUserLocalCLIWithMinimalLaunchdPATH(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS launchd CLI path")
	}
	home := t.TempDir()
	cmd := filepath.Join(home, ".local", "bin", "orca")
	if err := os.MkdirAll(filepath.Dir(cmd), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("ORCA_CLI_COMMAND", "")
	t.Setenv("ORCA_DEV_REPO_ROOT", "")
	p, err := NewOrcaProvider(OrcaProviderConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if p.command != cmd {
		t.Fatalf("discovered command=%q, want %q", p.command, cmd)
	}
}

func TestOrcaProviderResolvesExplicitExecutableOverrideWithMinimalLaunchdPATH(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS launchd CLI path")
	}
	root := t.TempDir()
	cmd := filepath.Join(root, "orca")
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("ORCA_CLI_COMMAND", cmd)
	p, err := NewOrcaProvider(OrcaProviderConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if p.command != cmd {
		t.Fatalf("override command=%q, want %q", p.command, cmd)
	}
}
