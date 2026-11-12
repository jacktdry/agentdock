package acp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type flakyReleaseSessionMCPProvider struct {
	mu       sync.Mutex
	calls    map[string]int
	failOnce bool
}

func (p *flakyReleaseSessionMCPProvider) Servers(_ context.Context, sessionID, _ string, _ []string) ([]SessionMCPServer, error) {
	return []SessionMCPServer{{Name: "host", Type: "http", URL: "http://127.0.0.1/host", Headers: []SessionMCPHeader{{Name: "Authorization", Value: "Bearer " + sessionID}}}}, nil
}

func (p *flakyReleaseSessionMCPProvider) ReleaseSession(_ context.Context, sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.calls == nil {
		p.calls = make(map[string]int)
	}
	p.calls[sessionID]++
	if p.failOnce && p.calls[sessionID] == 1 {
		return errors.New("injected host capability release failure")
	}
	return nil
}

func (p *flakyReleaseSessionMCPProvider) count(sessionID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[sessionID]
}

func terminalEventCount(result PromptEventsResult) int {
	count := 0
	for _, event := range result.Events {
		switch event.Type {
		case "completed", "cancelled", "interrupted", "error":
			count++
		}
	}
	return count
}

func TestManualCloseRacingActivePromptSettlesRunOnceAndReleasesOnce(t *testing.T) {
	provider := &recordingSessionMCPProvider{}
	manager := newLifecycleTestManager(t, "claude_steer_fallback", provider)
	created, err := manager.NewSession(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	started, err := manager.StartPrompt(context.Background(), created.Session.ID, "block until close")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	closed, err := manager.CloseSession(ctx, created.Session.ID)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != SessionClosed || closed.ClosedAt == nil {
		t.Fatalf("closed=%+v", closed)
	}
	settled := waitForSettledRun(t, manager, started.RunID)
	if settled.Status != RunCancelled || terminalEventCount(settled) != 1 {
		t.Fatalf("settled=%+v terminal_events=%d", settled, terminalEventCount(settled))
	}
	_, releases := provider.snapshot()
	if len(releases) != 1 || releases[0] != created.Session.ID {
		t.Fatalf("releases=%v", releases)
	}
}

func TestManualCloseAdapterFailureRollsBackAndCanRetry(t *testing.T) {
	provider := &recordingSessionMCPProvider{}
	manager := newLifecycleTestManager(t, "close_once_failure", provider)
	created, err := manager.NewSession(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CloseSession(context.Background(), created.Session.ID); err == nil {
		t.Fatal("first close failure hidden")
	}
	record, err := manager.InspectSession(created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status == SessionClosed || record.ClosedAt != nil {
		t.Fatalf("adapter close failure marked session closed: %+v", record)
	}
	manager.mu.RLock()
	_, transitioning := manager.terminalSessions[created.Session.ID]
	manager.mu.RUnlock()
	if transitioning {
		t.Fatal("failed close left terminal marker")
	}
	closed, err := manager.CloseSession(context.Background(), created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != SessionClosed {
		t.Fatalf("retry closed=%+v", closed)
	}
	_, releases := provider.snapshot()
	if len(releases) != 1 || releases[0] != created.Session.ID {
		t.Fatalf("releases=%v", releases)
	}
}

func TestAutoCloseFailureManualRetryClearsCurrentFailure(t *testing.T) {
	provider := &recordingSessionMCPProvider{}
	manager := newLifecycleTestManager(t, "close_once_failure", provider)
	created, err := manager.NewSessionWithLifecycle(context.Background(), "", nil, SessionLifecycleOptions{Policy: LifecycleEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	manager.autoCloseSession(created.Session.ID, "ephemeral_prompt_terminal", created.Session.LastActiveAt)
	failed, err := manager.InspectSession(created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status == SessionClosed || failed.AutoCloseAttemptedAt == nil || failed.AutoCloseError == "" {
		t.Fatalf("failed auto-close=%+v", failed)
	}
	closed, err := manager.CloseSession(context.Background(), created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != SessionClosed || closed.AutoCloseError != "" || closed.AutoCloseAttemptedAt == nil || closed.ClosedReason != "manual" {
		t.Fatalf("manual retry=%+v", closed)
	}
	_, releases := provider.snapshot()
	if len(releases) != 1 || releases[0] != created.Session.ID {
		t.Fatalf("releases=%v", releases)
	}
}

func TestConcurrentManualAndAutoCloseReleaseCapabilityOnce(t *testing.T) {
	provider := &recordingSessionMCPProvider{}
	manager := newLifecycleTestManager(t, "codex_recovered", provider)
	const iterations = 20
	ids := make([]string, 0, iterations)
	for i := 0; i < iterations; i++ {
		created, err := manager.NewSessionWithLifecycle(context.Background(), "", nil, SessionLifecycleOptions{Policy: LifecycleEphemeral})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, created.Session.ID)
		var wg sync.WaitGroup
		wg.Add(2)
		go func(id string, lastActive time.Time) {
			defer wg.Done()
			manager.autoCloseSession(id, "ephemeral_prompt_terminal", lastActive)
		}(created.Session.ID, created.Session.LastActiveAt)
		manualErr := make(chan error, 1)
		go func(id string) {
			defer wg.Done()
			_, closeErr := manager.CloseSession(context.Background(), id)
			manualErr <- closeErr
		}(created.Session.ID)
		wg.Wait()
		if err := <-manualErr; err != nil && errorCode(err) != "ACP_SESSION_TRANSITION" {
			t.Fatalf("iteration %d manual close err=%v", i, err)
		}
		record := waitLifecycleSession(t, manager, created.Session.ID, func(record SessionRecord) bool { return record.Status == SessionClosed })
		if record.ClosedAt == nil {
			t.Fatalf("iteration %d record=%+v", i, record)
		}
	}
	_, releases := provider.snapshot()
	counts := make(map[string]int)
	for _, id := range releases {
		counts[id]++
	}
	for _, id := range ids {
		if counts[id] != 1 {
			t.Fatalf("session %s release count=%d all=%v", id, counts[id], releases)
		}
	}
}

func TestClosedSessionRetriesFailedHostCapabilityRelease(t *testing.T) {
	provider := &flakyReleaseSessionMCPProvider{failOnce: true}
	manager := newLifecycleTestManager(t, "codex_recovered", provider)
	created, err := manager.NewSession(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := manager.CloseSession(context.Background(), created.Session.ID)
	if err == nil || errorCode(err) != "ACP_SESSION_MCP_RELEASE_FAILED" {
		t.Fatalf("first close err=%#v closed=%+v", err, closed)
	}
	if closed.Status != SessionClosed || closed.ClosedAt == nil || provider.count(created.Session.ID) != 1 {
		t.Fatalf("first close state=%+v release_calls=%d", closed, provider.count(created.Session.ID))
	}
	closed, err = manager.CloseSession(context.Background(), created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != SessionClosed || provider.count(created.Session.ID) != 2 {
		t.Fatalf("retry close=%+v release_calls=%d", closed, provider.count(created.Session.ID))
	}
}
