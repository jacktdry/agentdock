package acp

import (
	"context"
	"errors"
	"sync"
)

var errInstrumentedSessionMCPUnavailable = errors.New("Next Codex MCP instrumentation unavailable")

// InstrumentedSessionProvider only adds a Core-authorized and immutable
// stdio wrapper to an ACP Session; it never allows the model to select the
// executable or its command line. The original host MCP provider is retained.
// No native Codex per-Thread ownership is inferred from the returned config.
type InstrumentedSessionProvider struct {
	underlying SessionMCPProvider
	server     SessionMCPServer
	mu         sync.Mutex
	borrowers  map[string]struct{}
}

func NewInstrumentedSessionProvider(base SessionMCPProvider, server SessionMCPServer) (*InstrumentedSessionProvider, error) {
	if server.Name == "" || server.Type != "" || server.URL != "" || len(server.Headers) != 0 || server.Command == "" || len(server.Args) == 0 || server.Args[len(server.Args)-1] == "" {
		return nil, errInstrumentedSessionMCPUnavailable
	}
	return &InstrumentedSessionProvider{underlying: base, server: server, borrowers: make(map[string]struct{})}, nil
}
func (p *InstrumentedSessionProvider) Servers(ctx context.Context, id, cwd string, dirs []string) ([]SessionMCPServer, error) {
	if p == nil || ctx == nil || ctx.Err() != nil || id == "" {
		return nil, errInstrumentedSessionMCPUnavailable
	}
	existing := []SessionMCPServer{}
	if p.underlying != nil {
		out, err := p.underlying.Servers(ctx, id, cwd, dirs)
		if err != nil {
			return nil, err
		}
		existing = append(existing, out...)
	}
	for _, s := range existing {
		if s.Name == p.server.Name {
			return nil, errInstrumentedSessionMCPUnavailable
		}
	}
	p.mu.Lock()
	p.borrowers[id] = struct{}{}
	p.mu.Unlock()
	return append(existing, p.server), nil
}
func (p *InstrumentedSessionProvider) ReleaseSession(ctx context.Context, id string) error {
	if p == nil || ctx == nil || ctx.Err() != nil || id == "" {
		return errInstrumentedSessionMCPUnavailable
	}
	// Release underlying host capability first. Failure keeps our borrower
	// recorded and prevents false "all released" claims.
	if p.underlying != nil {
		if err := p.underlying.ReleaseSession(ctx, id); err != nil {
			return err
		}
	}
	p.mu.Lock()
	delete(p.borrowers, id)
	p.mu.Unlock()
	return nil
}

// Borrowers returns logical ACP references only. The full native server might
// also have opaque Codex Thread, background, or external borrowers.
func (p *InstrumentedSessionProvider) Borrowers() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.borrowers)
}

// InstallInstrumentedSessionMCP is exclusively for a Next Core-prepared
// adapter with NO locally known Sessions. Never mutate a live Session's
// server inventory, and never override a preexisting instrumented provider.
func (m *Manager) InstallInstrumentedSessionMCP(server SessionMCPServer) error {
	if m == nil {
		return errInstrumentedSessionMCPUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || len(m.sessions) != 0 || len(m.loaded) != 0 || len(m.runs) != 0 || m.opts.Agent.Environment["AGENTDOCK_NEXT_CODEX_MCP_ISOLATION"] != "source-only" {
		return errInstrumentedSessionMCPUnavailable
	}
	if _, ok := m.opts.SessionMCPProvider.(*InstrumentedSessionProvider); ok {
		return errInstrumentedSessionMCPUnavailable
	}
	provider, err := NewInstrumentedSessionProvider(m.opts.SessionMCPProvider, server)
	if err != nil {
		return err
	}
	m.opts.SessionMCPProvider = provider
	return nil
}

func (m *Manager) InstrumentedSessionMCPBorrowers() int {
	if m == nil {
		return -1
	}
	m.mu.RLock()
	p, ok := m.opts.SessionMCPProvider.(*InstrumentedSessionProvider)
	m.mu.RUnlock()
	if !ok {
		return -1
	}
	return p.Borrowers()
}
