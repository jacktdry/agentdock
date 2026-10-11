package client

import (
	"context"
	"strings"
	"time"
)

// NextStdioRegistryGate binds the Next-only stdio prototype to P5's REAL
// persistent registry/generation. This is a source-only candidate, not a Core
// admission or M8 permission capability. Only a trusted Next Core runtime may
// construct and call it; M8 admission must precede production use.
// Registry reads alone cannot atomically fence another process's mutation
// during a tool call. Changed authority suppresses the reply and quarantines
// this ONLY-owned stdio session, but cannot undo a dispatched tool side effect.
type NextStdioRegistryGate struct {
	registry          *Manager
	leases            *NextStdioLeaseController
	postHandshakeHook func() // test-only deterministic revocation fence; never set by runtime
}

type NextRegistryLease struct {
	lease    NextStdioLease
	revision string
	cfg      ServerConfig
}

func NewNextStdioRegistryGate(registry *Manager, leases *NextStdioLeaseController) (*NextStdioRegistryGate, error) {
	if registry == nil || leases == nil {
		return nil, errNextLeaseDenied
	}
	return &NextStdioRegistryGate{registry: registry, leases: leases}, nil
}

// checkedConfig always rereads P5's disk-backed snapshot. The caller supplies
// an exact revision and Generation, not a ServerConfig or executable path.
// Plugin-owned and HTTP servers are intentionally excluded, not migrated into
// a second registry. Dynamic MCP environment resolution is checked too.
func (g *NextStdioRegistryGate) checkedConfig(name, revision, generation string) (ServerConfig, error) {
	if g == nil || g.registry == nil || g.leases == nil || !validNextID(name) || !validNextID(revision) || !validNextID(generation) {
		return ServerConfig{}, errNextLeaseDenied
	}
	snapshot, err := g.registry.Registry()
	if err != nil || snapshot.Revision != revision {
		return ServerConfig{}, errNextLeaseDenied
	}
	cfg, ok := snapshot.Servers[name]
	if !ok || cfg.Generation != generation || cfg.SourceType != "standalone" || cfg.PluginName != "" || !cfg.Enabled || cfg.Transport != TransportStdio || strings.TrimSpace(cfg.Command) == "" {
		return ServerConfig{}, errNextLeaseDenied
	}
	effective, err := g.registry.runtimeConfig(cfg)
	if err != nil || effective.Generation != generation {
		return ServerConfig{}, errNextLeaseDenied
	}
	return effective, nil
}

// AcquireChecked reserves an exact P5 server incarnation before launching and
// REVALIDATES after the handshake. No caller-supplied argv/env/command is used.
// Stale or failed post-validation consumes its own borrower token and closes
// only the session actually spawned by the candidate controller.
func (g *NextStdioRegistryGate) acquireChecked(ctx context.Context, owner, server, revision, generation string, policy NextLeasePolicy, ttl time.Duration) (NextRegistryLease, error) {
	if ctx == nil || ctx.Err() != nil {
		return NextRegistryLease{}, errNextLeaseDenied
	}
	cfg, err := g.checkedConfig(server, revision, generation)
	if err != nil {
		return NextRegistryLease{}, errNextLeaseDenied
	}
	lease, err := g.leases.acquire(ctx, owner, cfg, policy, ttl)
	if err != nil {
		return NextRegistryLease{}, errNextLeaseDenied
	}
	if g.postHandshakeHook != nil {
		g.postHandshakeHook()
	}
	current, err := g.checkedConfig(server, revision, generation)
	if err != nil || ctx.Err() != nil || !sameNextConfig(current, cfg) {
		g.leases.invalidate(lease)
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = g.leases.Release(cleanupCtx, lease)
		return NextRegistryLease{}, errNextLeaseDenied
	}
	return NextRegistryLease{lease: lease, revision: revision, cfg: cfg}, nil
}

func (g *NextStdioRegistryGate) callChecked(ctx context.Context, lease NextRegistryLease, tool string, args map[string]any) (map[string]any, error) {
	if g == nil || ctx == nil || ctx.Err() != nil || lease.lease.id == "" || lease.revision == "" {
		return nil, errNextLeaseDenied
	}
	_, server, ok := strings.Cut(lease.lease.key, "\x00")
	if !ok {
		return nil, errNextLeaseDenied
	}
	current, err := g.checkedConfig(server, lease.revision, lease.lease.generation)
	if err != nil || !sameNextConfig(current, lease.cfg) {
		g.leases.invalidate(lease.lease)
		return nil, errNextLeaseDenied
	}
	result, err := g.leases.call(ctx, lease.lease, tool, args)
	if err != nil {
		return nil, err
	}
	after, err := g.checkedConfig(server, lease.revision, lease.lease.generation)
	if err != nil || ctx.Err() != nil || !sameNextConfig(after, lease.cfg) {
		g.leases.invalidate(lease.lease)
		return nil, errNextLeaseDenied
	}
	return result, nil
}

func (g *NextStdioRegistryGate) Release(ctx context.Context, lease NextRegistryLease) error {
	if g == nil || g.leases == nil || lease.lease.id == "" {
		return errNextLeaseDenied
	}
	return g.leases.Release(ctx, lease.lease)
}
