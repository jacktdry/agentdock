package acp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
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
	// true means accepted; false retains a failed bootstrap's host cleanup obligation.
	borrowers    map[string]bool
	sessionLocks map[string]*instrumentedSessionLock
}

type instrumentedSessionLock struct {
	busy  chan struct{}
	users int // holder plus waiters; protected by the provider mutex
}

func NewInstrumentedSessionProvider(base SessionMCPProvider, server SessionMCPServer) (*InstrumentedSessionProvider, error) {
	if server.Name == "" || server.Type != "" || server.URL != "" || len(server.Headers) != 0 || server.Command == "" || len(server.Args) == 0 || server.Args[len(server.Args)-1] == "" {
		return nil, errInstrumentedSessionMCPUnavailable
	}
	return &InstrumentedSessionProvider{underlying: base, server: server, borrowers: make(map[string]bool), sessionLocks: make(map[string]*instrumentedSessionLock)}, nil
}

// lockSession serializes only one session's lifecycle. Entries live only while
// held or awaited, and cancellation can abandon a wait without blocking others.
// The provider mutex protects bookkeeping only, never underlying provider I/O.
func (p *InstrumentedSessionProvider) lockSession(ctx context.Context, id string) (func(), error) {
	p.mu.Lock()
	lock := p.sessionLocks[id]
	if lock == nil {
		lock = &instrumentedSessionLock{busy: make(chan struct{}, 1)}
		p.sessionLocks[id] = lock
	}
	lock.users++
	p.mu.Unlock()
	releaseRef := func() {
		p.mu.Lock()
		lock.users--
		if lock.users == 0 {
			delete(p.sessionLocks, id)
		}
		p.mu.Unlock()
	}
	select {
	case lock.busy <- struct{}{}:
		return func() {
			<-lock.busy
			releaseRef()
		}, nil
	case <-ctx.Done():
		releaseRef()
		return nil, errInstrumentedSessionMCPUnavailable
	}
}
func (p *InstrumentedSessionProvider) Servers(ctx context.Context, id, cwd string, dirs []string) ([]SessionMCPServer, error) {
	if p == nil || ctx == nil || ctx.Err() != nil || id == "" {
		return nil, errInstrumentedSessionMCPUnavailable
	}
	unlock, err := p.lockSession(ctx, id)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if ctx.Err() != nil {
		return nil, errInstrumentedSessionMCPUnavailable
	}
	p.mu.Lock()
	accepted, tracked := p.borrowers[id]
	p.mu.Unlock()
	if tracked && !accepted {
		return nil, fmt.Errorf("%w: host capability cleanup pending; retry ReleaseSession", errInstrumentedSessionMCPUnavailable)
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
			// A retry may reuse an accepted host capability. Only roll back a
			// new bootstrap; releasing an accepted session would interrupt it.
			if !tracked {
				p.mu.Lock()
				p.borrowers[id] = false
				p.mu.Unlock()
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
				err := p.underlying.ReleaseSession(cleanupCtx, id)
				cancel()
				if err != nil {
					return nil, errors.Join(errInstrumentedSessionMCPUnavailable, fmt.Errorf("rollback host MCP capability: %w", err))
				}
				p.mu.Lock()
				delete(p.borrowers, id)
				p.mu.Unlock()
			}
			return nil, errInstrumentedSessionMCPUnavailable
		}
	}
	p.mu.Lock()
	p.borrowers[id] = true
	p.mu.Unlock()
	return append(existing, p.server), nil
}
func (p *InstrumentedSessionProvider) ReleaseSession(ctx context.Context, id string) error {
	if p == nil || ctx == nil || ctx.Err() != nil || id == "" {
		return errInstrumentedSessionMCPUnavailable
	}
	unlock, err := p.lockSession(ctx, id)
	if err != nil {
		return err
	}
	defer unlock()
	if ctx.Err() != nil {
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

// Borrowers includes logical ACP references and pending host rollback obligations.
// The full native server might also have opaque Codex Thread, background, or
// external borrowers.
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
