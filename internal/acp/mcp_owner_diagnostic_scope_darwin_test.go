//go:build darwin

package acp

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func newMCPDiagnosticTestManager(t *testing.T) (*Manager, SessionRecord) {
	t.Helper()
	workspace := t.TempDir()
	manager, err := newTestManager(t.TempDir(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Errorf("close owned test adapter: %v", err)
		}
	})
	created, err := manager.NewSession(context.Background(), workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	return manager, created.Session
}

func TestMCPDiagnosticScopeOnlyLocalLoadedSnapshot(t *testing.T) {
	manager, record := newMCPDiagnosticTestManager(t)
	// An in-memory persisted-but-unloaded mapping is NOT evidence of native load.
	manager.mu.Lock()
	manager.sessions["persisted-CANARY"] = SessionRecord{ID: "persisted-CANARY", RemoteSessionID: "native-secret", Status: SessionReady}
	manager.remoteToLocal["native-secret"] = "persisted-CANARY"
	manager.mu.Unlock()
	called := 0
	err := manager.withMCPDiagnosticScope(context.Background(), func(ctx context.Context, snapshot diagnosticManagerScope) error {
		called++
		if ctx.Err() != nil || snapshot.adapter.PID <= 0 {
			t.Error("invalid manager diagnostic scope")
		}
		if !reflect.DeepEqual(snapshot.loaded, []diagnosticLoadedRef{{record.ID, record.RemoteSessionID}}) {
			t.Errorf("loaded catalog leaked persisted record: %#v", snapshot.loaded)
		}
		return nil
	})
	if err != nil || called != 1 {
		t.Fatalf("diagnostic failure: %v calls=%d", err, called)
	}
}

func TestMCPDiagnosticScopeRejectsDirtyBindingsAndRedacts(t *testing.T) {
	manager, record := newMCPDiagnosticTestManager(t)
	for _, tc := range []struct {
		name   string
		mutate func()
	}{
		{"remote_reverse_mismatch", func() { manager.remoteToLocal[record.RemoteSessionID] = "other" }},
		{"session_remote_mismatch", func() {
			v := manager.sessions[record.ID]
			v.RemoteSessionID = "CANARY-other"
			manager.sessions[record.ID] = v
		}},
		{"loaded_session_mismatch", func() { manager.loaded[record.ID] = sessionLifecycleResponse{SessionID: "CANARY-other"} }},
		{"transition", func() { manager.terminalSessions[record.ID] = SessionClosed }},
		{"closed_record", func() { v := manager.sessions[record.ID]; v.Status = SessionClosed; manager.sessions[record.ID] = v }},
		{"invalid_status", func() { r := manager.sessions[record.ID]; r.Status = SessionError; manager.sessions[record.ID] = r }},
		{"duplicate_remote", func() {
			manager.loaded["other"] = sessionLifecycleResponse{SessionID: record.RemoteSessionID}
			manager.sessions["other"] = SessionRecord{ID: "other", RemoteSessionID: record.RemoteSessionID, Status: SessionReady}
		}},
		{"missing_record", func() { delete(manager.sessions, record.ID) }},
		{"no_loaded", func() { delete(manager.loaded, record.ID) }},
		{"oversize", func() {
			for i := 0; i < diagnosticScopeMaxLoaded; i++ {
				manager.loaded[fmt.Sprintf("extra_%04d", i)] = sessionLifecycleResponse{}
			}
		}},
		{"wrong_pid", func() { manager.process.pid = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager.mu.Lock()
			originalSessions, originalRemote, originalLoaded, originalTerminal, originalPID := manager.sessions, manager.remoteToLocal, manager.loaded, manager.terminalSessions, manager.process.pid
			manager.sessions = make(map[string]SessionRecord, len(originalSessions))
			for k, v := range originalSessions {
				manager.sessions[k] = v
			}
			manager.remoteToLocal = make(map[string]string, len(originalRemote))
			for k, v := range originalRemote {
				manager.remoteToLocal[k] = v
			}
			manager.loaded = make(map[string]sessionLifecycleResponse, len(originalLoaded))
			for k, v := range originalLoaded {
				manager.loaded[k] = v
			}
			manager.terminalSessions = make(map[string]SessionStatus, len(originalTerminal))
			for k, v := range originalTerminal {
				manager.terminalSessions[k] = v
			}
			tc.mutate()
			manager.mu.Unlock()
			defer func() {
				manager.mu.Lock()
				manager.sessions, manager.remoteToLocal, manager.loaded, manager.terminalSessions, manager.process.pid = originalSessions, originalRemote, originalLoaded, originalTerminal, originalPID
				manager.mu.Unlock()
			}()
			called := false
			err := manager.withMCPDiagnosticScope(context.Background(), func(context.Context, diagnosticManagerScope) error { called = true; return nil })
			if !errors.Is(err, errMCPDiagnosticNotQualified) || called {
				t.Fatalf("bad binding accepted: %v called=%v", err, called)
			}
		})
	}
	if err := manager.withMCPDiagnosticScope(context.Background(), func(context.Context, diagnosticManagerScope) error { return errors.New("CANARY-secret-api-token") }); !errors.Is(err, errMCPDiagnosticNotQualified) || strings.Contains(err.Error(), "CANARY") {
		t.Fatalf("secret leaked in error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.withMCPDiagnosticScope(ctx, func(context.Context, diagnosticManagerScope) error { t.Fatal("cancelled callback invoked"); return nil }); !errors.Is(err, errMCPDiagnosticNotQualified) {
		t.Fatalf("cancelled scope accepted: %v", err)
	}
	if err := manager.withMCPDiagnosticScope(context.Background(), nil); !errors.Is(err, errMCPDiagnosticNotQualified) {
		t.Fatalf("nil callback accepted: %v", err)
	}
	if err := (*Manager)(nil).withMCPDiagnosticScope(context.Background(), func(context.Context, diagnosticManagerScope) error { return nil }); !errors.Is(err, errMCPDiagnosticNotQualified) {
		t.Fatalf("nil manager accepted: %v", err)
	}
}

func TestMCPDiagnosticScopeFencesConcurrentManagerMutation(t *testing.T) {
	manager, record := newMCPDiagnosticTestManager(t)
	entered, release, result := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		result <- manager.withMCPDiagnosticScope(context.Background(), func(_ context.Context, s diagnosticManagerScope) error {
			if len(s.loaded) != 1 || s.loaded[0].localSessionID != record.ID {
				return errors.New("bad mapping")
			}
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("scope did not enter")
	}
	writing, written := make(chan struct{}), make(chan struct{})
	go func() {
		close(writing)
		manager.mu.Lock()
		delete(manager.loaded, record.ID)
		manager.mu.Unlock()
		close(written)
	}()
	<-writing
	select {
	case <-written:
		t.Fatal("manager mutation escaped diagnostic fence")
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("scope did not return")
	}
	select {
	case <-written:
	case <-time.After(3 * time.Second):
		t.Fatal("writer blocked after scope")
	}
	if err := manager.withMCPDiagnosticScope(context.Background(), func(context.Context, diagnosticManagerScope) error { t.Fatal("unloaded mapping published"); return nil }); !errors.Is(err, errMCPDiagnosticNotQualified) {
		t.Fatalf("unloaded scope accepted: %v", err)
	}
}

func TestMCPDiagnosticScopeCancelDuringReadFailsClosed(t *testing.T) {
	manager, _ := newMCPDiagnosticTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := manager.withMCPDiagnosticScope(ctx, func(_ context.Context, _ diagnosticManagerScope) error { cancel(); return nil })
	if !errors.Is(err, errMCPDiagnosticNotQualified) {
		t.Fatalf("cancelled callback accepted: %v", err)
	}
}

func TestMCPDiagnosticScopeLostTransportOrExpiredContext(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		manager, _ := newMCPDiagnosticTestManager(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		err := manager.withMCPDiagnosticScope(ctx, func(ctx context.Context, _ diagnosticManagerScope) error {
			<-ctx.Done()
			return nil
		})
		if !errors.Is(err, errMCPDiagnosticNotQualified) {
			t.Fatalf("expired callback accepted: %v", err)
		}
	})
	t.Run("transport_disappears_during_callback", func(t *testing.T) {
		manager, _ := newMCPDiagnosticTestManager(t)
		err := manager.withMCPDiagnosticScope(context.Background(), func(_ context.Context, _ diagnosticManagerScope) error {
			// Only the fake test adapter's owned connection is closed.
			return manager.process.connection.Close()
		})
		if !errors.Is(err, errMCPDiagnosticNotQualified) {
			t.Fatalf("dead adapter accepted: %v", err)
		}
		called := false
		err = manager.withMCPDiagnosticScope(context.Background(), func(context.Context, diagnosticManagerScope) error { called = true; return nil })
		if !errors.Is(err, errMCPDiagnosticNotQualified) || called {
			t.Fatalf("closed adapter accepted: %v %v", err, called)
		}
	})
}

func TestNextEarlyMCPDiagnosticRequiresLiveAdapterButNotLoadedSession(t *testing.T) {
	manager, _ := newMCPDiagnosticTestManager(t)
	manager.mu.Lock()
	originallyLoaded := manager.loaded
	manager.loaded = make(map[string]sessionLifecycleResponse)
	manager.mu.Unlock()
	t.Cleanup(func() { manager.mu.Lock(); manager.loaded = originallyLoaded; manager.mu.Unlock() })
	called := 0
	if err := manager.withMCPDiagnosticScope(context.Background(), func(context.Context, diagnosticManagerScope) error { called++; return nil }); err == nil || called != 0 {
		t.Fatalf("regular scope accepted empty loaded Session: err=%v called=%d", err, called)
	}
	if err := manager.withMCPDiagnosticScopePolicy(context.Background(), true, func(_ context.Context, scope diagnosticManagerScope) error {
		called++
		if scope.adapter.PID <= 0 || len(scope.loaded) != 0 {
			t.Fatalf("invalid early signed Adapter scope: %+v", scope)
		}
		return nil
	}); err != nil || called != 1 {
		t.Fatalf("early scoped adapter evidence unavailable: err=%v called=%d", err, called)
	}
}
