package acp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// ManagedSessionRetention is independent of adapter-native session retention.
// Adapter transcripts may also belong to standalone CLI workflows and must not
// be deleted by unattended AgentDock maintenance.
const ManagedSessionRetention = 7 * 24 * time.Hour

// SessionRetentionReport is returned for both read-only preview and cleanup.
type SessionRetentionReport struct {
	Scanned    int `json:"scanned"`
	Candidates int `json:"candidates"`
	Removed    int `json:"removed"`
}

// SweepStaleManagedSessions removes only AgentDock-owned local mappings that
// have not been active for seven days. It never calls session/delete on an
// adapter, and never modifies the adapter's own transcript or native sessions.
// dryRun=true is a read-only preview with the same eligibility checks.
func (m *Manager) SweepStaleManagedSessions(now time.Time, dryRun bool) (SessionRetentionReport, error) {
	var report SessionRetentionReport
	if m == nil {
		return report, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return report, nil
	}
	report.Scanned = len(m.sessions)
	ids := make([]string, 0)
	for id, record := range m.sessions {
		if m.retentionEligibleLocked(id, record, now) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	report.Candidates = len(ids)
	if dryRun {
		m.mu.Unlock()
		return report, nil
	}
	var errs []error
	removedIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		record, exists := m.sessions[id]
		if !exists || !m.retentionEligibleLocked(id, record, now) {
			continue
		}
		// Store deletion is atomic with the manager's mapping removal under
		// the same lock. A failure leaves both intact for a later retry.
		if err := m.store.Delete(id); err != nil {
			errs = append(errs, fmt.Errorf("prune managed ACP session %s: %w", id, err))
			continue
		}
		delete(m.sessions, id)
		delete(m.remoteToLocal, record.RemoteSessionID)
		delete(m.projections, id)
		delete(m.terminalSessions, id)
		delete(m.loaded, id)
		removedIDs = append(removedIDs, id)
		report.Removed++
	}
	m.mu.Unlock()
	// Release any host-owned per-session MCP resources outside the manager
	// lock; the adapter-native transcript remains completely untouched.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, id := range removedIDs {
		if err := m.releaseSessionMCP(ctx, id); err != nil {
			errs = append(errs, fmt.Errorf("release pruned ACP session resources %s: %w", id, err))
		}
	}
	return report, errors.Join(errs...)
}

func (m *Manager) retentionEligibleLocked(id string, record SessionRecord, now time.Time) bool {
	if record.RetentionProtected || record.LastActiveAt.IsZero() ||
		now.Before(record.LastActiveAt.Add(ManagedSessionRetention)) ||
		record.Status == SessionRunning {
		return false
	}
	if m.sessionOperations[id] > 0 || m.activeRunBySession[id] != "" {
		return false
	}
	if _, busy := m.terminalSessions[id]; busy {
		return false
	}
	if _, loaded := m.loaded[id]; loaded {
		return false
	}
	for _, interaction := range m.interactions {
		if interaction.SessionID == id && interaction.Status == InteractionPending {
			return false
		}
	}
	return true
}
