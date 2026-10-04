package acp

import (
	"context"
	"log/slog"
	"time"
)

const autoCloseTimeout = 35 * time.Second

func (m *Manager) scheduleEphemeralAutoClose(record SessionRecord, status RunStatus) {
	if record.LifecyclePolicy != LifecycleEphemeral || record.ID == "" || status == RunInterrupted {
		return
	}
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return
	}
	go m.autoCloseSession(record.ID, "ephemeral_prompt_terminal", record.LastActiveAt)
}

func (m *Manager) autoCloseSession(id, reason string, expectedLastActive time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), autoCloseTimeout)
	defer cancel()
	record, attemptedAt, eligible, err := m.beginAutoCloseTransition(ctx, id, expectedLastActive)
	if err != nil {
		if errorCode(err) != "ACP_SESSION_NOT_FOUND" {
			slog.Warn("begin ACP auto-close transition failed", "session_id", id, "reason", reason, "error", err)
		}
		return
	}
	if !eligible {
		return
	}
	_, closeErr := m.closeSessionClaimed(ctx, record, reason, true, "", false)
	if closeErr == nil || errorCode(closeErr) == "ACP_SESSION_NOT_FOUND" || errorCode(closeErr) == "ACP_SESSION_CLOSED" {
		return
	}
	if err := m.recordAutoCloseFailure(id, attemptedAt, closeErr); err != nil {
		slog.Warn("persist ACP auto-close failure failed", "session_id", id, "reason", reason, "close_error", closeErr, "persist_error", err)
	}
}

func (m *Manager) beginAutoCloseTransition(ctx context.Context, id string, expectedLastActive time.Time) (SessionRecord, time.Time, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return SessionRecord{}, time.Time{}, false, newError("ACP_SESSION_TRANSITION_CANCELLED", "ACP session auto-close transition was cancelled", true, map[string]any{"session_id": id}, err)
	}
	m.mu.Lock()
	stopWake := context.AfterFunc(ctx, func() {
		m.mu.Lock()
		m.operationsCond.Broadcast()
		m.mu.Unlock()
	})
	defer func() {
		stopWake()
		m.mu.Unlock()
	}()
	if m.closed {
		return SessionRecord{}, time.Time{}, false, newError("ACP_MANAGER_CLOSED", "ACP manager is closed", false, nil, nil)
	}
	if _, transitioning := m.terminalSessions[id]; transitioning {
		return SessionRecord{}, time.Time{}, false, nil
	}
	eligible := func() (SessionRecord, bool) {
		record, exists := m.sessions[id]
		if !exists || record.Status == SessionClosed || record.LifecyclePolicy != LifecycleEphemeral {
			return record, false
		}
		if !expectedLastActive.IsZero() && !record.LastActiveAt.Equal(expectedLastActive) {
			return record, false
		}
		if m.activeRunBySession[id] != "" {
			return record, false
		}
		for _, interaction := range m.interactions {
			if interaction.SessionID == id && interaction.Status == InteractionPending {
				return record, false
			}
		}
		return record, true
	}
	record, ok := eligible()
	if !ok {
		return record, time.Time{}, false, nil
	}
	m.terminalSessions[id] = SessionClosed
	for m.sessionOperations[id] > 0 {
		if err := ctx.Err(); err != nil {
			delete(m.terminalSessions, id)
			return SessionRecord{}, time.Time{}, false, newError("ACP_SESSION_TRANSITION_CANCELLED", "ACP session auto-close transition was cancelled", true, map[string]any{"session_id": id}, err)
		}
		if m.closed {
			delete(m.terminalSessions, id)
			return SessionRecord{}, time.Time{}, false, newError("ACP_MANAGER_CLOSED", "ACP manager is closed", false, nil, nil)
		}
		m.operationsCond.Wait()
	}
	record, ok = eligible()
	if !ok {
		delete(m.terminalSessions, id)
		return record, time.Time{}, false, nil
	}
	now := time.Now().UTC()
	record.AutoCloseAttemptedAt = &now
	record.AutoCloseError = ""
	record.UpdatedAt = now
	if err := m.store.Save(record); err != nil {
		delete(m.terminalSessions, id)
		return SessionRecord{}, time.Time{}, false, err
	}
	m.sessions[id] = record
	return record, now, true, nil
}

func (m *Manager) recordAutoCloseFailure(id string, attemptedAt time.Time, closeErr error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, exists := m.sessions[id]
	if !exists {
		return newError("ACP_SESSION_NOT_FOUND", "ACP session was not found", false, map[string]any{"session_id": id}, nil)
	}
	if record.AutoCloseAttemptedAt == nil {
		stamp := attemptedAt
		if stamp.IsZero() {
			stamp = time.Now().UTC()
		}
		record.AutoCloseAttemptedAt = &stamp
	}
	record.AutoCloseError = closeErr.Error()
	record.UpdatedAt = time.Now().UTC()
	if err := m.store.Save(record); err != nil {
		return err
	}
	m.sessions[id] = record
	return nil
}
