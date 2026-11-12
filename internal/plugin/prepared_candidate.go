package plugin

import (
	"context"
	"errors"
	"sync"
)

// PreparedCandidate owns one immutable, AgentDock-managed staged Plugin
// snapshot. The original user-selected source path is never needed after
// preparation. Candidates are single-use for lifecycle mutations.
type PreparedCandidate struct {
	mu      sync.Mutex
	stage   string
	pkg     Package
	cleanup func()
	claimed bool
	closed  bool
}

func (m *Manager) PrepareCandidate(source string) (*PreparedCandidate, error) {
	stage, pkg, cleanup, err := m.prepareCandidateSource(source)
	if err != nil {
		return nil, err
	}
	return &PreparedCandidate{stage: stage, pkg: pkg, cleanup: cleanup}, nil
}

func (c *PreparedCandidate) Review() (Review, error) {
	if c == nil {
		return Review{}, errors.New("Plugin candidate is unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.claimed {
		return Review{}, errors.New("Plugin candidate is unavailable")
	}
	return buildReview(c.pkg), nil
}

func (c *PreparedCandidate) claim() (string, Package, func(), error) {
	if c == nil {
		return "", Package{}, func() {}, errors.New("Plugin candidate is unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.claimed {
		return "", Package{}, func() {}, errors.New("Plugin candidate is unavailable")
	}
	c.claimed = true
	cleanup := c.cleanup
	if cleanup == nil {
		cleanup = func() {}
	}
	return c.stage, c.pkg, cleanup, nil
}

func (c *PreparedCandidate) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	cleanup := c.cleanup
	c.cleanup = nil
	c.mu.Unlock()
	if cleanup != nil {
		cleanup()
	}
}

func (m *Manager) InstallPreparedCandidate(ctx context.Context, candidate *PreparedCandidate, enabled bool) (ChangeResult, error) {
	stage, pkg, cleanup, err := candidate.claim()
	if err != nil {
		return ChangeResult{}, pluginError("PLUGIN_CANDIDATE_UNAVAILABLE", "install.candidate", err)
	}
	defer cleanup()
	return m.installPreparedCandidate(ctx, stage, pkg, enabled)
}

func (m *Manager) UpdatePreparedCandidate(ctx context.Context, candidate *PreparedCandidate, beforeSwitch func(State) error) (ChangeResult, error) {
	stage, pkg, cleanup, err := candidate.claim()
	if err != nil {
		return ChangeResult{}, pluginError("PLUGIN_CANDIDATE_UNAVAILABLE", "update.candidate", err)
	}
	defer cleanup()
	return m.updatePreparedCandidate(ctx, stage, pkg, beforeSwitch)
}
