package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/execution"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
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

func TestRuntimePermissionApproveOnceRequiresSameStablePrincipalAndExactRetry(t *testing.T) {
	rt := newPermissionRuntime(t)
	replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
		policy.GlobalMode = permission.Rules
	})
	principal := requestmeta.AuthPrincipal{Kind: "static_bearer", ID: "sha256:principal-a", Authenticated: true, Stable: true}
	ctx := requestmeta.WithAuthPrincipal(context.Background(), principal)
	target := filepath.Join(rt.ws.Root(), "approved-once.txt")
	args := map[string]any{"action": "add", "path": target, "content": "approved once\n"}

	_, err := rt.Call(ctx, "file_edit", args)
	first := requirePermissionError(t, err, "APPROVAL_REQUIRED")
	approvalID, _ := first.Details["approval_id"].(string)
	version, ok := first.Details["approval_version"].(uint64)
	if !ok {
		// ToolError details are in-process values today; keep a tolerant numeric fallback for future envelope changes.
		if raw, numeric := first.Details["approval_version"].(float64); numeric {
			version = uint64(raw)
		} else {
			t.Fatalf("approval version = %#v", first.Details["approval_version"])
		}
	}
	metadata, readErr := rt.permissions.Approval(approvalID)
	if readErr != nil || metadata.Decision == nil || len(metadata.Decision.Sources) == 0 || !metadata.CanApproveOnce || metadata.CanApproveWorkspace || metadata.WorkspaceUnavailableReason == "" {
		t.Fatalf("built-in workspace approval metadata must remain truthful: %#v, %v", metadata, readErr)
	}
	policyRevision, ok := first.Details["policy_revision"].(uint64)
	if !ok {
		if raw, numeric := first.Details["policy_revision"].(float64); numeric {
			policyRevision = uint64(raw)
		} else {
			t.Fatalf("policy revision = %#v", first.Details["policy_revision"])
		}
	}
	if _, err := rt.permissions.ApproveOnce(context.Background(), permission.Mutation{
		ApprovalID: approvalID, ApprovalVersion: version, PolicyRevision: policyRevision, Actor: "test-desktop-control",
	}); err != nil {
		t.Fatalf("ApproveOnce: %v", err)
	}

	otherCtx := requestmeta.WithAuthPrincipal(context.Background(), requestmeta.AuthPrincipal{
		Kind: "static_bearer", ID: "sha256:principal-b", Authenticated: true, Stable: true,
	})
	_, err = rt.Call(otherCtx, "file_edit", args)
	requirePermissionError(t, err, "APPROVAL_REQUIRED")
	if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("different principal consumed approval, stat=%v", statErr)
	}

	result, err := rt.Call(ctx, "file_edit", args)
	if err != nil {
		t.Fatalf("exact same-principal retry failed: %v", err)
	}
	if result["changed"] != true {
		t.Fatalf("retry result = %#v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "approved once\n" {
		t.Fatalf("approved file data=%q err=%v", data, err)
	}

	record, err := rt.permissions.Approval(approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != permission.Consumed || record.DispatchOutcome != permission.Succeeded || record.RetryCallID == "" {
		t.Fatalf("consumed record = %#v", record)
	}

	_, err = rt.Call(ctx, "file_edit", map[string]any{
		"action": "replace", "path": target, "old": "approved once", "new": "second use",
	})
	requirePermissionError(t, err, "APPROVAL_REQUIRED")
	data, _ = os.ReadFile(target)
	if string(data) != "approved once\n" {
		t.Fatalf("one-shot grant was reusable: %q", data)
	}
}

func TestHostOperationRulesAskBeforeDispatchAndJournalPermissionFailure(t *testing.T) {
	rt := newPermissionRuntime(t)
	replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
		policy.GlobalMode = permission.Rules
	})
	ctx := requestmeta.WithAuthPrincipal(context.Background(), requestmeta.NewStableAuthPrincipal("acp_bridge", "session-a", "codex"))
	op := permission.HostOperation{
		Tool: "acp_browser", Action: "acquire",
		SessionID: "session-a", ProfileID: "codex",
		WorkspaceRoot: rt.ws.Root(),
		Payload:       map[string]any{"url": "https://example.test"},
	}

	finish, err := rt.admitHostOperation(ctx, op)
	if finish != nil {
		t.Fatal("approval-required host operation returned a dispatch finisher")
	}
	toolErr := requirePermissionError(t, err, "APPROVAL_REQUIRED")
	approvalID, _ := toolErr.Details["approval_id"].(string)
	if approvalID == "" {
		t.Fatalf("approval details=%#v", toolErr.Details)
	}
	record, err := rt.permissions.Approval(approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != permission.Pending || record.Tool != "acp_browser" || record.Action != "acquire" {
		t.Fatalf("approval record=%#v", record)
	}
	call := lastExecutionCall(t, rt)
	if call.Tool != "acp_browser" || call.Source != "acp_bridge" ||
		call.Status != execution.StatusFailed || call.ErrorCode != "APPROVAL_REQUIRED" || call.ErrorCategory != "permission" {
		t.Fatalf("host execution call=%#v", call)
	}
}

func TestHostOperationRulesAllowsCoreProvenReadOnlyAndJournalsCompletion(t *testing.T) {
	rt := newPermissionRuntime(t)
	replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
		policy.GlobalMode = permission.Rules
	})
	ctx := requestmeta.WithAuthPrincipal(context.Background(), requestmeta.NewStableAuthPrincipal("acp_bridge", "session-read", "codex"))
	finish, err := rt.admitHostOperation(ctx, permission.HostOperation{
		Tool: "acp_browser", Action: "take_snapshot",
		SessionID: "session-read", ProfileID: "codex", WorkspaceRoot: rt.ws.Root(),
		Payload: map[string]any{"lease_id": "lease-read"},
	})
	if err != nil || finish == nil {
		t.Fatalf("read-only admission finish=%v err=%v", finish != nil, err)
	}
	finish(nil)
	call := lastExecutionCall(t, rt)
	if call.Tool != "acp_browser" || call.Status != execution.StatusCompleted || call.ErrorCode != "" {
		t.Fatalf("read-only host execution call=%#v", call)
	}
}

func TestHostOperationApproveOnceConsumesExactSamePrincipalRequest(t *testing.T) {
	rt := newPermissionRuntime(t)
	replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
		policy.GlobalMode = permission.Rules
	})
	principal := requestmeta.NewStableAuthPrincipal("acp_bridge", "session-once", "codex")
	ctx := requestmeta.WithAuthPrincipal(context.Background(), principal)
	op := permission.HostOperation{
		Tool: "acp_computer", Action: "act",
		SessionID: "session-once", ProfileID: "codex", WorkspaceRoot: rt.ws.Root(),
		Payload: map[string]any{"session_id": "computer-1", "request": map[string]any{"action": "click", "element_index": 1}},
	}

	_, err := rt.admitHostOperation(ctx, op)
	first := requirePermissionError(t, err, "APPROVAL_REQUIRED")
	approvalID, _ := first.Details["approval_id"].(string)
	version := permissionDetailUint64(t, first.Details, "approval_version")
	revision := permissionDetailUint64(t, first.Details, "policy_revision")
	if _, err := rt.permissions.ApproveOnce(context.Background(), permission.Mutation{
		ApprovalID: approvalID, ApprovalVersion: version, PolicyRevision: revision, Actor: "test-desktop-control",
	}); err != nil {
		t.Fatalf("ApproveOnce: %v", err)
	}

	otherCtx := requestmeta.WithAuthPrincipal(context.Background(), requestmeta.NewStableAuthPrincipal("acp_bridge", "other-session", "codex"))
	if finish, err := rt.admitHostOperation(otherCtx, op); finish != nil || err == nil {
		t.Fatalf("different principal consumed host grant: finish=%v err=%v", finish != nil, err)
	}

	finish, err := rt.admitHostOperation(ctx, op)
	if err != nil || finish == nil {
		t.Fatalf("exact host retry finish=%v err=%v", finish != nil, err)
	}
	finish(nil)
	record, err := rt.permissions.Approval(approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != permission.Consumed || record.DispatchOutcome != permission.Succeeded || record.RetryCallID == "" {
		t.Fatalf("consumed host approval=%#v", record)
	}

	if finish, err := rt.admitHostOperation(ctx, op); finish != nil || err == nil {
		t.Fatalf("one-shot host grant was reusable: finish=%v err=%v", finish != nil, err)
	}
}

func TestRuntimeManagementMutationAdmissionJournalsBeforeAndAfterDispatch(t *testing.T) {
	t.Run("ask stops dispatch", func(t *testing.T) {
		rt := newPermissionRuntime(t)
		replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
			policy.GlobalMode = permission.Rules
		})
		ctx := requestmeta.WithAuthPrincipal(context.Background(), requestmeta.NewStableAuthPrincipal("static_bearer", "runtime-api-test"))
		dispatched := false
		result, err := rt.runRuntimeManagementMutation(ctx, "runtime_task", "delete", map[string]any{"task_id": "task_demo"}, func() (Result, error) {
			dispatched = true
			return Result{"changed": true}, nil
		})
		if result != nil || dispatched {
			t.Fatalf("approval-required runtime mutation dispatched=%v result=%#v", dispatched, result)
		}
		toolErr := requirePermissionError(t, err, "APPROVAL_REQUIRED")
		approvalID, _ := toolErr.Details["approval_id"].(string)
		record, approvalErr := rt.permissions.Approval(approvalID)
		if approvalErr != nil {
			t.Fatal(approvalErr)
		}
		if record.Tool != "runtime_task" || record.Action != "delete" || record.Scope != "AgentDock Runtime management" {
			t.Fatalf("runtime management approval=%#v", record)
		}
		call := lastExecutionCall(t, rt)
		if call.Tool != "runtime_task" || call.Source != "internal" || call.Status != execution.StatusFailed ||
			call.ErrorCode != "APPROVAL_REQUIRED" || call.ErrorCategory != "permission" {
			t.Fatalf("runtime management denied call=%#v", call)
		}
	})

	t.Run("success settles completion", func(t *testing.T) {
		rt := newPermissionRuntime(t)
		dispatched := false
		result, err := rt.runRuntimeManagementMutation(context.Background(), "runtime_task", "delete", map[string]any{"task_id": "task_demo"}, func() (Result, error) {
			dispatched = true
			return Result{"changed": true}, nil
		})
		if err != nil || !dispatched || result["changed"] != true {
			t.Fatalf("successful runtime mutation dispatched=%v result=%#v err=%v", dispatched, result, err)
		}
		call := lastExecutionCall(t, rt)
		if call.Tool != "runtime_task" || call.Source != "internal" || call.Status != execution.StatusCompleted ||
			call.ErrorCode != "" || call.ErrorCategory != "" {
			t.Fatalf("runtime management completed call=%#v", call)
		}
	})

	t.Run("dispatch failure is truthful", func(t *testing.T) {
		rt := newPermissionRuntime(t)
		dispatchErr := toolError("RUNTIME_TEST_FAILED", "runtime mutation failed", "runtime")
		result, err := rt.runRuntimeManagementMutation(context.Background(), "runtime_task", "delete", map[string]any{"task_id": "task_demo"}, func() (Result, error) {
			return nil, dispatchErr
		})
		if result != nil || !errors.Is(err, dispatchErr) {
			t.Fatalf("failed runtime mutation result=%#v err=%v", result, err)
		}
		call := lastExecutionCall(t, rt)
		if call.Status != execution.StatusFailed || call.ErrorCode != "RUNTIME_TEST_FAILED" || call.ErrorCategory != "runtime" {
			t.Fatalf("runtime management failed call=%#v", call)
		}
	})
}

func TestRuntimeManagementEntrypointsShareAdmissionBoundary(t *testing.T) {
	t.Run("insertion enqueue", func(t *testing.T) {
		rt := newPermissionRuntime(t)
		_, target := rt.execution.Begin(context.Background(), execution.BeginInput{
			Tool: "exec_command", Source: "mcp", InsertionSupported: true,
		})
		replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
			policy.GlobalMode = permission.Rules
		})
		result, err := rt.RuntimeInsertionManage(context.Background(), map[string]any{
			"action": "enqueue", "call_id": target.ID, "text": "must wait for approval",
		})
		if result != nil {
			t.Fatalf("approval-required insertion result=%#v", result)
		}
		requirePermissionError(t, err, "APPROVAL_REQUIRED")
		if got := rt.execution.Insertions(target.ID); len(got) != 0 {
			t.Fatalf("insertion dispatched before approval: %#v", got)
		}
	})

	t.Run("insertion cancel remains safe continuation", func(t *testing.T) {
		rt := newPermissionRuntime(t)
		_, target := rt.execution.Begin(context.Background(), execution.BeginInput{
			Tool: "exec_command", Source: "mcp", InsertionSupported: true,
		})
		item, err := rt.execution.EnqueueInsertion(target.ID, "cancel me")
		if err != nil {
			t.Fatal(err)
		}
		replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
			policy.GlobalMode = permission.Rules
		})
		result, err := rt.RuntimeInsertionManage(context.Background(), map[string]any{
			"action": "cancel", "insertion_id": item.ID,
		})
		if err != nil {
			t.Fatalf("safe insertion cancel rejected: %v", err)
		}
		cancelled, ok := result["insertion"].(execution.Insertion)
		if !ok || cancelled.Status != execution.InsertionCancelled {
			t.Fatalf("cancel result=%#v", result)
		}
		call := lastExecutionCall(t, rt)
		if call.Tool != "runtime_insertion" || call.Status != execution.StatusCompleted {
			t.Fatalf("cancel journal=%#v", call)
		}
	})

	t.Run("task delete", func(t *testing.T) {
		rt := newPermissionRuntime(t)
		created, err := rt.Call(context.Background(), "task_manage", map[string]any{
			"action": "create",
			"title":  "Keep until approved",
			"goal":   "verify runtime task admission",
			"steps": []map[string]any{{
				"id": "verify", "title": "Verify task remains",
			}},
			"completion_conditions": []string{"task remains before approval"},
		})
		if err != nil {
			t.Fatal(err)
		}
		taskID, _ := created["task_id"].(string)
		replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
			policy.GlobalMode = permission.Rules
		})
		result, err := rt.RuntimeTaskDelete(context.Background(), taskID)
		if result != nil {
			t.Fatalf("approval-required task delete result=%#v", result)
		}
		requirePermissionError(t, err, "APPROVAL_REQUIRED")
		if _, lookupErr := rt.RuntimeTask(taskID); lookupErr != nil {
			t.Fatalf("task deleted before approval: %v", lookupErr)
		}
	})

	t.Run("mcp manage", func(t *testing.T) {
		rt := newPermissionRuntime(t)
		replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
			policy.GlobalMode = permission.Rules
		})
		result, err := rt.RuntimeMCPManage(context.Background(), map[string]any{
			"action":      "add",
			"name":        "permission-denied-demo",
			"description": "must not be registered before approval",
			"transport":   "streamable_http",
			"url":         "http://127.0.0.1:65534/mcp",
		})
		if result != nil {
			t.Fatalf("approval-required MCP manage result=%#v", result)
		}
		requirePermissionError(t, err, "APPROVAL_REQUIRED")
		listed, listErr := rt.RuntimeMCPServers(context.Background())
		if listErr != nil {
			t.Fatal(listErr)
		}
		var servers []struct {
			Name string `json:"name"`
		}
		if err := remarshal(listed["servers"], &servers); err != nil {
			t.Fatal(err)
		}
		for _, server := range servers {
			if server.Name == "permission-denied-demo" {
				t.Fatalf("MCP server registered before approval: %#v", server)
			}
		}
	})

	t.Run("evolution proposal", func(t *testing.T) {
		rt := newPermissionRuntime(t)
		replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
			policy.GlobalMode = permission.Rules
		})
		result, err := rt.RuntimeEvolve(context.Background(), map[string]any{
			"intent": "propose",
			"candidate": map[string]any{
				"type": "preference", "statement": "permission gate comes before provider dispatch",
				"project": "agentdock", "source": "user-explicit",
			},
		})
		if result != nil {
			t.Fatalf("approval-required evolution result=%#v", result)
		}
		requirePermissionError(t, err, "APPROVAL_REQUIRED")
		call := lastExecutionCall(t, rt)
		if call.Tool != "runtime_evolve" || call.Source != "internal" || call.Status != execution.StatusFailed {
			t.Fatalf("evolution admission journal=%#v", call)
		}
	})
}

func TestACPProviderContinuationRequiresCoreAdmissionBeforeProviderResume(t *testing.T) {
	t.Setenv("GO_OUTPUT_CONTRACT_ACP_PERMISSION", "1")
	rt := newOutputContractACPRuntime(t, false)
	t.Cleanup(func() { _ = rt.Close() })
	principal := requestmeta.NewStableAuthPrincipal("static_bearer", "provider-continuation-test")
	ctx := requestmeta.WithAuthPrincipal(context.Background(), principal)

	created, err := rt.Call(ctx, "acp_session", map[string]any{"action": "new"})
	if err != nil {
		t.Fatal(err)
	}
	session, ok := created["session"].(acpruntime.SessionRecord)
	if !ok || session.ID == "" {
		t.Fatalf("created ACP session=%#v", created)
	}
	started, err := rt.Call(ctx, "acp_prompt", map[string]any{
		"action": "start", "session_id": session.ID,
		"prompt": []map[string]any{{"type": "text", "text": "request permission"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	runID, _ := started["run_id"].(string)
	if runID == "" {
		t.Fatalf("prompt start=%#v", started)
	}

	var interaction acpruntime.Interaction
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		listed, listErr := rt.Call(ctx, "acp_interaction", map[string]any{
			"action": "list", "session_id": session.ID, "pending_only": true,
		})
		if listErr != nil {
			t.Fatal(listErr)
		}
		var items []acpruntime.Interaction
		if err := remarshal(listed["interactions"], &items); err != nil {
			t.Fatal(err)
		}
		if len(items) == 1 {
			interaction = items[0]
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if interaction.ID == "" {
		t.Fatal("provider permission interaction did not become pending")
	}
	if len(interaction.Options) != 2 {
		t.Fatalf("always option was not filtered: %#v", interaction.Options)
	}

	replaceRuntimePermissionPolicy(t, rt, func(policy *permission.Policy) {
		policy.GlobalMode = permission.Rules
	})
	respondArgs := map[string]any{
		"action": "respond", "interaction_id": interaction.ID,
		"response": map[string]any{"option_id": "reject-once"},
	}
	result, err := rt.Call(ctx, "acp_interaction", respondArgs)
	if result != nil {
		t.Fatalf("approval-required provider continuation result=%#v", result)
	}
	toolErr := requirePermissionError(t, err, "APPROVAL_REQUIRED")
	approvalID, _ := toolErr.Details["approval_id"].(string)
	version := permissionDetailUint64(t, toolErr.Details, "approval_version")
	revision := permissionDetailUint64(t, toolErr.Details, "policy_revision")
	record, err := rt.permissions.Approval(approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Tool != "acp_provider_continuation" || record.Action != "select" ||
		record.Binding.Source != "acp_bridge" || record.Scope != "AgentDock ACP provider continuation" ||
		record.Status != permission.Pending {
		t.Fatalf("provider continuation approval=%#v", record)
	}

	listed, err := rt.Call(ctx, "acp_interaction", map[string]any{
		"action": "list", "session_id": session.ID, "pending_only": true,
	})
	if err != nil {
		t.Fatalf("rules-mode interaction list should remain read-only: %v", err)
	}
	var stillPending []acpruntime.Interaction
	if err := remarshal(listed["interactions"], &stillPending); err != nil {
		t.Fatal(err)
	}
	if len(stillPending) != 1 || stillPending[0].ID != interaction.ID || stillPending[0].Status != acpruntime.InteractionPending {
		t.Fatalf("provider resumed before approval: %#v", stillPending)
	}
	events, err := rt.Call(ctx, "acp_prompt", map[string]any{"action": "events", "run_id": runID})
	if err != nil {
		t.Fatalf("rules-mode prompt events should remain read-only: %v", err)
	}
	if events["status"] != acpruntime.RunRunning {
		t.Fatalf("provider run status before approval=%#v", events)
	}

	if _, err := rt.permissions.ApproveOnce(context.Background(), permission.Mutation{
		ApprovalID: approvalID, ApprovalVersion: version, PolicyRevision: revision, Actor: "test-desktop-control",
	}); err != nil {
		t.Fatalf("ApproveOnce: %v", err)
	}

	if finish, differentErr := rt.admitHostOperation(ctx, permission.HostOperation{
		Tool: "acp_provider_continuation", Action: "select", Source: "acp_bridge",
		SessionID: session.ID, ProfileID: "output-contract-helper", WorkspaceRoot: session.CWD,
		Payload: map[string]any{
			"interaction_id": "different-interaction",
			"option_id":      "reject-once",
			"tool_call":      map[string]any{"toolCallId": "tool-1", "title": "write file", "kind": "edit"},
		},
	}); finish != nil || differentErr == nil {
		t.Fatalf("different interaction consumed exact grant: finish=%v err=%v", finish != nil, differentErr)
	} else {
		requirePermissionError(t, differentErr, "APPROVAL_REQUIRED")
	}
	original, err := rt.permissions.Approval(approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if original.Status != permission.ApprovedOnce || original.RetryCallID != "" {
		t.Fatalf("different interaction consumed original approval=%#v", original)
	}

	otherCtx := requestmeta.WithAuthPrincipal(context.Background(), requestmeta.NewStableAuthPrincipal("static_bearer", "other-provider-continuation-test"))
	if otherResult, otherErr := rt.Call(otherCtx, "acp_interaction", respondArgs); otherResult != nil || otherErr == nil {
		t.Fatalf("different principal resumed provider: result=%#v err=%v", otherResult, otherErr)
	} else {
		requirePermissionError(t, otherErr, "APPROVAL_REQUIRED")
	}
	original, err = rt.permissions.Approval(approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if original.Status != permission.ApprovedOnce || original.RetryCallID != "" {
		t.Fatalf("different principal consumed original approval=%#v", original)
	}

	result, err = rt.Call(ctx, "acp_interaction", respondArgs)
	if err != nil {
		t.Fatalf("same-principal provider continuation retry failed: %v", err)
	}
	if result["responded"] != true {
		t.Fatalf("provider continuation retry result=%#v", result)
	}
	record, err = rt.permissions.Approval(approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != permission.Consumed || record.DispatchOutcome != permission.Succeeded || record.RetryCallID == "" {
		t.Fatalf("consumed provider continuation approval=%#v", record)
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		events, err = rt.Call(ctx, "acp_prompt", map[string]any{"action": "events", "run_id": runID})
		if err != nil {
			t.Fatal(err)
		}
		if events["status"] == acpruntime.RunCompleted {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if events["status"] != acpruntime.RunCompleted {
		t.Fatalf("provider did not resume after approved retry: %#v", events)
	}

	snapshot := rt.execution.Snapshot()
	failedAdmission, completedAdmission := false, false
	for _, call := range snapshot.Calls {
		if call.Tool != "acp_provider_continuation" {
			continue
		}
		if call.Status == execution.StatusFailed && call.ErrorCode == "APPROVAL_REQUIRED" && call.ErrorCategory == "permission" {
			failedAdmission = true
		}
		if call.Status == execution.StatusCompleted && call.ErrorCode == "" {
			completedAdmission = true
		}
	}
	if !failedAdmission || !completedAdmission {
		t.Fatalf("provider continuation journal failed=%v completed=%v calls=%#v", failedAdmission, completedAdmission, snapshot.Calls)
	}
}

func TestDesktopPermissionControlAuthorityIsDistinctExactAndCoreAttributed(t *testing.T) {
	rt := newPermissionRuntime(t)
	credential := rt.DesktopPermissionControlCredential()
	if credential == "" {
		t.Fatal("desktop permission control credential is empty")
	}
	if _, err := rt.BeginPermissionConfirmation("normal-mcp-bearer", permission.ControlMutationRequest{
		Kind: permission.ControlMutationReject, ApprovalID: "not-used", ApprovalVersion: 1, PolicyRevision: rt.permissions.Policy().Revision,
	}); !errors.Is(err, permission.ErrControlUnauthorized) {
		t.Fatalf("normal MCP credential began desktop confirmation: %v", err)
	}

	policy := rt.permissions.Policy()
	policy.GlobalMode = permission.Rules
	update := permission.ControlMutationRequest{
		Kind: permission.ControlMutationUpdatePolicy, PolicyRevision: policy.Revision, Policy: &policy,
	}
	challenge, err := rt.BeginPermissionConfirmation(credential, update)
	if err != nil {
		t.Fatalf("BeginPermissionConfirmation(update): %v", err)
	}
	wrong := update
	wrongPolicy := policy
	wrongPolicy.GlobalMode = permission.ReadOnly
	wrong.Policy = &wrongPolicy
	if _, err := rt.ApplyPermissionControlMutation(context.Background(), credential, challenge.ID, wrong); !errors.Is(err, permission.ErrConfirmationMismatch) {
		t.Fatalf("mismatched policy mutation err=%v", err)
	}
	if _, err := rt.ApplyPermissionControlMutation(context.Background(), credential, challenge.ID, update); !errors.Is(err, permission.ErrConfirmationNotFound) {
		t.Fatalf("mismatched challenge was reusable: %v", err)
	}

	challenge, err = rt.BeginPermissionConfirmation(credential, update)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.ApplyPermissionControlMutation(context.Background(), credential, challenge.ID, update)
	if err != nil {
		t.Fatalf("desktop policy mutation: %v", err)
	}
	updated, ok := result["policy"].(permission.Policy)
	if !ok || updated.GlobalMode != permission.Rules || updated.Revision != policy.Revision+1 {
		t.Fatalf("updated policy=%#v", result["policy"])
	}
	if _, err := rt.ApplyPermissionControlMutation(context.Background(), credential, challenge.ID, update); !errors.Is(err, permission.ErrConfirmationNotFound) {
		t.Fatalf("successful challenge was reusable: %v", err)
	}

	principalCtx := requestmeta.WithAuthPrincipal(context.Background(), requestmeta.NewStableAuthPrincipal("static_bearer", "desktop-control-approval-test"))
	target := filepath.Join(rt.ws.Root(), "desktop-control-approval.txt")
	_, err = rt.Call(principalCtx, "file_edit", map[string]any{
		"action": "add", "path": target, "content": "must not dispatch before retry\n",
	})
	toolErr := requirePermissionError(t, err, "APPROVAL_REQUIRED")
	approvalID, _ := toolErr.Details["approval_id"].(string)
	approvalVersion := permissionDetailUint64(t, toolErr.Details, "approval_version")
	policyRevision := permissionDetailUint64(t, toolErr.Details, "policy_revision")
	approve := permission.ControlMutationRequest{
		Kind:       permission.ControlMutationApproveOnce,
		ApprovalID: approvalID, ApprovalVersion: approvalVersion, PolicyRevision: policyRevision,
	}
	challenge, err = rt.BeginPermissionConfirmation(credential, approve)
	if err != nil {
		t.Fatalf("BeginPermissionConfirmation(approve): %v", err)
	}
	result, err = rt.ApplyPermissionControlMutation(context.Background(), credential, challenge.ID, approve)
	if err != nil {
		t.Fatalf("desktop approve once mutation: %v", err)
	}
	record, ok := result["approval"].(permission.ApprovalRecord)
	if !ok || record.Status != permission.ApprovedOnce || record.DecidedBy != permission.DesktopControlActor {
		t.Fatalf("desktop approval result=%#v", result["approval"])
	}
	if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("approval mutation dispatched original operation: %v", statErr)
	}
}

func TestDesktopPermissionControlCredentialRotatesPerRuntime(t *testing.T) {
	first := newPermissionRuntime(t)
	second := newPermissionRuntime(t)
	firstCredential := first.DesktopPermissionControlCredential()
	secondCredential := second.DesktopPermissionControlCredential()
	if firstCredential == "" || secondCredential == "" || firstCredential == secondCredential {
		t.Fatalf("desktop control credential rotation invalid: first=%t second=%t equal=%t",
			firstCredential != "", secondCredential != "", firstCredential == secondCredential)
	}
	request := permission.ControlMutationRequest{
		Kind:           permission.ControlMutationUpdatePolicy,
		PolicyRevision: second.permissions.Policy().Revision,
		Policy: func() *permission.Policy {
			policy := second.permissions.Policy()
			return &policy
		}(),
	}
	if _, err := second.BeginPermissionConfirmation(firstCredential, request); !errors.Is(err, permission.ErrControlUnauthorized) {
		t.Fatalf("prior runtime credential authorized new runtime: %v", err)
	}
}

func TestProtectedControlPathsAreBlockedAcrossFileAndMediaSurfaces(t *testing.T) {
	rt := newPermissionRuntime(t)
	statePath := rt.permissions.StatePath()
	permissionsDir := filepath.Dir(statePath)
	home := filepath.Dir(permissionsDir)

	cases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "read state", tool: "read_file", args: map[string]any{"path": statePath}},
		{name: "list permissions", tool: "list_dir", args: map[string]any{"path": permissionsDir}},
		{name: "search permissions", tool: "search_text", args: map[string]any{"path": permissionsDir, "query": "global_mode"}},
		{name: "replace state", tool: "file_edit", args: map[string]any{"action": "replace", "path": statePath, "old": "x", "new": "y"}},
		{name: "add protected sibling", tool: "file_edit", args: map[string]any{"action": "add", "path": filepath.Join(permissionsDir, "credential.txt"), "content": "blocked"}},
		{name: "structured patch state", tool: "file_edit", args: map[string]any{
			"action": "patch", "workdir": home,
			"patch": "*** Begin Patch\n*** Update File: permissions/state.json\n@@\n bogus\n*** End Patch",
		}},
		{name: "unified patch state", tool: "file_edit", args: map[string]any{
			"action": "patch", "workdir": home,
			"patch": "diff --git a/permissions/state.json b/permissions/state.json\n--- a/permissions/state.json\n+++ b/permissions/state.json\n@@ -1 +1 @@\n-x\n+y\n",
		}},
		{name: "delete home ancestor", tool: "file_edit", args: map[string]any{"action": "delete", "path": home, "recursive": true}},
		{name: "publish state", tool: "file_publish", args: map[string]any{"path": statePath}},
		{name: "publish home ancestor", tool: "file_publish", args: map[string]any{"path": home}},
		{name: "view state as image", tool: "view_image", args: map[string]any{"path": statePath}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, err := rt.Call(context.Background(), test.tool, test.args)
			if result != nil {
				t.Fatalf("protected operation result=%#v", result)
			}
			requirePermissionError(t, err, "PROTECTED_CONTROL_PATH")
		})
	}

	alias := filepath.Join(rt.ws.Root(), "permission-state-alias")
	if err := os.Symlink(permissionsDir, alias); err == nil {
		result, err := rt.Call(context.Background(), "read_file", map[string]any{"path": filepath.Join(alias, "state.json")})
		if result != nil {
			t.Fatalf("symlink protected read result=%#v", result)
		}
		requirePermissionError(t, err, "PROTECTED_CONTROL_PATH")
	}
}

func TestProtectedControlPathsAreSkippedFromAncestorListAndSearch(t *testing.T) {
	rt := newPermissionRuntime(t)
	permissionsDir := filepath.Dir(rt.permissions.StatePath())
	home := filepath.Dir(permissionsDir)
	visible := filepath.Join(home, "visible.txt")
	if err := os.WriteFile(visible, []byte("visible marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	listed, err := rt.Call(context.Background(), "list_dir", map[string]any{
		"path": home, "max_depth": 3, "include_hidden": true, "include_ignored": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var entries []map[string]any
	if err := remarshal(listed["entries"], &entries); err != nil {
		t.Fatal(err)
	}
	visibleFound := false
	for _, entry := range entries {
		path, _ := entry["path"].(string)
		if path == "visible.txt" {
			visibleFound = true
		}
		if path == "permissions" || strings.HasPrefix(path, "permissions/") {
			t.Fatalf("protected control path leaked through list_dir: %#v", entries)
		}
	}
	if !visibleFound {
		t.Fatalf("ordinary home file disappeared from list_dir: %#v", entries)
	}
	if partial, _ := listed["partial"].(bool); !partial {
		t.Fatalf("list_dir did not report skipped protected path: %#v", listed)
	}

	searched, err := rt.Call(context.Background(), "search_text", map[string]any{
		"path": home, "query": "global_mode", "include_hidden": true, "include_ignored": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var matches []map[string]any
	if err := remarshal(searched["matches"], &matches); err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("protected permission state leaked through search_text: %#v", matches)
	}

	searched, err = rt.Call(context.Background(), "search_text", map[string]any{
		"path": home, "query": "visible marker", "include_hidden": true, "include_ignored": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	matches = nil
	if err := remarshal(searched["matches"], &matches); err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0]["path"] != filepath.Clean(visible) {
		t.Fatalf("ordinary search result changed by protected path filtering: %#v", matches)
	}
}

func permissionDetailUint64(t *testing.T, details map[string]any, key string) uint64 {
	t.Helper()
	switch value := details[key].(type) {
	case uint64:
		return value
	case float64:
		return uint64(value)
	default:
		t.Fatalf("%s=%#v", key, details[key])
		return 0
	}
}
