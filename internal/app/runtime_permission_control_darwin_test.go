//go:build darwin

package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/permission"
)

type desktopPolicyResult struct {
	result Result
	err    error
}

func confirmedDesktopPolicyUpdate(t *testing.T, rt *Runtime, mode string) (string, permission.ControlMutationRequest, string) {
	t.Helper()
	policy := rt.permissions.Policy()
	policy.GlobalMode = mode
	request := permission.ControlMutationRequest{
		Kind: permission.ControlMutationUpdatePolicy, PolicyRevision: policy.Revision, Policy: &policy,
	}
	credential := rt.DesktopPermissionControlCredential()
	challenge, err := rt.BeginPermissionConfirmation(credential, request)
	if err != nil {
		t.Fatal(err)
	}
	return credential, request, challenge.ID
}

func awaitDesktopPolicyResult(t *testing.T, done <-chan desktopPolicyResult) desktopPolicyResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("Desktop policy mutation did not finish")
		return desktopPolicyResult{}
	}
}

// Caller holds a read lock: a failed TryRLock proves a writer is queued.
func awaitDesktopPolicyWriter(t *testing.T, rt *Runtime, done <-chan desktopPolicyResult) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for rt.lifecycleMu.TryRLock() {
		rt.lifecycleMu.RUnlock()
		select {
		case result := <-done:
			t.Fatalf("policy mutation returned inside native lifecycle fence: %v", result.err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("confirmed policy mutation did not reach the dispatch fence")
		}
		runtime.Gosched()
	}
}

func TestNextDesktopPolicyMutationSerializesWithNativeLifecycle(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "commit-after-dispatch"
		if canceled {
			name = "cancel-while-waiting"
		}
		t.Run(name, func(t *testing.T) {
			rt, profile, config, principalCtx := nextCodexAuthorizationFixture(t)
			if rt.nextPermissionOwner == nil {
				t.Fatal("fixture lacks the real source-only Next permission owner")
			}
			before := replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Full })
			diskBefore, err := os.ReadFile(rt.permissions.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			credential, request, confirmation := confirmedDesktopPolicyUpdate(t, rt, permission.ReadOnly)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Native dispatch holds this same mutex exclusively. A read hold
			// also excludes the policy writer and lets TryRLock observe its
			// pending write lock deterministically, without sleeps or hooks.
			rt.lifecycleMu.RLock()
			unlock := rt.lifecycleMu.RUnlock
			defer func() { unlock() }()
			done := make(chan desktopPolicyResult, 1)
			go func() {
				result, err := rt.ApplyPermissionControlMutation(ctx, credential, confirmation, request)
				done <- desktopPolicyResult{result, err}
			}()
			awaitDesktopPolicyWriter(t, rt, done)
			if policy := rt.permissions.Policy(); policy.Revision != before.Revision || policy.GlobalMode != before.GlobalMode {
				t.Fatalf("policy committed while lifecycle fence held: %+v", policy)
			}
			disk, err := os.ReadFile(rt.permissions.StatePath())
			if err != nil || !bytes.Equal(disk, diskBefore) {
				t.Fatalf("policy persisted while lifecycle fence held: %v", err)
			}
			// Real Store admissions remain independent of the lifecycle fence
			// even while an authenticated Desktop policy writer is queued.
			for _, action := range []string{"prepare", "attach"} {
				finish, revision, err := rt.admitNextCodexMCP(principalCtx, config, action)
				if err != nil || revision != before.Revision {
					t.Fatalf("%s admission under lifecycle fence: revision=%d err=%v", action, revision, err)
				}
				finish(nil)
			}
			if canceled {
				cancel()
			}
			rt.lifecycleMu.RUnlock()
			unlock = func() {}
			outcome := awaitDesktopPolicyResult(t, done)
			if canceled {
				if !errors.Is(outcome.err, context.Canceled) || outcome.result != nil {
					t.Fatalf("canceled update result=%v err=%v", outcome.result, outcome.err)
				}
				if policy := rt.permissions.Policy(); policy.Revision != before.Revision || policy.GlobalMode != before.GlobalMode {
					t.Fatalf("canceled policy committed: %+v", policy)
				}
			} else {
				if outcome.err != nil {
					t.Fatal(outcome.err)
				}
				policy, ok := outcome.result["policy"].(permission.Policy)
				if !ok || policy.Revision != before.Revision+1 || policy.GlobalMode != permission.ReadOnly {
					t.Fatalf("incorrect committed policy return: %#v", outcome.result)
				}
				// Subsequent private native operations must use the new policy.
				_, err := rt.prepareNextCodexMCPRegistry(principalCtx, profile, nextCodexTestKey(), config)
				requirePermissionError(t, err, "PERMISSION_DENIED")
				requirePermissionError(t, rt.attachNextCodexInstrumentedMCP(principalCtx, config), "PERMISSION_DENIED")
				assertNoNextCodexStartup(t, rt)
			}
			if err := rt.permissions.VerifyDiskState(); err != nil {
				t.Fatalf("policy memory/disk inconsistency: %v", err)
			}
			if _, err := rt.ApplyPermissionControlMutation(context.Background(), credential, confirmation, request); !errors.Is(err, permission.ErrConfirmationNotFound) {
				t.Fatalf("confirmed mutation was replayable: %v", err)
			}
		})
	}
}

func TestDesktopWorkspacePolicyMutationLifecycleFence(t *testing.T) {
	for _, variant := range []string{"source-only-next", "stable", "ordinary-next"} {
		t.Run(variant, func(t *testing.T) {
			var rt *Runtime
			if variant == "source-only-next" {
				rt, _, _, _ = nextCodexAuthorizationFixture(t)
				if rt.nextPermissionOwner == nil {
					t.Fatal("fixture lacks source-only Next permission ownership")
				}
			} else {
				t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
				if variant == "stable" {
					t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "stable")
				}
				t.Setenv(nextCodexMCPRegistrationEnv, "")
				rt = newPermissionRuntime(t)
				if rt.nextPermissionOwner != nil {
					t.Fatal("ordinary runtime unexpectedly owns native authority")
				}
			}
			before := replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Rules })
			// A pre-existing Core-qualified workspace approval is the input to
			// Desktop control. Current runtime file_edit has no WorkspaceBound
			// attestation; never forge eligibility via the public tool path.
			// This fixture verifies the mutation's lifecycle fence, not the
			// provenance of a production workspace grant.
			target := filepath.Join(rt.ws.Root(), "workspace-grant.txt")
			binding := permission.PermissionBinding{
				Principal:    permission.AuthorizationPrincipal{Kind: "test", ID: "workspace-control", Authenticated: true, Stable: true},
				RuntimeEpoch: rt.permissions.RuntimeEpoch(), Source: "acp_bridge",
				WorkspaceRoot: rt.ws.Root(), WorkspaceID: trustedWorkspacePermissionID(rt.ws.Root()),
				TrustedWorkspace: true, ACPSessionID: "fixture-session", Provider: "fixture-codex",
			}
			facts := permission.PermissionFacts{
				EffectsKnown: true, Filesystem: permission.FileWrite, WorkspaceBound: true,
				WorkspaceRuleEligible: true, Tool: "file_edit", Action: "patch", Binding: binding,
			}
			prepared := permission.PreparedRequest{
				Fingerprint: "sha256:workspace-source-only-fixture", Binding: binding,
				Generations: map[string]string{}, Tool: facts.Tool, Action: facts.Action,
				PolicyRevision: before.Revision, RuntimeEpoch: rt.permissions.RuntimeEpoch(),
			}
			pending, err := rt.permissions.CreateApproval(context.Background(), permission.CreateApprovalInput{
				Audit:   permission.AuditBinding{CallID: "fixture-source-only-workspace"},
				Binding: binding, Facts: facts, Prepared: prepared,
				Summary: "fixture workspace patch", Scope: "trusted workspace", Reason: "test policy review",
				RuleID: "rules-review-side-effects", ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			})
			ctx := context.Background()
			id := pending.ID
			if err != nil || !pending.CanApproveWorkspace {
				t.Fatalf("workspace approval unavailable: %+v %v", pending, err)
			}
			request := permission.ControlMutationRequest{
				Kind: permission.ControlMutationApproveWorkspace, ApprovalID: id,
				ApprovalVersion: pending.Version, PolicyRevision: before.Revision,
			}
			credential := rt.DesktopPermissionControlCredential()
			challenge, err := rt.BeginPermissionConfirmation(credential, request)
			if err != nil {
				t.Fatal(err)
			}
			diskBefore, err := os.ReadFile(rt.permissions.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			rt.lifecycleMu.RLock()
			unlock := rt.lifecycleMu.RUnlock
			defer func() { unlock() }()
			done := make(chan desktopPolicyResult, 1)
			go func() {
				result, err := rt.ApplyPermissionControlMutation(ctx, credential, challenge.ID, request)
				done <- desktopPolicyResult{result, err}
			}()
			if rt.nextPermissionOwner != nil {
				awaitDesktopPolicyWriter(t, rt, done)
				if rt.permissions.Policy().Revision != before.Revision {
					t.Fatal("workspace grant changed policy inside native lifecycle fence")
				}
				disk, err := os.ReadFile(rt.permissions.StatePath())
				if err != nil || !bytes.Equal(disk, diskBefore) {
					t.Fatalf("workspace grant persisted inside native lifecycle fence: %v", err)
				}
				rt.lifecycleMu.RUnlock()
				unlock = func() {}
			}
			// Stable/ordinary Next must finish while the lifecycle lock is held.
			outcome := awaitDesktopPolicyResult(t, done)
			if outcome.err != nil {
				t.Fatal(outcome.err)
			}
			policy, ok := outcome.result["policy"].(permission.Policy)
			if !ok || policy.Revision != before.Revision+1 || len(policy.Rules) != len(before.Rules)+1 {
				t.Fatalf("incorrect workspace policy return: %#v", outcome.result)
			}
			record, ok := outcome.result["approval"].(permission.ApprovalRecord)
			if !ok || record.Status != permission.ApprovedWorkspace || record.GrantedPolicyRevision != policy.Revision || record.DecidedBy != permission.DesktopControlActor {
				t.Fatalf("incorrect workspace approval return: %#v", outcome.result)
			}
			if err := rt.permissions.VerifyDiskState(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("workspace approval dispatched the original operation: %v", err)
			}
		})
	}
}

func TestNextDesktopPolicyMutationRejectsAfterClose(t *testing.T) {
	rt, _, _, _ := nextCodexAuthorizationFixture(t)
	credential, request, confirmation := confirmedDesktopPolicyUpdate(t, rt, permission.ReadOnly)
	before := rt.permissions.Policy().Revision
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := rt.ApplyPermissionControlMutation(context.Background(), credential, confirmation, request)
	requirePermissionErrorClosing := err != nil && result == nil
	var toolErr *ToolError
	if !requirePermissionErrorClosing || !errors.As(err, &toolErr) || toolErr.Code != "RUNTIME_CLOSING" {
		t.Fatalf("closed runtime policy mutation result=%v err=%v", result, err)
	}
	if rt.permissions.Policy().Revision != before {
		t.Fatal("closed runtime committed policy")
	}
}

func TestDesktopPolicyMutationWithoutNativeOwnerDoesNotWaitForLifecycle(t *testing.T) {
	for _, variant := range []string{"stable", "next"} {
		t.Run(variant, func(t *testing.T) {
			t.Setenv("AGENTDOCK_DESKTOP_VARIANT", variant)
			t.Setenv(nextCodexMCPRegistrationEnv, "")
			rt := newPermissionRuntime(t)
			if rt.nextPermissionOwner != nil {
				t.Fatal("ordinary runtime unexpectedly owns native source-only authority")
			}
			rt.lifecycleMu.Lock()
			defer rt.lifecycleMu.Unlock()
			credential, request, confirmation := confirmedDesktopPolicyUpdate(t, rt, permission.ReadOnly)
			done := make(chan desktopPolicyResult, 1)
			go func() {
				result, err := rt.ApplyPermissionControlMutation(nil, credential, confirmation, request)
				done <- desktopPolicyResult{result, err}
			}()
			if outcome := awaitDesktopPolicyResult(t, done); outcome.err != nil || outcome.result["policy"] == nil {
				t.Fatalf("ordinary Desktop policy update result=%v err=%v", outcome.result, outcome.err)
			}
		})
	}
}
