package acp

// IsLiveManagedSession is an advisory read-only assertion that a local ACP
// Session belongs to this Manager and is currently loaded in its attached
// Adapter. It does NOT attest the native Codex Thread, MCP child PIDs or OS
// ownership. This snapshot can become stale immediately after return; callers
// must independently fence each actual M8 tool dispatch.
func (m *Manager) IsLiveManagedSession(localID string) bool {
	if m == nil || !diagnosticOpaqueID(localID) {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed || m.process == nil || m.process.connection == nil || m.process.pid <= 0 {
		return false
	}
	select {
	case <-m.process.connection.Closed():
		return false
	default:
	}
	s, ok := m.sessions[localID]
	loaded, live := m.loaded[localID]
	if !ok || !live || s.ID != localID || !diagnosticOpaqueID(s.RemoteSessionID) || m.remoteToLocal[s.RemoteSessionID] != localID || (loaded.SessionID != "" && loaded.SessionID != s.RemoteSessionID) || (s.Status != SessionReady && s.Status != SessionRunning) {
		return false
	}
	if _, ending := m.terminalSessions[localID]; ending {
		return false
	}
	return true
}
