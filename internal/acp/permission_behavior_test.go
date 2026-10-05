package acp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPermissionExplicitRejectOptionIsReturned(t *testing.T) {
	manager := newPermissionTestManager(time.Second)
	resultCh := startPermissionRequest(t, manager, permissionParams(map[string]any{"title": "write"}))
	interaction := waitForPendingInteraction(t, manager)
	settled, err := manager.RespondInteraction(context.Background(), interaction.ID, "reject-once", false)
	if err != nil {
		t.Fatal(err)
	}
	if settled.Status != InteractionResponded {
		t.Fatalf("interaction status = %s", settled.Status)
	}
	result := <-resultCh
	assertPermissionOutcome(t, result.value, result.err, "selected", "reject-once")
}

func TestPermissionExplicitCancellationIsReturned(t *testing.T) {
	manager := newPermissionTestManager(time.Second)
	resultCh := startPermissionRequest(t, manager, permissionParams(map[string]any{"title": "write"}))
	interaction := waitForPendingInteraction(t, manager)
	settled, err := manager.RespondInteraction(context.Background(), interaction.ID, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if settled.Status != InteractionCancelled {
		t.Fatalf("interaction status = %s", settled.Status)
	}
	result := <-resultCh
	assertPermissionOutcome(t, result.value, result.err, "cancelled", "")
}

func TestPermissionOptionSelectionRequiresContinuationAdmission(t *testing.T) {
	manager := newPermissionTestManager(time.Second)
	blocked := errors.New("continuation blocked")
	var captured PermissionContinuation
	manager.opts.PermissionContinuationAdmission = func(_ context.Context, continuation PermissionContinuation) (PermissionContinuationFinish, error) {
		captured = continuation
		return nil, blocked
	}
	resultCh := startPermissionRequest(t, manager, permissionParams(map[string]any{"title": "write", "path": "demo.txt"}))
	interaction := waitForPendingInteraction(t, manager)

	if _, err := manager.RespondInteraction(context.Background(), interaction.ID, "reject-once", false); !errors.Is(err, blocked) {
		t.Fatalf("provider-labelled reject bypassed admission: %v", err)
	}
	if captured.InteractionID != interaction.ID || captured.SessionID != "local-session" || captured.WorkspaceRoot != "/workspace" ||
		captured.OptionID != "reject-once" || captured.ToolCall["title"] != "write" {
		t.Fatalf("continuation admission input=%#v", captured)
	}
	if current, err := manager.InspectInteraction(interaction.ID); err != nil || current.Status != InteractionPending {
		t.Fatalf("blocked interaction status=%#v err=%v", current, err)
	}
	select {
	case result := <-resultCh:
		t.Fatalf("provider resumed before admission: %#v", result)
	default:
	}

	finished := false
	manager.opts.PermissionContinuationAdmission = func(_ context.Context, continuation PermissionContinuation) (PermissionContinuationFinish, error) {
		if continuation.OptionID != "reject-once" {
			t.Fatalf("retry continuation=%#v", continuation)
		}
		return func(err error) {
			if err != nil {
				t.Fatalf("allowed continuation finish err=%v", err)
			}
			finished = true
		}, nil
	}
	settled, err := manager.RespondInteraction(context.Background(), interaction.ID, "reject-once", false)
	if err != nil {
		t.Fatal(err)
	}
	if settled.Status != InteractionResponded || !finished {
		t.Fatalf("allowed continuation settled=%#v finished=%v", settled, finished)
	}
	result := <-resultCh
	assertPermissionOutcome(t, result.value, result.err, "selected", "reject-once")
}

func TestPermissionCancellationBypassesContinuationAdmission(t *testing.T) {
	manager := newPermissionTestManager(time.Second)
	admissionCalls := 0
	manager.opts.PermissionContinuationAdmission = func(context.Context, PermissionContinuation) (PermissionContinuationFinish, error) {
		admissionCalls++
		return nil, errors.New("must not be called for cancellation")
	}
	resultCh := startPermissionRequest(t, manager, permissionParams(map[string]any{"title": "write"}))
	interaction := waitForPendingInteraction(t, manager)
	settled, err := manager.RespondInteraction(context.Background(), interaction.ID, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if admissionCalls != 0 || settled.Status != InteractionCancelled {
		t.Fatalf("cancel admission calls=%d settled=%#v", admissionCalls, settled)
	}
	result := <-resultCh
	assertPermissionOutcome(t, result.value, result.err, "cancelled", "")
}

func TestPermissionOptionSelectionFailsClosedWithoutContinuationAdmission(t *testing.T) {
	manager := newPermissionTestManager(time.Second)
	manager.opts.PermissionContinuationAdmission = nil
	resultCh := startPermissionRequest(t, manager, permissionParams(map[string]any{"title": "write"}))
	interaction := waitForPendingInteraction(t, manager)
	_, err := manager.RespondInteraction(context.Background(), interaction.ID, "allow-once", false)
	var acpErr *Error
	if !errors.As(err, &acpErr) || acpErr.Code != "ACP_PERMISSION_ADMISSION_UNAVAILABLE" {
		t.Fatalf("missing admission error=%#v", err)
	}
	if current, inspectErr := manager.InspectInteraction(interaction.ID); inspectErr != nil || current.Status != InteractionPending {
		t.Fatalf("fail-closed interaction=%#v err=%v", current, inspectErr)
	}
	select {
	case result := <-resultCh:
		t.Fatalf("provider resumed without admission hook: %#v", result)
	default:
	}
	manager.cancelPendingInteractions("local-session")
	result := <-resultCh
	assertPermissionOutcome(t, result.value, result.err, "cancelled", "")
}

func TestPermissionTimeoutExpiresInteraction(t *testing.T) {
	manager := newPermissionTestManager(25 * time.Millisecond)
	resultCh := startPermissionRequest(t, manager, permissionParams(map[string]any{"title": "wait"}))
	interaction := waitForAnyInteraction(t, manager)
	result := <-resultCh
	assertPermissionOutcome(t, result.value, result.err, "cancelled", "")
	settled, err := manager.InspectInteraction(interaction.ID)
	if err != nil {
		t.Fatal(err)
	}
	if settled.Status != InteractionExpired {
		t.Fatalf("interaction status = %s", settled.Status)
	}
}

func TestPermissionManagerCloseExpiresInteraction(t *testing.T) {
	manager := newPermissionTestManager(time.Second)
	resultCh := startPermissionRequest(t, manager, permissionParams(map[string]any{"title": "wait"}))
	interaction := waitForPendingInteraction(t, manager)
	close(manager.closedCh)
	result := <-resultCh
	assertPermissionOutcome(t, result.value, result.err, "cancelled", "")
	settled, err := manager.InspectInteraction(interaction.ID)
	if err != nil {
		t.Fatal(err)
	}
	if settled.Status != InteractionExpired {
		t.Fatalf("interaction status = %s", settled.Status)
	}
}

func TestPermissionOversizedToolCallIsCancelledWithoutRetention(t *testing.T) {
	manager := newPermissionTestManager(time.Second)
	value, err := manager.handlePermission(context.Background(), permissionParams(map[string]any{
		"title": "oversized", "payload": strings.Repeat("x", maxPermissionToolCallBytes+1),
	}))
	assertPermissionOutcome(t, value, err, "cancelled", "")
	if interactions := manager.ListInteractions("local-session", false); len(interactions) != 0 {
		t.Fatalf("oversized interaction was retained: %#v", interactions)
	}
}

func TestPermissionPendingLimitCancelsNewRequest(t *testing.T) {
	manager := newPermissionTestManager(time.Second)
	now := time.Now().UTC()
	for index := 0; index < maxPendingInteractionsPerSession; index++ {
		id := "existing-" + string(rune('a'+index))
		manager.interactions[id] = &Interaction{
			ID: id, SessionID: "local-session", Kind: "permission", Status: InteractionPending,
			CreatedAt: now, ExpiresAt: now.Add(time.Hour), respond: make(chan interactionResponse, 1),
		}
	}
	value, err := manager.handlePermission(context.Background(), permissionParams(map[string]any{"title": "bounded"}))
	assertPermissionOutcome(t, value, err, "cancelled", "")
	if interactions := manager.ListInteractions("local-session", true); len(interactions) != maxPendingInteractionsPerSession {
		t.Fatalf("pending interactions = %d", len(interactions))
	}
}

type permissionResult struct {
	value any
	err   error
}

func newPermissionTestManager(timeout time.Duration) *Manager {
	return &Manager{
		opts: Options{
			InteractionTimeout: timeout,
			PermissionContinuationAdmission: func(context.Context, PermissionContinuation) (PermissionContinuationFinish, error) {
				return func(error) {}, nil
			},
		},
		remoteToLocal: map[string]string{"remote-session": "local-session"},
		sessions: map[string]SessionRecord{
			"local-session": {ID: "local-session", CWD: "/workspace"},
		},
		interactions: make(map[string]*Interaction), runs: make(map[string]*Run),
		activeRunBySession: make(map[string]string), closedCh: make(chan struct{}),
	}
}

func permissionParams(toolCall map[string]any) json.RawMessage {
	return testMarshalRaw(map[string]any{
		"sessionId": "remote-session", "toolCall": toolCall,
		"options": []map[string]any{
			{"optionId": "allow-once", "name": "Allow once", "kind": "allow_once"},
			{"optionId": "reject-once", "name": "Reject", "kind": "reject_once"},
			{"optionId": "allow-always", "name": "Always", "kind": "allow_always"},
		},
	})
}

func startPermissionRequest(t *testing.T, manager *Manager, params json.RawMessage) <-chan permissionResult {
	t.Helper()
	resultCh := make(chan permissionResult, 1)
	go func() {
		value, err := manager.handlePermission(context.Background(), params)
		resultCh <- permissionResult{value: value, err: err}
	}()
	return resultCh
}

func waitForAnyInteraction(t *testing.T, manager *Manager) Interaction {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		interactions := manager.ListInteractions("local-session", false)
		if len(interactions) == 1 {
			return interactions[0]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("permission interaction was not registered")
	return Interaction{}
}

func waitForPendingInteraction(t *testing.T, manager *Manager) Interaction {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		interactions := manager.ListInteractions("local-session", true)
		if len(interactions) == 1 {
			return interactions[0]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("permission interaction did not become pending")
	return Interaction{}
}

func assertPermissionOutcome(t *testing.T, value any, err error, wantOutcome, wantOption string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	outer, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("permission response type = %T", value)
	}
	outcome, ok := outer["outcome"].(map[string]any)
	if !ok || outcome["outcome"] != wantOutcome {
		t.Fatalf("permission outcome = %#v", value)
	}
	if wantOption != "" && outcome["optionId"] != wantOption {
		t.Fatalf("permission option = %#v", value)
	}
}

func TestHelperPermissionRoutesToOriginatingSecondSession(t *testing.T) {
	workspace := t.TempDir()
	manager, err := newTestManager(t.TempDir(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()

	first, err := manager.NewSession(context.Background(), workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.NewSession(context.Background(), workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	started, err := manager.StartPrompt(context.Background(), second.Session.ID, "route to second session")
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	var interaction Interaction
	for time.Now().Before(deadline) {
		pending := manager.ListInteractions(second.Session.ID, true)
		if len(pending) == 1 {
			interaction = pending[0]
			break
		}
		time.Sleep(time.Millisecond)
	}
	if interaction.ID == "" || interaction.SessionID != second.Session.ID {
		t.Fatalf("second-session interaction = %#v", interaction)
	}
	if pending := manager.ListInteractions(first.Session.ID, true); len(pending) != 0 {
		t.Fatalf("first session received second-session interaction: %#v", pending)
	}
	if _, err := manager.RespondInteraction(context.Background(), interaction.ID, "allow-once", false); err != nil {
		t.Fatal(err)
	}
	result := waitForSettledRun(t, manager, started.RunID)
	if result.Status != RunCompleted {
		t.Fatalf("second-session prompt = %#v", result)
	}
}
