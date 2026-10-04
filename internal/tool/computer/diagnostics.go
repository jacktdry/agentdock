package computer

import (
	"sort"
	"time"
)

const maxComputerDiagnosticEvents = 50

type DiagnosticEvent struct {
	At              time.Time    `json:"at"`
	Kind            string       `json:"kind"`
	SessionID       string       `json:"computer_session_id,omitempty"`
	Provider        ProviderID   `json:"computer_provider,omitempty"`
	Action          string       `json:"action_kind,omitempty"`
	Message         string       `json:"message,omitempty"`
	ActiveAppBefore *AppIdentity `json:"active_app_before,omitempty"`
	ActiveAppAfter  *AppIdentity `json:"active_app_after,omitempty"`
}

type BrokerDiagnostics struct {
	Provider         ProviderID        `json:"computer_provider"`
	ActiveSessions   []SessionMetadata `json:"active_sessions"`
	ReleasedSessions []SessionMetadata `json:"released_sessions"`
	RecentEvents     []DiagnosticEvent `json:"recent_events"`
}

func (b *Broker) recordDiagnostic(event DiagnosticEvent) {
	if b == nil {
		return
	}
	event.At = time.Now().UTC()
	if event.Provider == "" && b.provider != nil {
		event.Provider = b.provider.ID()
	}
	b.diagMu.Lock()
	if len(b.diagEvents) >= maxComputerDiagnosticEvents {
		copy(b.diagEvents, b.diagEvents[len(b.diagEvents)-maxComputerDiagnosticEvents+1:])
		b.diagEvents = b.diagEvents[:maxComputerDiagnosticEvents-1]
	}
	b.diagEvents = append(b.diagEvents, event)
	b.diagMu.Unlock()
}

func (b *Broker) Diagnostics() BrokerDiagnostics {
	if b == nil {
		return BrokerDiagnostics{}
	}
	out := BrokerDiagnostics{}
	if b.provider != nil {
		out.Provider = b.provider.ID()
	}
	b.mu.RLock()
	active := make([]*session, 0, len(b.sessions))
	for _, s := range b.sessions {
		active = append(active, s)
	}
	for _, meta := range b.released {
		out.ReleasedSessions = append(out.ReleasedSessions, meta)
	}
	b.mu.RUnlock()
	for _, s := range active {
		s.mu.Lock()
		out.ActiveSessions = append(out.ActiveSessions, s.metadata)
		s.mu.Unlock()
	}
	sort.Slice(out.ActiveSessions, func(i, j int) bool { return out.ActiveSessions[i].CreatedAt.Before(out.ActiveSessions[j].CreatedAt) })
	sort.Slice(out.ReleasedSessions, func(i, j int) bool {
		return out.ReleasedSessions[i].CreatedAt.Before(out.ReleasedSessions[j].CreatedAt)
	})
	if len(out.ReleasedSessions) > 200 {
		out.ReleasedSessions = append([]SessionMetadata(nil), out.ReleasedSessions[len(out.ReleasedSessions)-200:]...)
	}
	b.diagMu.Lock()
	out.RecentEvents = append([]DiagnosticEvent(nil), b.diagEvents...)
	b.diagMu.Unlock()
	return out
}
