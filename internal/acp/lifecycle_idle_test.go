package acp

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func ageLifecycleSession(t *testing.T, manager *Manager, id string, lastActive time.Time) SessionRecord {
	t.Helper()
	manager.mu.Lock()
	record, exists := manager.sessions[id]
	if !exists {
		manager.mu.Unlock()
		t.Fatalf("session %s missing", id)
	}
	record.LastActiveAt = lastActive.UTC()
	record.UpdatedAt = record.LastActiveAt
	if err := manager.store.Save(record); err != nil {
		manager.mu.Unlock()
		t.Fatal(err)
	}
	manager.sessions[id] = record
	manager.mu.Unlock()
	return record
}

func TestIdleManagedSweepClosesExpiredLoadedSessionAndCanResume(t *testing.T) {
	provider := &recordingSessionMCPProvider{}
	manager := newLifecycleTestManager(t, "codex_recovered", provider)
	created, err := manager.NewSessionWithLifecycle(context.Background(), "", nil, SessionLifecycleOptions{Policy: LifecycleIdleManaged, IdleCloseAfter: MinIdleCloseAfter})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	ageLifecycleSession(t, manager, created.Session.ID, now.Add(-2*MinIdleCloseAfter))
	if err := manager.SweepIdleManaged(now); err != nil {
		t.Fatal(err)
	}
	closed, err := manager.InspectSession(created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != SessionClosed || closed.ClosedReason != "idle_timeout" || closed.AutoCloseAttemptedAt == nil || closed.AutoCloseError != "" {
		t.Fatalf("closed=%+v", closed)
	}
	_, releases := provider.snapshot()
	if len(releases) != 1 || releases[0] != created.Session.ID {
		t.Fatalf("releases=%v", releases)
	}

	resumed, err := manager.EnsureSessionActive(context.Background(), created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Session.Status != SessionReady || resumed.Session.ClosedAt != nil || resumed.Session.ClosedReason != "" || resumed.Session.LifecyclePolicy != LifecycleIdleManaged {
		t.Fatalf("resumed=%+v", resumed.Session)
	}
	serverCalls, _ := provider.snapshot()
	if len(serverCalls) < 2 || serverCalls[len(serverCalls)-1] != created.Session.ID {
		t.Fatalf("server calls=%v", serverCalls)
	}
}

func TestIdleManagedSweepRespectsTTLAndActiveWorkGuards(t *testing.T) {
	manager := newLifecycleTestManager(t, "codex_recovered", nil)
	created, err := manager.NewSessionWithLifecycle(context.Background(), "", nil, SessionLifecycleOptions{Policy: LifecycleIdleManaged, IdleCloseAfter: MinIdleCloseAfter})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	ageLifecycleSession(t, manager, created.Session.ID, now.Add(-MinIdleCloseAfter/2))
	if err := manager.SweepIdleManaged(now); err != nil {
		t.Fatal(err)
	}
	if record, _ := manager.InspectSession(created.Session.ID); record.Status != SessionReady || record.AutoCloseAttemptedAt != nil {
		t.Fatalf("closed before TTL: %+v", record)
	}

	ageLifecycleSession(t, manager, created.Session.ID, now.Add(-2*MinIdleCloseAfter))
	manager.mu.Lock()
	manager.interactions["pending-idle"] = &Interaction{ID: "pending-idle", SessionID: created.Session.ID, Status: InteractionPending}
	manager.mu.Unlock()
	if err := manager.SweepIdleManaged(now); err != nil {
		t.Fatal(err)
	}
	if record, _ := manager.InspectSession(created.Session.ID); record.Status != SessionReady || record.AutoCloseAttemptedAt != nil {
		t.Fatalf("pending interaction did not block idle close: %+v", record)
	}
	manager.mu.Lock()
	delete(manager.interactions, "pending-idle")
	manager.sessionOperations[created.Session.ID] = 1
	manager.mu.Unlock()
	if err := manager.SweepIdleManaged(now); err != nil {
		t.Fatal(err)
	}
	if record, _ := manager.InspectSession(created.Session.ID); record.Status != SessionReady || record.AutoCloseAttemptedAt != nil {
		t.Fatalf("tracked operation did not block idle close: %+v", record)
	}
	manager.mu.Lock()
	delete(manager.sessionOperations, created.Session.ID)
	manager.operationsCond.Broadcast()
	manager.mu.Unlock()
	if err := manager.SweepIdleManaged(now); err != nil {
		t.Fatal(err)
	}
	if record, _ := manager.InspectSession(created.Session.ID); record.Status != SessionClosed {
		t.Fatalf("eligible idle session not closed: %+v", record)
	}
}

func TestIdleManagedSweepDoesNotStartAdapterForUnloadedPersistedSession(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	manager, err := newTestManager(home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	created, err := manager.NewSessionWithLifecycle(context.Background(), "", nil, SessionLifecycleOptions{Policy: LifecycleIdleManaged, IdleCloseAfter: MinIdleCloseAfter})
	if err != nil {
		t.Fatal(err)
	}
	ageLifecycleSession(t, manager, created.Session.ID, time.Now().UTC().Add(-2*MinIdleCloseAfter))
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewManager(Options{
		Home: home, DefaultCWD: workspace,
		Agent: AgentSpec{Name: "helper", Command: filepath.Join(t.TempDir(), "missing-adapter")},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restarted.Close() }()
	if err := restarted.SweepIdleManaged(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	restarted.mu.RLock()
	process := restarted.process
	_, loaded := restarted.loaded[created.Session.ID]
	restarted.mu.RUnlock()
	if process != nil || loaded {
		t.Fatalf("idle sweep started/reloaded adapter: process=%v loaded=%v", process != nil, loaded)
	}
	record, err := restarted.InspectSession(created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != SessionReady || record.AutoCloseAttemptedAt != nil {
		t.Fatalf("unloaded session changed=%+v", record)
	}
}

func TestIdleManagedSweepDoesNotRestartDeadAdapter(t *testing.T) {
	manager := newLifecycleTestManager(t, "codex_recovered", nil)
	created, err := manager.NewSessionWithLifecycle(context.Background(), "", nil, SessionLifecycleOptions{Policy: LifecycleIdleManaged, IdleCloseAfter: MinIdleCloseAfter})
	if err != nil {
		t.Fatal(err)
	}
	ageLifecycleSession(t, manager, created.Session.ID, time.Now().UTC().Add(-2*MinIdleCloseAfter))
	manager.mu.RLock()
	process := manager.process
	manager.mu.RUnlock()
	if process == nil {
		t.Fatal("adapter process missing")
	}
	if err := process.Close(); err != nil {
		t.Fatal(err)
	}
	if err := manager.SweepIdleManaged(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	manager.mu.RLock()
	current := manager.process
	manager.mu.RUnlock()
	if current != process {
		t.Fatal("idle sweep replaced dead adapter process")
	}
	record, _ := manager.InspectSession(created.Session.ID)
	if record.Status != SessionReady || record.AutoCloseAttemptedAt != nil {
		t.Fatalf("dead-adapter session changed=%+v", record)
	}
}

func TestIdleManagedSweepIsBoundedPerPass(t *testing.T) {
	manager := newLifecycleTestManager(t, "codex_recovered", nil)
	now := time.Now().UTC()
	ids := make([]string, 0, maxIdleClosePerSweep+1)
	for i := 0; i < maxIdleClosePerSweep+1; i++ {
		created, err := manager.NewSessionWithLifecycle(context.Background(), "", nil, SessionLifecycleOptions{Policy: LifecycleIdleManaged, IdleCloseAfter: MinIdleCloseAfter})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, created.Session.ID)
		ageLifecycleSession(t, manager, created.Session.ID, now.Add(-2*MinIdleCloseAfter))
	}
	if err := manager.SweepIdleManaged(now); err != nil {
		t.Fatal(err)
	}
	closed := 0
	for _, id := range ids {
		record, _ := manager.InspectSession(id)
		if record.Status == SessionClosed {
			closed++
		}
	}
	if closed != maxIdleClosePerSweep {
		t.Fatalf("closed=%d want=%d", closed, maxIdleClosePerSweep)
	}
	if err := manager.SweepIdleManaged(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	closed = 0
	for _, id := range ids {
		record, _ := manager.InspectSession(id)
		if record.Status == SessionClosed {
			closed++
		}
	}
	if closed != len(ids) {
		t.Fatalf("second pass closed=%d want=%d", closed, len(ids))
	}
}

func TestIdleManagedSweepFailureStaysOpenAndObservable(t *testing.T) {
	manager := newLifecycleTestManager(t, "close_failure", nil)
	created, err := manager.NewSessionWithLifecycle(context.Background(), "", nil, SessionLifecycleOptions{Policy: LifecycleIdleManaged, IdleCloseAfter: MinIdleCloseAfter})
	if err != nil {
		t.Fatal(err)
	}
	ageLifecycleSession(t, manager, created.Session.ID, time.Now().UTC().Add(-2*MinIdleCloseAfter))
	if err := manager.SweepIdleManaged(time.Now().UTC()); err == nil {
		t.Fatal("idle close failure was hidden")
	}
	record := waitLifecycleSession(t, manager, created.Session.ID, func(record SessionRecord) bool { return record.AutoCloseError != "" })
	if record.Status == SessionClosed || record.ClosedAt != nil || record.AutoCloseAttemptedAt == nil {
		t.Fatalf("failed idle close=%+v", record)
	}
}

func TestLifecycleSweeperStopsWithManager(t *testing.T) {
	manager, err := newTestManager(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	done := manager.lifecycleDone
	if done == nil {
		t.Fatal("lifecycle sweeper was not started")
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("lifecycle sweeper did not stop with manager")
	}
}
