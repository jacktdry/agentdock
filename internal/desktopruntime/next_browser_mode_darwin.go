//go:build darwin

package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/envstore"
	"github.com/uvwt/agentdock/internal/fs/atomicfile"
)

const nextBrowserModeKey = "AGENTDOCK_NEXT_SHARED_BROWSER_WORKSPACE"
const nextBrowserRestartMarker = ".shared-browser-restart-pending.v1"

// NextBrowserModeStatus intentionally distinguishes persisted preference from
// the live Core mode. Changing the preference never restarts another session.
type NextBrowserModeStatus struct {
	ConfiguredMode  string `json:"configuredMode"`
	EffectiveMode   string `json:"effectiveMode"`
	RestartRequired bool   `json:"restartRequired"`
	GuardPresent    bool   `json:"guardPresent"`
	Changed         bool   `json:"changed"`
	Message         string `json:"message"`
}

// RunNextBrowserModeCommand is a NEXT-only source-level operator workflow.
// A preference is staged in the existing private env; activation happens on a
// subsequent normal Core start, never by interrupting a live Broker/lease.
func RunNextBrowserModeCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || (args[0] != "status" && args[0] != "enable" && args[0] != "disable") {
		return errors.New("usage: agentdock browser-mode <status|enable|disable> --runtime-root <AgentDock Next runtime>")
	}
	action := args[0]
	flags := flag.NewFlagSet("agentdock browser-mode "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("runtime-root", "", "AgentDock Next runtime root")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *root == "" {
		return errors.New("browser-mode requires an explicit --runtime-root")
	}
	home, err := os.UserHomeDir()
	if err != nil || os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" ||
		*root != filepath.Join(home, "Library", "Application Support", "AgentDock Next") {
		return ErrNextIdentityUnavailable
	}
	if _, err := explicitNextDarwinRuntime(*root); err != nil {
		return ErrNextIdentityUnavailable
	}

	if action == "status" {
		state, err := readNextBrowserMode(ctx, *root)
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(state)
	}
	unlock, err := AcquireDesktopMutation(ctx, *root)
	if err != nil {
		return err
	}
	defer unlock()

	// Re-read under the cross-process desktop mutation lock, including identity,
	// ownership, guard state, and all env values. No restart or guard removal.
	if _, err := explicitNextDarwinRuntime(*root); err != nil {
		return ErrNextIdentityUnavailable
	}
	path, original, values, err := readBasicEnvironment(*root)
	if err != nil || values["AGENTDOCK_DESKTOP_VARIANT"] != "next" {
		return ErrNextIdentityUnavailable
	}
	state, err := readNextBrowserMode(ctx, *root)
	if err != nil {
		return err
	}
	if action == "enable" {
		if port := values["AGENTDOCK_PORT"]; port != "" && port != "8767" {
			return errors.New("Next shared Browser mode requires the isolated Next Core port 8767")
		}
		workspace := values["AGENTDOCK_DEFAULT_DIR"]
		if workspace == "" {
			workspace = filepath.Join(home, "AgentDock Next")
		}
		canonical, err := browserpolicy.CanonicalWorkspaceRoot(workspace)
		if err != nil || canonical != workspace || strings.ContainsAny(workspace, "\r\n") {
			return errors.New("Next default Browser workspace is not an existing canonical directory")
		}
		values[nextBrowserModeKey] = canonical
	} else {
		delete(values, nextBrowserModeKey)
	}
	updated, err := rewriteNextBrowserMode(original, values[nextBrowserModeKey])
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(original, updated) {
		if err := atomicfile.Write(path, updated, 0o600); err != nil {
			return err
		}
		state.Changed = true
	}
	state.ConfiguredMode = "exclusive"
	if values[nextBrowserModeKey] != "" {
		state.ConfiguredMode = "shared"
	}
	state.RestartRequired = state.Changed || (state.EffectiveMode != "not_observed" && state.EffectiveMode != state.ConfiguredMode)
	state.Message = "Preference saved for the next normal Next Core start; no Core, browser, lease, or guard was restarted or removed."
	if state.GuardPresent {
		state.Message += " Existing restart guard requires normal verified shutdown or manual reconciliation before the next start."
	}
	return json.NewEncoder(stdout).Encode(state)
}

func readNextBrowserMode(ctx context.Context, root string) (NextBrowserModeStatus, error) {
	if err := ctx.Err(); err != nil {
		return NextBrowserModeStatus{}, err
	}
	_, _, values, err := readBasicEnvironment(root)
	if err != nil || values["AGENTDOCK_DESKTOP_VARIANT"] != "next" {
		return NextBrowserModeStatus{}, ErrNextIdentityUnavailable
	}
	state, err := browserModeStatusFromValues(root, values)
	if err != nil {
		return state, err
	}
	observe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	snapshot, err := ReadVerifiedNextBrowserSnapshot(observe, root)
	if err == nil && snapshot.Availability == "available" {
		switch snapshot.ManagedPoolMode {
		case "exclusive":
			state.EffectiveMode = "exclusive"
		case "experimental_shared":
			state.EffectiveMode = "shared"
		}
		if state.EffectiveMode != "not_observed" {
			state.RestartRequired = state.EffectiveMode != state.ConfiguredMode
			if state.ConfiguredMode == "invalid" || state.GuardPresent {
				state.Message += " Live mode observed from verified Next Core."
			} else {
				state.Message = "Live mode observed from verified Next Core; preference changes apply on the next normal Core start."
			}
		}
	}
	return state, nil
}

func browserModeStatusFromValues(root string, values map[string]string) (NextBrowserModeStatus, error) {
	state := NextBrowserModeStatus{
		ConfiguredMode: "exclusive", EffectiveMode: "not_observed",
		Message: "Configuration is for the next Next Core start; live mode has not been verified.",
	}
	if _, configured := values[nextBrowserModeKey]; configured {
		selected := values[nextBrowserModeKey]
		if selected == "" {
			state.ConfiguredMode = "invalid"
			state.Message = "Empty explicit shared Browser workspace; Next Core startup will reject it."
		} else if canonical, err := browserpolicy.CanonicalWorkspaceRoot(selected); err != nil || canonical != selected {
			state.ConfiguredMode = "invalid"
			state.Message = "Shared Browser workspace is not an existing canonical directory; Next Core startup will reject it."
		} else {
			state.ConfiguredMode = "shared"
		}
	}
	// The restart guard lives under ~/.agentdock-next, not runtime-root.
	home, err := os.UserHomeDir()
	if err != nil {
		return state, err
	}
	marker := filepath.Join(home, ".agentdock-next", nextBrowserRestartMarker)
	if _, err = os.Lstat(marker); err == nil {
		state.GuardPresent = true
		state.Message += " Next shared Browser restart guard exists; do not delete it or force a restart."
	} else if !errors.Is(err, os.ErrNotExist) {
		return state, errors.New("Next shared Browser guard state cannot be verified")
	}
	return state, nil
}

// Rewrite only this one key. Preserve credentials, comments, ordering and every
// other byte exactly; reject ambiguous duplicate/complex assignments. Parse
// before and after to ensure no unrelated variable changes semantics.
func rewriteNextBrowserMode(original []byte, workspace string) ([]byte, error) {
	before, err := envstore.Parse(original)
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	found := 0
	for _, line := range strings.SplitAfter(string(original), "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimLeft(strings.TrimPrefix(trimmed, "export "), " \t")
		}
		if strings.HasPrefix(trimmed, nextBrowserModeKey) &&
			strings.HasPrefix(strings.TrimLeft(strings.TrimPrefix(trimmed, nextBrowserModeKey), " \t"), "=") {
			found++
			if workspace != "" {
				out.WriteString(nextBrowserModeKey + "='" + strings.ReplaceAll(workspace, "'", "'\\''") + "'\n")
			}
			continue
		}
		out.WriteString(line)
	}
	if found > 1 {
		return nil, errors.New("ambiguous duplicate shared Browser assignments; no configuration changed")
	}
	if workspace == before[nextBrowserModeKey] && ((workspace != "" && found == 1) || (workspace == "" && found == 0)) {
		return original, nil
	}
	if workspace != "" && found == 0 {
		if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
			out.WriteByte('\n')
		}
		out.WriteString(nextBrowserModeKey + "='" + strings.ReplaceAll(workspace, "'", "'\\''") + "'\n")
	}
	updated := []byte(out.String())
	after, err := envstore.Parse(updated)
	if err != nil {
		return nil, errors.New("shared Browser config could not be safely parsed; no change")
	}
	if workspace == "" {
		delete(before, nextBrowserModeKey)
	} else {
		before[nextBrowserModeKey] = workspace
	}
	if !reflect.DeepEqual(before, after) {
		return nil, fmt.Errorf("shared Browser configuration is ambiguous; no change")
	}
	return updated, nil
}
