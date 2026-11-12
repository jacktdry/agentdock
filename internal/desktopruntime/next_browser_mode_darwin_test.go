//go:build darwin

package desktopruntime

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/envstore"
)

func TestNextBrowserModeConfigRoundTripPreservesOtherBytes(t *testing.T) {
	original := []byte("# user-managed Next settings\nAGENTDOCK_DESKTOP_VARIANT='next'\nAGENTDOCK_AUTH_TOKEN='private-sentinel'\nCUSTOM='unrelated'\n")
	workspace := filepath.Join(t.TempDir(), "Work Space")
	enabled, err := rewriteNextBrowserMode(original, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(enabled), string(original)) {
		t.Fatal("enable rewrote unrelated configuration or credential bytes")
	}
	parsed, err := envstore.Parse(enabled)
	if err != nil || parsed[nextBrowserModeKey] != workspace {
		t.Fatalf("canonical opt-in not preserved: %v", err)
	}
	again, err := rewriteNextBrowserMode(enabled, workspace)
	if err != nil || !reflect.DeepEqual(enabled, again) {
		t.Fatal("repeated enable modified already-selected mode")
	}
	disabled, err := rewriteNextBrowserMode(enabled, "")
	if err != nil || !reflect.DeepEqual(original, disabled) {
		t.Fatal("disable failed to restore exact original Next env bytes")
	}
}

func TestNextBrowserModeConfigLegacyAndAmbiguousValues(t *testing.T) {
	const key = nextBrowserModeKey
	raw := []byte("AGENTDOCK_DESKTOP_VARIANT=next\nexport " + key + "='/old/workspace'\n")
	updated, err := rewriteNextBrowserMode(raw, "/new/workspace")
	if err != nil {
		t.Fatal(err)
	}
	values, err := envstore.Parse(updated)
	if err != nil || values[key] != "/new/workspace" {
		t.Fatalf("legacy mode was not changed: %v", err)
	}
	if _, err := rewriteNextBrowserMode(append(raw, []byte(key+"  ='/duplicate'\n")...), ""); err == nil {
		t.Fatal("duplicate env assignments must be rejected")
	}
	if _, err := rewriteNextBrowserMode([]byte("UNFINISHED='\n"), "/shared"); err == nil {
		t.Fatal("malformed env must be rejected without mutation")
	}
	if _, err := rewriteNextBrowserMode([]byte("CUSTOM='line1\n"+key+"=secret\nline3'\n"), ""); err == nil {
		t.Fatal("multiline content mistaken for assignment must be rejected")
	}
}

func TestNextBrowserModeStatusPreservesGuardWithoutClearing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	private := filepath.Join(home, ".agentdock-next")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(private, nextBrowserRestartMarker)
	data := []byte("unresolved worker generation")
	if err := os.WriteFile(marker, data, 0600); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(home, "AgentDock Next")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	state, err := browserModeStatusFromValues(filepath.Join(home, "Library", "Application Support", "AgentDock Next"), map[string]string{
		nextBrowserModeKey: workspace,
	})
	if err != nil || state.ConfiguredMode != "shared" || !state.GuardPresent || state.EffectiveMode != "not_observed" {
		t.Fatalf("guard state not visible: %+v / %v", state, err)
	}
	after, err := os.ReadFile(marker)
	if err != nil || !reflect.DeepEqual(data, after) {
		t.Fatal("status changed the restart guard")
	}
	if invalid, err := browserModeStatusFromValues("", map[string]string{nextBrowserModeKey: ""}); err != nil || invalid.ConfiguredMode != "invalid" {
		t.Fatal("invalid empty explicit opt-in reported as exclusive")
	}
}

func TestNextBrowserModeCommandNeverTouchesUnqualifiedRuntime(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "stable")
	root := t.TempDir()
	if err := RunNextBrowserModeCommand(context.Background(), []string{"enable", "--runtime-root", root}, &strings.Builder{}, &strings.Builder{}); err == nil {
		t.Fatal("stable or unrelated root was accepted")
	}
	if err := RunNextBrowserModeCommand(context.Background(), []string{"enable"}, &strings.Builder{}, &strings.Builder{}); err == nil {
		t.Fatal("implicit root was accepted")
	}
	if err := RunNextBrowserModeCommand(context.Background(), []string{"apply", "--runtime-root", root}, &strings.Builder{}, &strings.Builder{}); err == nil {
		t.Fatal("unsupported auto-restart action was accepted")
	}
}
