package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/execution"
	"github.com/uvwt/agentdock/internal/permission"
)

func newPermissionRuntime(t *testing.T) *Runtime {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	work := filepath.Join(root, "workspace")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AgentDockHome: home, AgentDockDefaultDir: work}
	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	return rt
}

func replaceRuntimePermissionPolicy(t *testing.T, rt *Runtime, mutate func(*permission.Policy)) permission.Policy {
	t.Helper()
	current := rt.permissions.Policy()
	next := current
	mutate(&next)
	updated, err := rt.permissions.ReplacePolicy(current.Revision, next)
	if err != nil {
		t.Fatalf("ReplacePolicy: %v", err)
	}
	return updated
}

func requirePermissionError(t *testing.T, err error, code string) *ToolError {
	t.Helper()
	var toolErr *ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError %s, got %T: %v", code, err, err)
	}
	if toolErr.Code != code || toolErr.Category != "permission" {
		t.Fatalf("permission error = %#v, want code=%s category=permission", toolErr, code)
	}
	return toolErr
}

func lastExecutionCall(t *testing.T, rt *Runtime) execution.Call {
	t.Helper()
	snapshot := rt.execution.Snapshot()
	if len(snapshot.Calls) == 0 {
		t.Fatal("execution snapshot contains no calls")
	}
	return snapshot.Calls[len(snapshot.Calls)-1]
}

func TestSnapshotValidatedToolArgumentsIsDetachedAndCanonical(t *testing.T) {
	original := map[string]any{
		"action": "add",
		"nested": map[string]any{"value": "before"},
		"items":  []any{"one", "two"},
	}
	snapshot, err := snapshotValidatedToolArguments(original)
	if err != nil {
		t.Fatal(err)
	}
	original["action"] = "delete"
	original["nested"].(map[string]any)["value"] = "after"
	original["items"].([]any)[0] = "changed"

	if snapshot["action"] != "add" {
		t.Fatalf("snapshot action = %#v", snapshot["action"])
	}
	nested, _ := snapshot["nested"].(map[string]any)
	if nested["value"] != "before" {
		t.Fatalf("snapshot nested = %#v", nested)
	}
	items, _ := snapshot["items"].([]any)
	if len(items) != 2 || items[0] != "one" {
		t.Fatalf("snapshot items = %#v", items)
	}
	first, err := runtimePermissionFingerprint("file_edit", "add", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtimePermissionFingerprint("file_edit", "add", map[string]any{
		"items":  []any{"one", "two"},
		"nested": map[string]any{"value": "before"},
		"action": "add",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("canonical fingerprints differ: %s != %s", first, second)
	}
}

func TestRuntimePermissionUsesExecutionEpochAndDefaultFullCompatibility(t *testing.T) {
	rt := newPermissionRuntime(t)
	if got, want := rt.permissions.RuntimeEpoch(), rt.execution.Epoch(); got == "" || got != want {
		t.Fatalf("permission epoch=%q execution epoch=%q", got, want)
	}

	target := filepath.Join(rt.ws.Root(), "created.txt")
	result, err := rt.Call(context.Background(), "file_edit", map[string]any{
		"action":  "add",
		"path":    target,
		"content": "created by default full policy\n",
	})
	if err != nil {
		t.Fatalf("default full file_edit failed: %v", err)
	}
	if result["changed"] != true {
		t.Fatalf("file_edit result = %#v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "created by default full policy\n" {
		t.Fatalf("target content = %q", data)
	}
	call := lastExecutionCall(t, rt)
	if call.Status != execution.StatusCompleted || call.ErrorCode != "" || call.ErrorCategory != "" {
		t.Fatalf("execution call = %#v", call)
	}
}

func TestRuntimePermissionRulesAskDoesNotDispatchAndJournalsFailure(t *testing.T) {
	rt := newPermissionRuntime(t)
	replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
		policy.GlobalMode = permission.Rules
	})

	target := filepath.Join(rt.ws.Root(), "must-not-exist.txt")
	result, err := rt.Call(context.Background(), "file_edit", map[string]any{
		"action":  "add",
		"path":    target,
		"content": "must not be written\n",
	})
	if result != nil {
		t.Fatalf("approval-required result = %#v, want nil", result)
	}
	toolErr := requirePermissionError(t, err, "APPROVAL_REQUIRED")
	if executed, ok := toolErr.Details["executed"].(bool); !ok || executed {
		t.Fatalf("approval details executed = %#v", toolErr.Details["executed"])
	}
	approvalID, _ := toolErr.Details["approval_id"].(string)
	if strings.TrimSpace(approvalID) == "" {
		t.Fatalf("approval details = %#v", toolErr.Details)
	}
	if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("handler ran before approval, stat err=%v", statErr)
	}

	history, historyErr := rt.permissions.History()
	if historyErr != nil {
		t.Fatal(historyErr)
	}
	if len(history) != 1 || history[0].ID != approvalID || history[0].Status != permission.Pending {
		t.Fatalf("approval history = %#v", history)
	}
	call := lastExecutionCall(t, rt)
	if call.Status != execution.StatusFailed || call.ErrorCode != "APPROVAL_REQUIRED" || call.ErrorCategory != "permission" {
		t.Fatalf("execution call = %#v", call)
	}
}

func TestRuntimePermissionReadOnlyDenyDoesNotDispatchAndJournalsFailure(t *testing.T) {
	rt := newPermissionRuntime(t)
	replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
		policy.GlobalMode = permission.ReadOnly
	})

	target := filepath.Join(rt.ws.Root(), "denied.txt")
	result, err := rt.Call(context.Background(), "file_edit", map[string]any{
		"action":  "add",
		"path":    target,
		"content": "must not be written\n",
	})
	if result != nil {
		t.Fatalf("denied result = %#v, want nil", result)
	}
	requirePermissionError(t, err, "PERMISSION_DENIED")
	if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("handler ran after deny, stat err=%v", statErr)
	}
	history, historyErr := rt.permissions.History()
	if historyErr != nil {
		t.Fatal(historyErr)
	}
	if len(history) != 0 {
		t.Fatalf("deny created approval history: %#v", history)
	}
	call := lastExecutionCall(t, rt)
	if call.Status != execution.StatusFailed || call.ErrorCode != "PERMISSION_DENIED" || call.ErrorCategory != "permission" {
		t.Fatalf("execution call = %#v", call)
	}
}

func TestRuntimePermissionRulesAllowsCoreProvenReadOnly(t *testing.T) {
	rt := newPermissionRuntime(t)
	target := filepath.Join(rt.ws.Root(), "readable.txt")
	if err := os.WriteFile(target, []byte("read me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
		policy.GlobalMode = permission.Rules
	})

	result, err := rt.Call(context.Background(), "read_file", map[string]any{"path": target})
	if err != nil {
		t.Fatalf("rules read_file failed: %v", err)
	}
	if result["content"] != "read me\n" {
		t.Fatalf("read_file result = %#v", result)
	}
}

func TestNewRuntimeFailsClosedOnCorruptPermissionState(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	work := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(home, "permissions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "permissions", "state.json"), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}

	rt, err := NewRuntime(config.Config{AgentDockHome: home, AgentDockDefaultDir: work})
	if rt != nil {
		_ = rt.Close()
		t.Fatal("NewRuntime returned a runtime for corrupt permission state")
	}
	if err == nil || !strings.Contains(err.Error(), "initialize permission store") {
		t.Fatalf("NewRuntime error = %v", err)
	}
}
