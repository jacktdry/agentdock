package acp

import (
	"sort"
	"time"
)

// DiagnosticsSnapshot describes the Manager's managed catalog and runtime at
// ObservedAt. Managed counts the in-memory catalog hydrated from persistence,
// including closed and unloaded records; it does not rescan external disk edits.
// Status counts use the catalog status, independently of Loaded.
type DiagnosticsSnapshot struct {
	ObservedAt time.Time            `json:"observed_at"`
	Counts     DiagnosticsCounts    `json:"counts"`
	Sessions   []SessionDiagnostics `json:"sessions"`
}

type DiagnosticsCounts struct {
	Managed             int `json:"managed"`
	Loaded              int `json:"loaded"`
	Running             int `json:"running"`
	Ready               int `json:"ready"`
	IdleManagedIdle     int `json:"idle_managed_idle"`
	IdleManagedEligible int `json:"idle_managed_eligible"`
	Closed              int `json:"closed"`
	AutoCloseFailures   int `json:"auto_close_failures"`
}

// SessionDiagnostics is an allowlisted value projection, with no process, run,
// interaction payload, MCP configuration, or capability references. Free-form
// auto-close errors are represented by "[redacted]"; custom closed reasons are
// likewise redacted, because either may contain prompt content or credentials.
type SessionDiagnostics struct {
	SessionID            string                 `json:"session_id"`
	Status               SessionStatus          `json:"status"`
	LifecyclePolicy      SessionLifecyclePolicy `json:"lifecycle_policy"`
	Loaded               bool                   `json:"loaded"`
	ActiveRunID          string                 `json:"active_run_id,omitempty"`
	PendingInteractions  int                    `json:"pending_interactions"`
	SessionOperations    int                    `json:"session_operations"`
	LastActiveAt         time.Time              `json:"last_active_at"`
	IdleCloseAfterMS     int64                  `json:"idle_close_after_ms"`
	IdleManagedIdle      bool                   `json:"idle_managed_idle"`
	IdleManagedEligible  bool                   `json:"idle_managed_eligible"`
	ClosedAt             *time.Time             `json:"closed_at,omitempty"`
	ClosedReason         string                 `json:"closed_reason,omitempty"`
	AutoCloseAttemptedAt *time.Time             `json:"auto_close_attempted_at,omitempty"`
	AutoCloseError       string                 `json:"auto_close_error,omitempty"`
}

// Diagnostics only observes state under the Manager lock. Loaded means present
// in m.loaded, not proof that the adapter is alive. Idle means idle-managed,
// ready, loaded, positive TTL, no active run/pending interaction/tracked operation
// or terminal transition, and an open manager with a live adapter connection.
// Eligible additionally means now >= last_active_at + TTL. These are the same
// guards used by beginIdleAutoCloseTransition, not a new lifecycle state.
// Eligibility is observational: a later sweep must recheck it before closing.
func (m *Manager) Diagnostics() DiagnosticsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := DiagnosticsSnapshot{ObservedAt: time.Now().UTC(), Sessions: make([]SessionDiagnostics, 0, len(m.sessions))}
	for id, record := range m.sessions {
		_, loaded := m.loaded[id]
		idle, eligible := m.idleManagedStateLocked(id, record, result.ObservedAt)
		entry := SessionDiagnostics{
			SessionID: id, Status: record.Status, LifecyclePolicy: record.LifecyclePolicy,
			Loaded: loaded, ActiveRunID: m.activeRunBySession[id],
			PendingInteractions: m.pendingInteractionCountLocked(id), SessionOperations: m.sessionOperations[id],
			LastActiveAt: record.LastActiveAt, IdleCloseAfterMS: record.IdleCloseAfterMS,
			IdleManagedIdle: idle, IdleManagedEligible: eligible,
			ClosedAt: diagnosticsTime(record.ClosedAt), AutoCloseAttemptedAt: diagnosticsTime(record.AutoCloseAttemptedAt),
		}
		switch record.ClosedReason {
		case "", "manual", "idle_timeout", "ephemeral_prompt_terminal":
			entry.ClosedReason = record.ClosedReason
		default:
			entry.ClosedReason = "[redacted]"
		}
		if record.AutoCloseError != "" {
			entry.AutoCloseError = "[redacted]"
			result.Counts.AutoCloseFailures++
		}
		result.Counts.Managed++
		if loaded {
			result.Counts.Loaded++
		}
		if idle {
			result.Counts.IdleManagedIdle++
		}
		if eligible {
			result.Counts.IdleManagedEligible++
		}
		switch record.Status {
		case SessionReady:
			result.Counts.Ready++
		case SessionRunning:
			result.Counts.Running++
		case SessionClosed:
			result.Counts.Closed++
		}
		result.Sessions = append(result.Sessions, entry)
	}
	sort.Slice(result.Sessions, func(i, j int) bool { return result.Sessions[i].SessionID < result.Sessions[j].SessionID })
	return result
}

func diagnosticsTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (m *Manager) pendingInteractionCountLocked(id string) int {
	count := 0
	for _, interaction := range m.interactions {
		if interaction.SessionID == id && interaction.Status == InteractionPending {
			count++
		}
	}
	return count
}

// idleManagedStateLocked requires m.mu to be held for reading or writing.
func (m *Manager) idleManagedStateLocked(id string, record SessionRecord, now time.Time) (idle, eligible bool) {
	if m.closed || record.Status != SessionReady || record.LifecyclePolicy != LifecycleIdleManaged {
		return false, false
	}
	if _, transitioning := m.terminalSessions[id]; transitioning {
		return false, false
	}
	if _, loaded := m.loaded[id]; !loaded {
		return false, false
	}
	ttl := time.Duration(record.IdleCloseAfterMS) * time.Millisecond
	if ttl <= 0 || m.activeRunBySession[id] != "" || m.sessionOperations[id] > 0 || m.pendingInteractionCountLocked(id) > 0 {
		return false, false
	}
	process := m.process
	if process == nil || process.connection == nil {
		return false, false
	}
	select {
	case <-process.connection.Closed():
		return false, false
	default:
	}
	return true, !now.Before(record.LastActiveAt.Add(ttl))
}

// AdapterProcessID returns the immutable root PID of the currently owned ACP
// adapter process, or 0 when no adapter process has been started. It does not
// probe, start, resume, or otherwise mutate the adapter lifecycle.
func (m *Manager) AdapterProcessID() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	process := m.process
	m.mu.RUnlock()
	if process == nil {
		return 0
	}
	return process.pid
}
