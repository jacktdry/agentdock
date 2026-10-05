package permission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newPermissionStore(t *testing.T, root, epoch string) *Store {
	t.Helper()
	store, err := NewStore(root, epoch)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func approvalFixture(t *testing.T, store *Store) CreateApprovalInput {
	t.Helper()
	policy := store.Policy()
	binding := PermissionBinding{
		Principal: AuthorizationPrincipal{
			Kind:          "oauth",
			ID:            "client-1",
			Authenticated: true,
			Stable:        true,
		},
		RuntimeEpoch:     store.RuntimeEpoch(),
		Source:           "mcp",
		WorkspaceRoot:    "/trusted/workspace",
		WorkspaceID:      "workspace-1",
		TrustedWorkspace: true,
		ACPSessionID:     "acp-1",
		Provider:         "provider-1",
	}
	facts := PermissionFacts{
		EffectsKnown:          true,
		Filesystem:            FileWrite,
		WorkspaceBound:        true,
		WorkspaceRuleEligible: true,
		Tool:                  "file_edit",
		Action:                "patch",
		Binding:               binding,
	}
	prepared := PreparedRequest{
		Fingerprint:    "sha256:prepared-request-1",
		Binding:        binding,
		Generations:    map[string]string{"provider": "7", "workspace": "2"},
		Tool:           facts.Tool,
		Action:         facts.Action,
		PolicyRevision: policy.Revision,
		RuntimeEpoch:   store.RuntimeEpoch(),
	}
	return CreateApprovalInput{
		Audit:     AuditBinding{CallID: "call-original"},
		Binding:   binding,
		Facts:     facts,
		Prepared:  prepared,
		Summary:   "edit one workspace file",
		Scope:     "trusted workspace",
		Reason:    "write requires approval",
		RuleID:    "review-side-effects",
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}
}

func mustCreateApproval(t *testing.T, store *Store, mutate func(*CreateApprovalInput)) (ApprovalRecord, CreateApprovalInput) {
	t.Helper()
	input := approvalFixture(t, store)
	if mutate != nil {
		mutate(&input)
	}
	record, err := store.CreateApproval(context.Background(), input)
	if err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}
	return record, input
}

func mutationFor(store *Store, record ApprovalRecord) Mutation {
	return Mutation{
		ApprovalID:      record.ID,
		ApprovalVersion: record.Version,
		PolicyRevision:  store.Policy().Revision,
		Actor:           "desktop-local-user",
	}
}

func readStateFile(t *testing.T, path string) State {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	return state
}

func TestStoreBootstrapAndFileMode(t *testing.T) {
	root := t.TempDir()
	store := newPermissionStore(t, root, "epoch-1")
	state, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if state.RuntimeEpoch != "epoch-1" || state.Policy.GlobalMode != Full || state.Policy.Revision != 1 {
		t.Fatalf("state = %+v", state)
	}
	info, err := os.Stat(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode=%o", info.Mode().Perm())
	}
	if filepath.Dir(store.StatePath()) != filepath.Join(root, "permissions") {
		t.Fatalf("state path=%s", store.StatePath())
	}
}

func TestStoreExistingCorruptStateFailsClosed(t *testing.T) {
	root := t.TempDir()
	store := newPermissionStore(t, root, "epoch-1")
	if err := os.WriteFile(store.StatePath(), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(root, "epoch-2"); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("corrupt state error=%v", err)
	}
}

func TestStoreOversizedExistingStateFailsClosed(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "permissions")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(strings.Repeat("x", MaxStateBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(root, "epoch-1"); !errors.Is(err, ErrStateTooLarge) {
		t.Fatalf("oversized state error=%v", err)
	}
}

func TestPriorEpochPendingAndApprovedOnceInvalidate(t *testing.T) {
	t.Run("pending", func(t *testing.T) {
		root := t.TempDir()
		store := newPermissionStore(t, root, "epoch-1")
		record, _ := mustCreateApproval(t, store, nil)

		reopened := newPermissionStore(t, root, "epoch-2")
		got, err := reopened.Approval(record.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != Invalidated || got.DecidedBy != "core-recovery" {
			t.Fatalf("recovered record=%+v", got)
		}
	})

	t.Run("approved-once", func(t *testing.T) {
		root := t.TempDir()
		store := newPermissionStore(t, root, "epoch-1")
		record, _ := mustCreateApproval(t, store, nil)
		approved, err := store.ApproveOnce(context.Background(), mutationFor(store, record))
		if err != nil {
			t.Fatal(err)
		}
		if approved.Status != ApprovedOnce {
			t.Fatalf("approved=%+v", approved)
		}

		reopened := newPermissionStore(t, root, "epoch-2")
		got, err := reopened.Approval(record.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != Invalidated {
			t.Fatalf("recovered record=%+v", got)
		}
	})
}

func TestConsumedPendingDispatchRecoversUnknown(t *testing.T) {
	root := t.TempDir()
	store := newPermissionStore(t, root, "epoch-1")
	record, input := mustCreateApproval(t, store, nil)
	if _, err := store.ApproveOnce(context.Background(), mutationFor(store, record)); err != nil {
		t.Fatal(err)
	}
	consumed, ok, err := store.ConsumeOnce(context.Background(), ConsumeInput{Prepared: input.Prepared, RetryCallID: "call-retry"})
	if err != nil || !ok {
		t.Fatalf("consume ok=%v err=%v", ok, err)
	}
	if consumed.DispatchOutcome != DispatchPending {
		t.Fatalf("consumed=%+v", consumed)
	}

	reopened := newPermissionStore(t, root, "epoch-2")
	got, err := reopened.Approval(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != Consumed || got.DispatchOutcome != Unknown {
		t.Fatalf("recovered=%+v", got)
	}
}

func TestCreateApprovalBoundsAndPendingLimit(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "epoch-1")
	input := approvalFixture(t, store)
	input.Summary = strings.Repeat("x", MaxSummaryBytes+1)
	if _, err := store.CreateApproval(context.Background(), input); !errors.Is(err, ErrState) {
		t.Fatalf("summary bound error=%v", err)
	}

	now := time.Now().UTC()
	store.state.History = make([]ApprovalRecord, MaxPending)
	for i := range store.state.History {
		store.state.History[i] = dummyApprovalRecord(i, store.epoch, store.state.Policy.Revision, now)
	}
	if _, err := store.CreateApproval(context.Background(), approvalFixture(t, store)); !errors.Is(err, ErrLimit) {
		t.Fatalf("pending limit error=%v", err)
	}
}

func dummyApprovalRecord(index int, epoch string, revision uint64, now time.Time) ApprovalRecord {
	return ApprovalRecord{
		SchemaVersion:   SchemaVersion,
		Version:         1,
		ID:              fmt.Sprintf("approval_%03d", index),
		Tool:            "file_edit",
		Summary:         "summary",
		Scope:           "scope",
		Reason:          "reason",
		PolicyRevision:  revision,
		RuntimeEpoch:    epoch,
		Status:          Pending,
		CreatedAt:       now,
		ExpiresAt:       now.Add(time.Hour),
		GrantKind:       "none",
		DispatchOutcome: NotDispatched,
	}
}

func TestPruneHistoryKeepsLiveApprovals(t *testing.T) {
	now := time.Now().UTC()
	history := make([]ApprovalRecord, 0, MaxHistory+20)
	for i := 0; i < MaxHistory+20; i++ {
		record := dummyApprovalRecord(i, "epoch", 1, now.Add(time.Duration(i)*time.Second))
		if i < 10 {
			record.Status = Pending
		} else {
			record.Status = Rejected
		}
		history = append(history, record)
	}
	pruned := pruneHistory(history)
	if len(pruned) != MaxHistory {
		t.Fatalf("len=%d", len(pruned))
	}
	for i := 0; i < 10; i++ {
		if _, ok := approvalByID(pruned, fmt.Sprintf("approval_%03d", i)); !ok {
			t.Fatalf("live approval %d was pruned", i)
		}
	}
}

func TestReplacePolicyStateTooLargeDoesNotCommit(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "epoch-1")
	before := store.Policy()
	next := before
	next.Rules = []Rule{{
		ID:     "huge",
		Tool:   "file_edit",
		Effect: Ask,
		Reason: strings.Repeat("x", MaxStateBytes),
	}}
	if _, err := store.ReplacePolicy(before.Revision, next); !errors.Is(err, ErrStateTooLarge) {
		t.Fatalf("replace error=%v", err)
	}
	after := store.Policy()
	if after.Revision != before.Revision || len(after.Rules) != 0 {
		t.Fatalf("policy mutated despite failed persist: %+v", after)
	}
}

func TestApproveOnceRequiresStablePrincipal(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "epoch-1")
	record, _ := mustCreateApproval(t, store, func(input *CreateApprovalInput) {
		input.Binding.Principal.Stable = false
		input.Prepared.Binding = input.Binding
		input.Facts.Binding = input.Binding
	})
	if _, err := store.ApproveOnce(context.Background(), mutationFor(store, record)); !errors.Is(err, ErrNotEligible) {
		t.Fatalf("approve once error=%v", err)
	}
}

func TestConsumeOnceExactMatchAndMismatches(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "epoch-1")
	record, input := mustCreateApproval(t, store, nil)
	if _, err := store.ApproveOnce(context.Background(), mutationFor(store, record)); err != nil {
		t.Fatal(err)
	}

	variants := []func(*PreparedRequest){
		func(p *PreparedRequest) { p.Fingerprint = "different" },
		func(p *PreparedRequest) { p.Binding.Principal.ID = "other-client" },
		func(p *PreparedRequest) { p.Binding.WorkspaceID = "other-workspace" },
		func(p *PreparedRequest) { p.Generations["provider"] = "8" },
		func(p *PreparedRequest) { p.PolicyRevision++ },
		func(p *PreparedRequest) { p.RuntimeEpoch = "other-epoch" },
	}
	for i, mutate := range variants {
		prepared := clonePrepared(input.Prepared)
		mutate(&prepared)
		if _, ok, err := store.ConsumeOnce(context.Background(), ConsumeInput{Prepared: prepared, RetryCallID: fmt.Sprintf("retry-mismatch-%d", i)}); err != nil || ok {
			t.Fatalf("mismatch %d consumed=%v err=%v", i, ok, err)
		}
	}

	consumed, ok, err := store.ConsumeOnce(context.Background(), ConsumeInput{Prepared: input.Prepared, RetryCallID: "retry-exact"})
	if err != nil || !ok {
		t.Fatalf("exact consume ok=%v err=%v", ok, err)
	}
	if consumed.Status != Consumed || consumed.RetryCallID != "retry-exact" || consumed.DispatchOutcome != DispatchPending {
		t.Fatalf("consumed=%+v", consumed)
	}
	if _, ok, err := store.ConsumeOnce(context.Background(), ConsumeInput{Prepared: input.Prepared, RetryCallID: "retry-second"}); err != nil || ok {
		t.Fatalf("second consume ok=%v err=%v", ok, err)
	}
}

func TestConsumeOnceIsAtMostOnceUnderConcurrency(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "epoch-1")
	record, input := mustCreateApproval(t, store, nil)
	if _, err := store.ApproveOnce(context.Background(), mutationFor(store, record)); err != nil {
		t.Fatal(err)
	}

	var successes atomic.Int32
	var failures atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, ok, err := store.ConsumeOnce(context.Background(), ConsumeInput{
				Prepared:    input.Prepared,
				RetryCallID: fmt.Sprintf("retry-%d", index),
			})
			if err != nil {
				failures.Add(1)
				return
			}
			if ok {
				successes.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if failures.Load() != 0 || successes.Load() != 1 {
		t.Fatalf("successes=%d failures=%d", successes.Load(), failures.Load())
	}
}

func TestConsumePersistsBeforeReturningAndSettlesOutcome(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "epoch-1")
	record, input := mustCreateApproval(t, store, nil)
	if _, err := store.ApproveOnce(context.Background(), mutationFor(store, record)); err != nil {
		t.Fatal(err)
	}
	consumed, ok, err := store.ConsumeOnce(context.Background(), ConsumeInput{Prepared: input.Prepared, RetryCallID: "retry-persisted"})
	if err != nil || !ok {
		t.Fatalf("consume ok=%v err=%v", ok, err)
	}

	onDisk := readStateFile(t, store.StatePath())
	persisted, found := approvalByID(onDisk.History, consumed.ID)
	if !found || persisted.Status != Consumed || persisted.RetryCallID != "retry-persisted" || persisted.DispatchOutcome != DispatchPending {
		t.Fatalf("persisted=%+v found=%v", persisted, found)
	}

	settled, err := store.SettleDispatch(context.Background(), consumed.ID, Failed)
	if err != nil {
		t.Fatal(err)
	}
	if settled.DispatchOutcome != Failed {
		t.Fatalf("settled=%+v", settled)
	}
	onDisk = readStateFile(t, store.StatePath())
	persisted, _ = approvalByID(onDisk.History, consumed.ID)
	if persisted.DispatchOutcome != Failed {
		t.Fatalf("disk outcome=%+v", persisted)
	}
}

func TestRestartNeverReconstructsOneShotGrant(t *testing.T) {
	root := t.TempDir()
	store := newPermissionStore(t, root, "epoch-1")
	record, input := mustCreateApproval(t, store, nil)
	if _, err := store.ApproveOnce(context.Background(), mutationFor(store, record)); err != nil {
		t.Fatal(err)
	}

	reopened := newPermissionStore(t, root, "epoch-2")
	if _, ok, err := reopened.ConsumeOnce(context.Background(), ConsumeInput{Prepared: input.Prepared, RetryCallID: "retry-after-restart"}); err != nil || ok {
		t.Fatalf("restart consume ok=%v err=%v", ok, err)
	}
	got, err := reopened.Approval(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != Invalidated {
		t.Fatalf("record=%+v", got)
	}
}

func TestApproveWorkspaceAtomicallyAddsRuleAndSettles(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "epoch-1")
	record, _ := mustCreateApproval(t, store, nil)
	beforeRevision := store.Policy().Revision

	approved, policy, err := store.ApproveWorkspace(context.Background(), WorkspaceGrantInput{
		Mutation: mutationFor(store, record),
	})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != ApprovedWorkspace || approved.GrantKind != "workspace" || approved.GrantedRuleID == "" {
		t.Fatalf("approved=%+v", approved)
	}
	if policy.Revision != beforeRevision+1 || approved.GrantedPolicyRevision != policy.Revision {
		t.Fatalf("policy=%+v approved=%+v", policy, approved)
	}
	if !policyHasScope(policy, "workspace-1") {
		t.Fatalf("workspace scope missing: %+v", policy)
	}

	foundRule := false
	for _, rule := range policy.Rules {
		if rule.ID == approved.GrantedRuleID && rule.WorkspaceID == "workspace-1" && rule.Effect == Allow {
			foundRule = true
		}
	}
	if !foundRule {
		t.Fatalf("granted rule missing: %+v", policy.Rules)
	}

	onDisk := readStateFile(t, store.StatePath())
	diskApproval, ok := approvalByID(onDisk.History, approved.ID)
	if !ok || diskApproval.Status != ApprovedWorkspace || onDisk.Policy.Revision != policy.Revision {
		t.Fatalf("atomic disk state policy=%+v approval=%+v", onDisk.Policy, diskApproval)
	}
}

func TestApproveWorkspaceRejectsOpaqueUntrustedAndIneligible(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*CreateApprovalInput)
	}{
		{"opaque", func(input *CreateApprovalInput) {
			input.Facts.OpaqueProviderExecution = true
		}},
		{"untrusted", func(input *CreateApprovalInput) {
			input.Binding.TrustedWorkspace = false
			input.Prepared.Binding = input.Binding
			input.Facts.Binding = input.Binding
		}},
		{"ineligible", func(input *CreateApprovalInput) {
			input.Facts.WorkspaceRuleEligible = false
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newPermissionStore(t, t.TempDir(), "epoch-1")
			record, _ := mustCreateApproval(t, store, tc.mutate)
			if _, _, err := store.ApproveWorkspace(context.Background(), WorkspaceGrantInput{
				Mutation: mutationFor(store, record),
			}); !errors.Is(err, ErrNotEligible) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestApproveWorkspaceCannotOverrideGlobalAsk(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "epoch-1")
	policy := store.Policy()
	policy.Rules = []Rule{{ID: "global-ask", Tool: "file_edit", Action: "patch", Effect: Ask}}
	if _, err := store.ReplacePolicy(policy.Revision, policy); err != nil {
		t.Fatal(err)
	}

	record, _ := mustCreateApproval(t, store, nil)
	before := store.Policy()
	if _, _, err := store.ApproveWorkspace(context.Background(), WorkspaceGrantInput{
		Mutation: mutationFor(store, record),
	}); !errors.Is(err, ErrNotEligible) {
		t.Fatalf("error=%v", err)
	}
	after := store.Policy()
	if after.Revision != before.Revision || len(after.Rules) != len(before.Rules) {
		t.Fatalf("failed workspace grant mutated policy: before=%+v after=%+v", before, after)
	}
	got, err := store.Approval(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != Pending {
		t.Fatalf("failed workspace grant settled approval: %+v", got)
	}
}

func TestPolicyChangeInvalidatesLiveApprovals(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "epoch-1")
	record, input := mustCreateApproval(t, store, nil)
	if _, err := store.ApproveOnce(context.Background(), mutationFor(store, record)); err != nil {
		t.Fatal(err)
	}

	policy := store.Policy()
	policy.GlobalMode = Rules
	if _, err := store.ReplacePolicy(policy.Revision, policy); err != nil {
		t.Fatal(err)
	}
	got, err := store.Approval(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != Invalidated {
		t.Fatalf("record=%+v", got)
	}
	if _, ok, err := store.ConsumeOnce(context.Background(), ConsumeInput{Prepared: input.Prepared, RetryCallID: "retry"}); err != nil || ok {
		t.Fatalf("consume after policy change ok=%v err=%v", ok, err)
	}
}

func TestApprovalExpiry(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "epoch-1")
	base := time.Now().UTC()
	store.now = func() time.Time { return base }
	record, _ := mustCreateApproval(t, store, func(input *CreateApprovalInput) {
		input.ExpiresAt = base.Add(time.Minute)
	})
	store.now = func() time.Time { return base.Add(2 * time.Minute) }
	if err := store.SweepExpired(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := store.Approval(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != Expired {
		t.Fatalf("record=%+v", got)
	}
}

func TestHistoryReturnsNewestFirst(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "epoch-1")
	base := time.Now().UTC()
	for i := 0; i < 3; i++ {
		current := base.Add(time.Duration(i) * time.Second)
		store.now = func() time.Time { return current }
		record, _ := mustCreateApproval(t, store, nil)
		if _, err := store.Reject(context.Background(), mutationFor(store, record)); err != nil {
			t.Fatal(err)
		}
	}
	history, err := store.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("history len=%d", len(history))
	}
	for i := 1; i < len(history); i++ {
		if history[i-1].CreatedAt.Before(history[i].CreatedAt) {
			t.Fatalf("history not newest-first: %+v", history)
		}
	}
}
