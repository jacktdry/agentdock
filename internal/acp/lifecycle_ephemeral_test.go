package acp

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func newLifecycleTestManager(t *testing.T, promptMode string, provider SessionMCPProvider) *Manager {
	t.Helper()
	workspace := t.TempDir()
	manager, err := NewManager(Options{
		Home: t.TempDir(), DefaultCWD: workspace,
		Agent: AgentSpec{Name: "helper", Command: os.Args[0], Args: []string{"-test.run=TestACPHelperProcess"}, Environment: map[string]string{
			"GO_WANT_ACP_HELPER": "1", "GO_ACP_HELPER_AGENT_INFO_NAME": "helper-acp", "GO_ACP_HELPER_PROMPT_MODE": promptMode,
		}},
		MaxConcurrentRuns: 2, InteractionTimeout: 3 * time.Second, SessionMCPProvider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	return manager
}

func waitLifecycleSession(t *testing.T, manager *Manager, id string, predicate func(SessionRecord) bool) SessionRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		record, err := manager.InspectSession(id)
		if err == nil && predicate(record) {
			return record
		}
		time.Sleep(10 * time.Millisecond)
	}
	record, err := manager.InspectSession(id)
	t.Fatalf("session %s did not reach lifecycle state: record=%+v err=%v", id, record, err)
	return SessionRecord{}
}

func TestEphemeralPromptTerminalAutoClosesAndReleasesCapabilities(t *testing.T) {
	provider := &recordingSessionMCPProvider{}
	manager := newLifecycleTestManager(t, "codex_recovered", provider)
	created, err := manager.NewSessionWithLifecycle(context.Background(), "", nil, SessionLifecycleOptions{Policy: LifecycleEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	started, err := manager.StartPrompt(context.Background(), created.Session.ID, "finish bounded work")
	if err != nil {
		t.Fatal(err)
	}
	settled := waitForSettledRun(t, manager, started.RunID)
	if settled.Status != RunCompleted {
		t.Fatalf("run=%+v", settled)
	}
	closed := waitLifecycleSession(t, manager, created.Session.ID, func(record SessionRecord) bool { return record.Status == SessionClosed })
	if closed.ClosedReason != "ephemeral_prompt_terminal" || closed.AutoCloseAttemptedAt == nil || closed.AutoCloseError != "" || closed.ClosedAt == nil {
		t.Fatalf("closed=%+v", closed)
	}
	_, releases := provider.snapshot()
	if len(releases) != 1 || releases[0] != created.Session.ID {
		t.Fatalf("capability releases=%v", releases)
	}
}

func TestPersistentPromptTerminalDoesNotAutoClose(t *testing.T) {
	provider := &recordingSessionMCPProvider{}
	manager := newLifecycleTestManager(t, "codex_recovered", provider)
	created, err := manager.NewSession(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	started, err := manager.StartPrompt(context.Background(), created.Session.ID, "keep session")
	if err != nil {
		t.Fatal(err)
	}
	if settled := waitForSettledRun(t, manager, started.RunID); settled.Status != RunCompleted {
		t.Fatalf("run=%+v", settled)
	}
	time.Sleep(100 * time.Millisecond)
	record, err := manager.InspectSession(created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != SessionReady || record.AutoCloseAttemptedAt != nil || record.ClosedAt != nil {
		t.Fatalf("persistent session=%+v", record)
	}
	_, releases := provider.snapshot()
	if len(releases) != 0 {
		t.Fatalf("persistent capability releases=%v", releases)
	}
}

func TestEphemeralAutoCloseFailureStaysObservableAndOpen(t *testing.T) {
	provider := &recordingSessionMCPProvider{}
	manager := newLifecycleTestManager(t, "close_failure", provider)
	created, err := manager.NewSessionWithLifecycle(context.Background(), "", nil, SessionLifecycleOptions{Policy: LifecycleEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	started, err := manager.StartPrompt(context.Background(), created.Session.ID, "trigger close failure")
	if err != nil {
		t.Fatal(err)
	}
	_ = waitForSettledRun(t, manager, started.RunID)
	failed := waitLifecycleSession(t, manager, created.Session.ID, func(record SessionRecord) bool { return record.AutoCloseError != "" })
	if failed.Status == SessionClosed || failed.ClosedAt != nil || failed.AutoCloseAttemptedAt == nil || !strings.Contains(failed.AutoCloseError, "close failed") {
		t.Fatalf("failed auto-close=%+v", failed)
	}
	_, releases := provider.snapshot()
	if len(releases) != 0 {
		t.Fatalf("failed close released host capability: %v", releases)
	}
}

func TestEphemeralAutoCloseSkipsWhenPolicyChangesAfterSettlement(t *testing.T) {
	manager := newLifecycleTestManager(t, "", nil)
	created, err := manager.NewSessionWithLifecycle(context.Background(), "", nil, SessionLifecycleOptions{Policy: LifecycleEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	record := created.Session
	record.LastActiveAt = time.Now().UTC()
	manager.mu.Lock()
	current := manager.sessions[record.ID]
	current.LastActiveAt = record.LastActiveAt
	current.LifecyclePolicy = LifecyclePersistent
	if err := manager.store.Save(current); err != nil {
		manager.mu.Unlock()
		t.Fatal(err)
	}
	manager.sessions[record.ID] = current
	manager.mu.Unlock()
	manager.autoCloseSession(record.ID, "ephemeral_prompt_terminal", record.LastActiveAt)
	after, err := manager.InspectSession(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status == SessionClosed || after.AutoCloseAttemptedAt != nil || after.LifecyclePolicy != LifecyclePersistent {
		t.Fatalf("policy-change session=%+v", after)
	}
}
