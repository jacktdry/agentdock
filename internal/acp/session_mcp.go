package acp

import "context"

func (m *Manager) sessionMCPServers(ctx context.Context, sessionID, cwd string, additional []string) ([]SessionMCPServer, error) {
	if m.opts.SessionMCPProvider == nil {
		return []SessionMCPServer{}, nil
	}
	servers, err := m.opts.SessionMCPProvider.Servers(ctx, sessionID, cwd, append([]string(nil), additional...))
	if err != nil {
		return nil, newError("ACP_SESSION_MCP_FAILED", "prepare host-owned ACP MCP capabilities", false, map[string]any{"session_id": sessionID}, err)
	}
	m.sessionMCPMu.Lock()
	delete(m.sessionMCPReleased, sessionID)
	m.sessionMCPMu.Unlock()
	if servers == nil {
		return []SessionMCPServer{}, nil
	}
	return servers, nil
}

func (m *Manager) sessionCreationParams(ctx context.Context, sessionID, cwd string, additional []string) (map[string]any, error) {
	servers, err := m.sessionMCPServers(ctx, sessionID, cwd, additional)
	if err != nil {
		return nil, err
	}
	params := sessionCreationParams(cwd, additional)
	params["mcpServers"] = servers
	return params, nil
}

func (m *Manager) sessionActivationParams(ctx context.Context, record SessionRecord) (map[string]any, error) {
	params, err := m.sessionCreationParams(ctx, record.ID, record.CWD, record.AdditionalDirectories)
	if err != nil {
		return nil, err
	}
	params["sessionId"] = record.RemoteSessionID
	return params, nil
}

func (m *Manager) releaseSessionMCP(ctx context.Context, sessionID string) error {
	if m.opts.SessionMCPProvider == nil {
		return nil
	}
	m.sessionMCPMu.Lock()
	if _, released := m.sessionMCPReleased[sessionID]; released {
		m.sessionMCPMu.Unlock()
		return nil
	}
	m.sessionMCPReleased[sessionID] = struct{}{}
	m.sessionMCPMu.Unlock()
	if err := m.opts.SessionMCPProvider.ReleaseSession(ctx, sessionID); err != nil {
		m.sessionMCPMu.Lock()
		delete(m.sessionMCPReleased, sessionID)
		m.sessionMCPMu.Unlock()
		return newError("ACP_SESSION_MCP_RELEASE_FAILED", "release host-owned ACP MCP capabilities", true, map[string]any{"session_id": sessionID}, err)
	}
	return nil
}
