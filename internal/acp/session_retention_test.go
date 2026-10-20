package acp

import (
	"os"
	"testing"
	"time"
)

func TestSweepStaleManagedSessions_PreviewAndProtection(t *testing.T) {
	store, err := newSessionStore(t.TempDir(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	old := now.Add(-8 * 24 * time.Hour)
	recent := now.Add(-6 * 24 * time.Hour)
	m := &Manager{
		store:              store,
		sessions:           map[string]SessionRecord{},
		remoteToLocal:      map[string]string{},
		activeRunBySession: map[string]string{},
		sessionOperations:  map[string]int{},
		terminalSessions:   map[string]SessionStatus{},
		loaded:             map[string]sessionLifecycleResponse{},
		interactions:       map[string]*Interaction{},
		projections:        map[string]SessionProjection{},
	}
	add := func(id string, status SessionStatus, active time.Time, protected bool) {
		t.Helper()
		r := SessionRecord{
			ID: id, Agent: "codex", RemoteSessionID: "remote-" + id,
			CWD: t.TempDir(), Status: status, LifecyclePolicy: LifecyclePersistent,
			LastActiveAt: active, UpdatedAt: active, CreatedAt: active,
			RetentionProtected: protected,
		}
		if err := store.Save(r); err != nil {
			t.Fatal(err)
		}
		r.SchemaVersion = sessionSchemaVersion
		m.sessions[id] = r
		m.remoteToLocal[r.RemoteSessionID] = id
	}
	add("old-ready", SessionReady, old, false)
	add("old-closed", SessionClosed, old, false)
	add("old-error", SessionError, old, false)
	add("recent", SessionClosed, recent, false)
	add("protected", SessionClosed, old, true)
	add("running", SessionRunning, old, false)
	add("loaded", SessionReady, old, false)
	m.loaded["loaded"] = sessionLifecycleResponse{SessionID: "remote-loaded"}
	add("operation", SessionReady, old, false)
	m.sessionOperations["operation"] = 1
	add("run", SessionReady, old, false)
	m.activeRunBySession["run"] = "active-run"
	add("transition", SessionReady, old, false)
	m.terminalSessions["transition"] = SessionClosed
	add("approval", SessionReady, old, false)
	m.interactions["permission"] = &Interaction{SessionID: "approval", Status: InteractionPending}
	add("boundary", SessionClosed, now.Add(-ManagedSessionRetention), false)
	add("boundary-new", SessionClosed, now.Add(-ManagedSessionRetention).Add(time.Second), false)

	preview, err := m.SweepStaleManagedSessions(now, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Scanned != 13 || preview.Candidates != 4 || preview.Removed != 0 {
		t.Fatalf("unexpected dry run report: %+v", preview)
	}
	if _, err := store.Get("old-ready"); err != nil {
		t.Fatalf("dry run mutated store: %v", err)
	}

	report, err := m.SweepStaleManagedSessions(now, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Candidates != 4 || report.Removed != 4 {
		t.Fatalf("unexpected sweep report: %+v", report)
	}
	for _, id := range []string{"old-ready", "old-closed", "old-error", "boundary"} {
		if _, err := store.Get(id); !os.IsNotExist(err) && errorCode(err) != "ACP_SESSION_NOT_FOUND" {
			t.Errorf("session %s must be removed from store, got %v", id, err)
		}
		if _, ok := m.sessions[id]; ok {
			t.Errorf("session %s still mapped", id)
		}
		if _, ok := m.remoteToLocal["remote-"+id]; ok {
			t.Errorf("remote mapping %s still exists", id)
		}
	}
	for _, id := range []string{"recent", "protected", "running", "loaded", "operation", "run", "transition", "approval", "boundary-new"} {
		if _, err := store.Get(id); err != nil {
			t.Errorf("protected session %s vanished: %v", id, err)
		}
	}
	again, err := m.SweepStaleManagedSessions(now, false)
	if err != nil || again.Removed != 0 {
		t.Fatalf("repeat sweep should be a no-op: %+v, %v", again, err)
	}
}

func TestSweepStaleManagedSessionsDoesNotStartAdapter(t *testing.T) {
	store, err := newSessionStore(t.TempDir(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{
		store:              store,
		sessions:           map[string]SessionRecord{},
		remoteToLocal:      map[string]string{},
		sessionOperations:  map[string]int{},
		activeRunBySession: map[string]string{},
		terminalSessions:   map[string]SessionStatus{},
		loaded:             map[string]sessionLifecycleResponse{},
		interactions:       map[string]*Interaction{},
	}
	r, err := m.SweepStaleManagedSessions(time.Now(), false)
	if err != nil || r.Removed != 0 || m.process != nil {
		t.Fatalf("unexpected adapter side effect: %+v, %v", r, err)
	}
}
