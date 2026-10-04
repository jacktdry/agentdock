package acp

import (
	"context"
	"errors"
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

const (
	lifecycleSweepInterval = 30 * time.Second
	maxIdleClosePerSweep   = 4
)

func (m *Manager) runLifecycleSweeper() {
	if m == nil || m.lifecycleDone == nil {
		return
	}
	defer close(m.lifecycleDone)
	ticker := time.NewTicker(lifecycleSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.closedCh:
			return
		case now := <-ticker.C:
			if err := m.SweepIdleManaged(now.UTC()); err != nil {
				slog.Warn("ACP idle-managed lifecycle sweep failed", "agent", m.opts.Agent.Name, "error", err)
			}
		}
	}
}

// SweepIdleManaged closes at most maxIdleClosePerSweep loaded idle-managed
// sessions. It never starts or reloads an adapter merely to close an unloaded
// persisted session.
func (m *Manager) SweepIdleManaged(now time.Time) error {
	if m == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	m.mu.RLock()
	ids := make([]string, 0, maxIdleClosePerSweep)
	for id, record := range m.sessions {
		if len(ids) >= maxIdleClosePerSweep {
			break
		}
		if record.LifecyclePolicy != LifecycleIdleManaged || record.Status != SessionReady {
			continue
		}
		if _, loaded := m.loaded[id]; !loaded {
			continue
		}
		ttl := time.Duration(record.IdleCloseAfterMS) * time.Millisecond
		if ttl <= 0 || now.Before(record.LastActiveAt.Add(ttl)) {
			continue
		}
		ids = append(ids, id)
	}
	m.mu.RUnlock()

	var failures []error
	for _, id := range ids {
		ctx, cancel := context.WithTimeout(context.Background(), autoCloseTimeout)
		record, attemptedAt, process, eligible, err := m.beginIdleAutoCloseTransition(ctx, id, now)
		if err != nil {
			failures = append(failures, err)
			cancel()
			continue
		}
		if !eligible {
			cancel()
			continue
		}
		_, closeErr := m.closeSessionClaimedWithProcess(ctx, record, "idle_timeout", true, "", false, process)
		cancel()
		if closeErr == nil {
			continue
		}
		if persistErr := m.recordAutoCloseFailure(id, attemptedAt, closeErr); persistErr != nil {
			failures = append(failures, persistErr)
		}
		failures = append(failures, closeErr)
	}
	return errors.Join(failures...)
}

func (m *Manager) beginIdleAutoCloseTransition(ctx context.Context, id string, now time.Time) (SessionRecord, time.Time, *agentProcess, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return SessionRecord{}, time.Time{}, nil, false, newError("ACP_SESSION_TRANSITION_CANCELLED", "ACP idle-managed transition was cancelled", true, map[string]any{"session_id": id}, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	record, exists := m.sessions[id]
	if !exists {
		return record, time.Time{}, nil, false, nil
	}
	if _, eligible := m.idleManagedStateLocked(id, record, now); !eligible {
		return record, time.Time{}, nil, false, nil
	}
	process := m.process
	m.terminalSessions[id] = SessionClosed
	attemptedAt := time.Now().UTC()
	record.AutoCloseAttemptedAt = &attemptedAt
	record.AutoCloseError = ""
	record.UpdatedAt = attemptedAt
	if err := m.store.Save(record); err != nil {
		delete(m.terminalSessions, id)
		return SessionRecord{}, time.Time{}, nil, false, err
	}
	m.sessions[id] = record
	return record, attemptedAt, process, true, nil
}
