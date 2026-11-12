package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func diagnosticsTestManager(now time.Time) *Manager {
	return &Manager{
		sessions:           map[string]SessionRecord{"s": {ID: "s", Status: SessionReady, LifecyclePolicy: LifecycleIdleManaged, LastActiveAt: now.Add(-MinIdleCloseAfter), IdleCloseAfterMS: MinIdleCloseAfter.Milliseconds()}},
		loaded:             map[string]sessionLifecycleResponse{"s": {}},
		activeRunBySession: make(map[string]string), interactions: make(map[string]*Interaction),
		sessionOperations: make(map[string]int), terminalSessions: make(map[string]SessionStatus),
		process: &agentProcess{connection: &Connection{closed: make(chan struct{})}},
	}
}

func TestDiagnosticsIdleManagedGuards(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name           string
		change         func(*Manager)
		idle, eligible bool
	}{
		{"exact deadline", func(m *Manager) {}, true, true},
		{"before deadline", func(m *Manager) { r := m.sessions["s"]; r.LastActiveAt = now; m.sessions["s"] = r }, true, false},
		{"persistent ready", func(m *Manager) { r := m.sessions["s"]; r.LifecyclePolicy = LifecyclePersistent; m.sessions["s"] = r }, false, false},
		{"ephemeral", func(m *Manager) { r := m.sessions["s"]; r.LifecyclePolicy = LifecycleEphemeral; m.sessions["s"] = r }, false, false},
		{"unloaded", func(m *Manager) { delete(m.loaded, "s") }, false, false},
		{"running", func(m *Manager) {
			r := m.sessions["s"]
			r.Status = SessionRunning
			m.sessions["s"] = r
			m.activeRunBySession["s"] = "run"
		}, false, false},
		{"active run guard", func(m *Manager) { m.activeRunBySession["s"] = "run" }, false, false},
		{"closed", func(m *Manager) { r := m.sessions["s"]; r.Status = SessionClosed; m.sessions["s"] = r }, false, false},
		{"pending interaction", func(m *Manager) { m.interactions["i"] = &Interaction{SessionID: "s", Status: InteractionPending} }, false, false},
		{"settled interaction", func(m *Manager) { m.interactions["i"] = &Interaction{SessionID: "s", Status: InteractionExpired} }, true, true},
		{"operation", func(m *Manager) { m.sessionOperations["s"] = 2 }, false, false},
		{"transition", func(m *Manager) { m.terminalSessions["s"] = SessionClosed }, false, false},
		{"manager closed", func(m *Manager) { m.closed = true }, false, false},
		{"no process", func(m *Manager) { m.process = nil }, false, false},
		{"no connection", func(m *Manager) { m.process.connection = nil }, false, false},
		{"dead connection", func(m *Manager) { close(m.process.connection.closed) }, false, false},
		{"invalid TTL", func(m *Manager) { r := m.sessions["s"]; r.IdleCloseAfterMS = 0; m.sessions["s"] = r }, false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := diagnosticsTestManager(now)
			test.change(m)
			idle, eligible := m.idleManagedStateLocked("s", m.sessions["s"], now)
			if idle != test.idle || eligible != test.eligible {
				t.Fatalf("idle=%v eligible=%v", idle, eligible)
			}
			snapshot := m.Diagnostics()
			entry := snapshot.Sessions[0]
			if entry.IdleManagedIdle != test.idle || entry.IdleManagedEligible != test.eligible {
				t.Fatalf("entry=%+v", entry)
			}
			if entry.ActiveRunID != m.activeRunBySession["s"] || entry.SessionOperations != m.sessionOperations["s"] || entry.PendingInteractions != m.pendingInteractionCountLocked("s") {
				t.Fatalf("work counts=%+v", entry)
			}
		})
	}
}

func TestDiagnosticsCountsPrivacyAndDetachedValues(t *testing.T) {
	now := time.Now().UTC()
	m := diagnosticsTestManager(now)
	m.sessions["a"] = SessionRecord{ID: "a", Status: SessionRunning, LifecyclePolicy: LifecyclePersistent}
	m.activeRunBySession["a"] = "run"
	m.loaded["a"] = sessionLifecycleResponse{}
	m.sessions["b"] = SessionRecord{ID: "b", Status: SessionClosed, LifecyclePolicy: LifecycleEphemeral, ClosedAt: &now, AutoCloseAttemptedAt: &now, ClosedReason: "idle_timeout", AutoCloseError: "Bearer capability-secret prompt-secret", Title: "prompt-secret"}
	m.sessions["c"] = SessionRecord{ID: "c", Status: SessionReady, LifecyclePolicy: LifecyclePersistent, ClosedReason: "prompt-secret"}
	m.loaded["c"] = sessionLifecycleResponse{}
	m.interactions["secret"] = &Interaction{SessionID: "a", Status: InteractionPending, ToolCall: map[string]any{"prompt": "prompt-secret"}}
	m.sessionOperations["a"] = 3
	before := m.sessions["b"]
	snapshot := m.Diagnostics()
	want := DiagnosticsCounts{Managed: 4, Loaded: 3, Running: 1, Ready: 2, Closed: 1, IdleManagedIdle: 1, IdleManagedEligible: 1, AutoCloseFailures: 1}
	if snapshot.Counts != want {
		t.Fatalf("counts=%+v want=%+v", snapshot.Counts, want)
	}
	if snapshot.Sessions[0].SessionID != "a" || snapshot.Sessions[3].SessionID != "s" {
		t.Fatal("unstable order")
	}
	entry := snapshot.Sessions[1]
	if entry.ClosedReason != "idle_timeout" || entry.AutoCloseError != "[redacted]" || !entry.ClosedAt.Equal(now) || !entry.AutoCloseAttemptedAt.Equal(now) {
		t.Fatalf("close metadata=%+v", entry)
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || snapshot.Sessions[2].ClosedReason != "[redacted]" {
		t.Fatal("payload content retained")
	}
	*entry.ClosedAt = time.Time{}
	*entry.AutoCloseAttemptedAt = time.Time{}
	snapshot.Sessions[0].SessionID = "changed"
	if !reflect.DeepEqual(m.sessions["b"], before) || !m.sessions["b"].ClosedAt.Equal(now) || m.Diagnostics().Sessions[0].SessionID != "a" {
		t.Fatal("snapshot aliases manager state")
	}
}

func TestDiagnosticsPersistedUnloadedIsPureObservation(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	store, err := newSessionStore(home, "diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	record := SessionRecord{ID: "persisted", RemoteSessionID: "remote", Agent: "diagnostics", CWD: cwd, Status: SessionReady, LifecyclePolicy: LifecycleIdleManaged, LastActiveAt: now.Add(-2 * MinIdleCloseAfter), IdleCloseAfterMS: MinIdleCloseAfter.Milliseconds(), CreatedAt: now, UpdatedAt: now}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(Options{Home: home, DefaultCWD: cwd, Agent: AgentSpec{Name: "diagnostics", Command: filepath.Join(cwd, "missing-adapter")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	path, err := store.path(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		s := m.Diagnostics()
		if s.Counts.Managed != 1 || s.Counts.Loaded != 0 || s.Counts.Ready != 1 || s.Counts.IdleManagedEligible != 0 || s.Sessions[0].AutoCloseAttemptedAt != nil {
			t.Fatalf("snapshot=%+v", s)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || m.process != nil || len(m.loaded) != 0 || len(m.terminalSessions) != 0 {
		t.Fatal("diagnostics mutated lifecycle or started adapter")
	}
}

func TestDiagnosticsConcurrentLifecycleState(t *testing.T) {
	m := diagnosticsTestManager(time.Now().UTC())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 500 {
			m.mu.Lock()
			r := m.sessions["s"]
			stamp := time.Now().UTC()
			r.AutoCloseAttemptedAt = &stamp
			if i%2 == 0 {
				r.Status = SessionRunning
				m.activeRunBySession["s"] = "run"
				m.sessionOperations["s"] = 1
				m.interactions["i"] = &Interaction{SessionID: "s", Status: InteractionPending}
				m.loaded["s"] = sessionLifecycleResponse{}
			} else {
				r.Status = SessionClosed
				r.ClosedAt = &stamp
				r.AutoCloseError = "failure"
				delete(m.activeRunBySession, "s")
				delete(m.sessionOperations, "s")
				delete(m.interactions, "i")
				delete(m.loaded, "s")
			}
			m.sessions["s"] = r
			m.mu.Unlock()
		}
	}()
	for range 500 {
		s := m.Diagnostics()
		e := s.Sessions[0]
		if e.Status == SessionRunning && (s.Counts.Running != 1 || e.ActiveRunID != "run" || !e.Loaded || e.PendingInteractions != 1 || e.SessionOperations != 1 || e.IdleManagedEligible) {
			t.Errorf("inconsistent running snapshot=%+v", s)
		}
		if e.Status == SessionClosed && (s.Counts.Closed != 1 || e.Loaded || e.ActiveRunID != "" || e.PendingInteractions != 0 || e.SessionOperations != 0) {
			t.Errorf("inconsistent closed snapshot=%+v", s)
		}
		if e.ClosedAt != nil {
			*e.ClosedAt = time.Time{}
		}
		if e.AutoCloseAttemptedAt != nil {
			*e.AutoCloseAttemptedAt = time.Time{}
		}
	}
	wg.Wait()
}
