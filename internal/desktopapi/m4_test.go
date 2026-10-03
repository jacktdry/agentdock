package desktopapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/selfupdate"
)

func TestM4Manifest(t *testing.T) {
	expected := map[Domain][]string{DomainConnection: {"status", "start", "stop", "restart", "regenerate"}, DomainSettings: {"read", "save"}, DomainUpdate: {"check"}, DomainDiagnostics: {"snapshot"}}
	for _, cap := range DefaultManifest().Capabilities {
		names, ok := expected[cap.Domain]
		if !ok {
			continue
		}
		if cap.Availability != AvailabilityAvailable || cap.Version != 1 || len(cap.Operations) != len(names) {
			t.Fatalf("capability: %#v", cap)
		}
		for i, op := range cap.Operations {
			mutating := op.Name == "save" || (cap.Domain == DomainConnection && op.Name != "status")
			want := AccessRead
			if mutating {
				want = AccessMutating
			}
			if op.Name != names[i] || op.Access != want || op.RequiresConfirmation != mutating {
				t.Fatalf("operation: %#v", op)
			}
		}
		delete(expected, cap.Domain)
	}
	if len(expected) != 0 {
		t.Fatal(expected)
	}
}

func TestConnectionValidationAndSecretSafety(t *testing.T) {
	service := NewConnectionService(t.TempDir())
	calls := 0
	service.run = func(ctx context.Context, args []string, out, stderr io.Writer) error {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing deadline")
		}
		if args[0] == "status" {
			_, err := io.WriteString(out, `{"mode":"named","running":true,"ready":true,"startup_enabled":true,"public_url":"https://user:SECRET@example.com/?token=SECRET","token":"SECRET"}`)
			return err
		}
		return errors.New("SECRET")
	}
	for _, action := range []string{"invalid", "configure", "named", "launch", "autostart"} {
		result := service.Action(context.Background(), action)
		if result.Completed || result.Error == nil || result.Error.Category != ErrorCategoryValidation {
			t.Fatalf("%s: %#v", action, result)
		}
	}
	if calls != 0 {
		t.Fatal("invalid action called adapter")
	}
	result := service.Status(context.Background())
	if result.Error != nil || !result.Status.Ready || result.Status.PublicURL != "" {
		t.Fatalf("status: %#v", result)
	}
	for _, action := range []string{"start", "stop", "restart", "regenerate"} {
		result := service.Action(context.Background(), action)
		data, _ := json.Marshal(result)
		if result.Completed || result.Error == nil || strings.Contains(string(data), "SECRET") {
			t.Fatalf("unsafe result: %s", data)
		}
	}
	service.run = func(context.Context, []string, io.Writer, io.Writer) error { return context.DeadlineExceeded }
	if result := service.Action(context.Background(), "start"); result.Error.Category != ErrorCategoryTimeout {
		t.Fatal(result)
	}
	for _, raw := range []string{"https://example.com/?token=SECRET", "https://example.com/SECRET", "https://example.com/#SECRET", "http://example.com", "https://user:SECRET@example.com"} {
		if publicOrigin(raw) != "" {
			t.Fatal(raw)
		}
	}
	if publicOrigin("https://example.com/") != "https://example.com" {
		t.Fatal("valid origin rejected")
	}
}

func TestUpdateCheckOnlyAndSanitizedErrors(t *testing.T) {
	root := t.TempDir()
	service := NewUpdateService(root)
	if reflect.TypeOf(service).NumMethod() != 1 {
		t.Fatal("update mutation exposed")
	}
	service.check = func(ctx context.Context, gotRoot string) (selfupdate.CheckResult, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing deadline")
		}
		if gotRoot != root {
			t.Fatalf("runtime root = %q, want %q", gotRoot, root)
		}
		return selfupdate.CheckResult{CurrentVersion: "1", LatestVersion: "2", UpdateAvailable: true, Message: "SECRET"}, nil
	}
	result := service.Check(context.Background())
	data, _ := json.Marshal(result)
	if result.Error != nil || !result.Status.UpdateAvailable || strings.Contains(string(data), "SECRET") {
		t.Fatal(string(data))
	}
	service.check = func(context.Context, string) (selfupdate.CheckResult, error) {
		return selfupdate.CheckResult{}, errors.New("SECRET")
	}
	data, _ = json.Marshal(service.Check(context.Background()))
	if strings.Contains(string(data), "SECRET") {
		t.Fatal(string(data))
	}
}

func TestDiagnosticsDoesNotReadContents(t *testing.T) {
	root := t.TempDir()
	manifestName := "desktop-runtime.json"
	if runtime.GOOS == "windows" {
		manifestName = "runtime.json"
	}
	if err := os.WriteFile(filepath.Join(root, manifestName), []byte("SECRET invalid JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := NewDiagnosticsService(root).Snapshot()
	data, _ := json.Marshal(result)
	if result.Error != nil || !result.Snapshot.ManifestAvailable || !result.Snapshot.RuntimeDirectoryAvailable || strings.Contains(string(data), "SECRET") {
		t.Fatal(string(data))
	}
}

func TestM4SharedModelsContainOnlyAllowedFields(t *testing.T) {
	cases := []struct {
		model any
		names string
	}{
		{ConnectionStatus{}, "Mode Running Ready StartupEnabled PublicURL"},
		{BasicSettings{}, "Port LogLevel CoreAutostart"},
		{UpdateStatus{}, "CurrentVersion LatestVersion DesktopCurrentVersion UpdateAvailable DesktopUpdateAvailable"},
		{DiagnosticsSnapshot{}, "Platform Architecture RuntimeRoot RuntimeDirectoryAvailable ManifestAvailable"},
	}
	for _, item := range cases {
		typ := reflect.TypeOf(item.model)
		names := strings.Fields(item.names)
		if typ.NumField() != len(names) {
			t.Fatalf("unexpected fields in %s", typ)
		}
		for i, name := range names {
			if typ.Field(i).Name != name {
				t.Fatalf("unexpected field in %s", typ)
			}
		}
	}
}

func TestConnectionRegenerateRequiresQuickMode(t *testing.T) {
	service := NewConnectionService(t.TempDir())
	var actions []string
	service.run = func(ctx context.Context, args []string, out, stderr io.Writer) error {
		actions = append(actions, args[0])
		switch args[0] {
		case "status":
			_, err := io.WriteString(out, "{\"mode\":\"named\",\"running\":true,\"ready\":true}")
			return err
		case "regenerate":
			t.Fatal("regenerate adapter must not run outside quick mode")
		}
		return nil
	}

	result := service.Action(context.Background(), "regenerate")
	if result.Completed || result.Error == nil || result.Error.Category != ErrorCategoryUnavailable {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !reflect.DeepEqual(actions, []string{"status"}) {
		t.Fatalf("unexpected adapter calls: %#v", actions)
	}

	actions = nil
	service.run = func(ctx context.Context, args []string, out, stderr io.Writer) error {
		actions = append(actions, args[0])
		if args[0] == "status" {
			_, err := io.WriteString(out, "{\"mode\":\"quick\",\"running\":true,\"ready\":true}")
			return err
		}
		return nil
	}
	result = service.Action(context.Background(), "regenerate")
	if !result.Completed || result.Error != nil {
		t.Fatalf("quick regenerate failed: %#v", result)
	}
	if !reflect.DeepEqual(actions, []string{"status", "regenerate"}) {
		t.Fatalf("unexpected quick adapter calls: %#v", actions)
	}
}
