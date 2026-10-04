package acp

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestLegacySessionRecordNormalizesToPersistentWithoutSchemaMigration(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	store, err := newSessionStore(home, "helper")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	legacy := map[string]any{
		"schema_version":    sessionSchemaVersion,
		"id":                "acps_legacy",
		"agent":             "helper",
		"remote_session_id": "remote-legacy",
		"cwd":               workspace,
		"status":            string(SessionReady),
		"created_at":        now,
		"updated_at":        now,
	}
	encoded, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.path("acps_legacy")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get("acps_legacy")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SchemaVersion != sessionSchemaVersion || loaded.LifecyclePolicy != LifecyclePersistent || !loaded.LastActiveAt.Equal(now) || loaded.IdleCloseAfterMS != 0 {
		t.Fatalf("normalized legacy record=%+v", loaded)
	}
	if err := store.Save(loaded); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted["schema_version"] != float64(sessionSchemaVersion) || persisted["lifecycle_policy"] != string(LifecyclePersistent) || persisted["last_active_at"] == nil {
		t.Fatalf("rewritten legacy record=%s", raw)
	}
}

func TestSessionLifecycleOptionsValidation(t *testing.T) {
	persistent, err := NormalizeSessionLifecycleOptions(SessionLifecycleOptions{})
	if err != nil || persistent.Policy != LifecyclePersistent || persistent.IdleCloseAfter != 0 {
		t.Fatalf("persistent=%+v err=%v", persistent, err)
	}
	idle, err := NormalizeSessionLifecycleOptions(SessionLifecycleOptions{Policy: LifecycleIdleManaged})
	if err != nil || idle.IdleCloseAfter != DefaultIdleCloseAfter {
		t.Fatalf("idle=%+v err=%v", idle, err)
	}
	for _, options := range []SessionLifecycleOptions{
		{Policy: "future"},
		{Policy: LifecyclePersistent, IdleCloseAfter: time.Minute},
		{Policy: LifecycleIdleManaged, IdleCloseAfter: MinIdleCloseAfter - time.Millisecond},
		{Policy: LifecycleIdleManaged, IdleCloseAfter: MaxIdleCloseAfter + time.Millisecond},
	} {
		if _, err := NormalizeSessionLifecycleOptions(options); err == nil || errorCode(err) != "ACP_SESSION_LIFECYCLE_INVALID" {
			t.Fatalf("options=%+v err=%v", options, err)
		}
	}
}

func TestSessionCreationLifecycleDefaultsAndOverrides(t *testing.T) {
	manager, err := newTestManager(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()
	created, err := manager.NewSession(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if created.Session.LifecyclePolicy != LifecyclePersistent || created.Session.IdleCloseAfterMS != 0 || created.Session.LastActiveAt.IsZero() {
		t.Fatalf("default session=%+v", created.Session)
	}
	idle, err := manager.NewSessionWithLifecycle(context.Background(), "", nil, SessionLifecycleOptions{Policy: LifecycleIdleManaged})
	if err != nil {
		t.Fatal(err)
	}
	if idle.Session.LifecyclePolicy != LifecycleIdleManaged || idle.Session.IdleCloseAfterMS != DefaultIdleCloseAfter.Milliseconds() {
		t.Fatalf("idle session=%+v", idle.Session)
	}
	forked, err := manager.ForkSessionWithLifecycle(context.Background(), created.Session.ID, "", nil, SessionLifecycleOptions{Policy: LifecycleEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	if forked.Session.LifecyclePolicy != LifecycleEphemeral || forked.Session.IdleCloseAfterMS != 0 {
		t.Fatalf("forked session=%+v", forked.Session)
	}
}

func TestSetSessionLifecycleDoesNotStartAdapterProcess(t *testing.T) {
	manager, err := newTestManager(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()
	persisted, err := manager.persistNewSession(sessionLifecycleResponse{SessionID: "remote-lifecycle"}, manager.opts.DefaultCWD, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.RLock()
	processBefore := manager.process
	manager.mu.RUnlock()
	if processBefore != nil {
		t.Fatal("test setup unexpectedly started adapter process")
	}
	updated, err := manager.SetSessionLifecycle(persisted.ID, SessionLifecycleOptions{Policy: LifecycleIdleManaged, IdleCloseAfter: 2 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.RLock()
	processAfter := manager.process
	manager.mu.RUnlock()
	if processAfter != nil {
		t.Fatal("lifecycle metadata update started adapter process")
	}
	if updated.LifecyclePolicy != LifecycleIdleManaged || updated.IdleCloseAfterMS != (2*time.Hour).Milliseconds() || updated.LastActiveAt.IsZero() {
		t.Fatalf("updated=%+v", updated)
	}
	reloaded, err := manager.store.Get(persisted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.LifecyclePolicy != updated.LifecyclePolicy || reloaded.IdleCloseAfterMS != updated.IdleCloseAfterMS {
		t.Fatalf("reloaded=%+v updated=%+v", reloaded, updated)
	}
}

func TestLifecyclePolicyPersistsAcrossManagerRestartWithoutLoadingAdapter(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	manager, err := newTestManager(home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	created, err := manager.NewSessionWithLifecycle(context.Background(), workspace, nil, SessionLifecycleOptions{
		Policy: LifecycleIdleManaged, IdleCloseAfter: 2 * time.Hour,
	})
	if err != nil {
		_ = manager.Close()
		t.Fatal(err)
	}
	id := created.Session.ID
	remoteID := created.Session.RemoteSessionID
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := newTestManager(home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restarted.Close() }()
	record, err := restarted.InspectSession(id)
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != id || record.RemoteSessionID != remoteID || record.LifecyclePolicy != LifecycleIdleManaged || record.IdleCloseAfterMS != (2*time.Hour).Milliseconds() {
		t.Fatalf("restarted lifecycle record=%+v", record)
	}
	diagnostics := restarted.Diagnostics()
	if diagnostics.Counts.Managed != 1 || diagnostics.Counts.Loaded != 0 || len(diagnostics.Sessions) != 1 || diagnostics.Sessions[0].Loaded {
		t.Fatalf("restart diagnostics=%+v", diagnostics)
	}
	restarted.mu.RLock()
	process := restarted.process
	restarted.mu.RUnlock()
	if process != nil {
		t.Fatal("restart inspection loaded adapter process")
	}
}
