package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/uvwt/agentdock/internal/process"
)

// A Next-only capability scoped to the lifecycle of a single, directly spawned
// stdio MCP server. This does NOT own an ACP app-server, externally observed PID,
// Plugin process, Browser Broker worker, or shared HTTP service.
// The existing StdioSession/Controller remains the SOLE process Wait/close owner.
// There is no production runtime caller; not a second MCP registry.
type NextLeasePolicy string

const (
	NextLeaseEphemeral   NextLeasePolicy = "ephemeral"
	NextLeasePersistent  NextLeasePolicy = "persistent"
	NextLeaseIdleManaged NextLeasePolicy = "idle-managed"
)

type NextStdioLease struct{ key, id, generation string }
type NextLeaseCounts struct {
	Namespace       string `json:"namespace"`
	Managed         int    `json:"managed"`
	Starting        int    `json:"starting"`
	Ready           int    `json:"ready"`
	Draining        int    `json:"draining"`
	CleanupRequired int    `json:"cleanup_required"`
	Borrowers       int    `json:"borrowers"`
	Inflight        int    `json:"inflight"`
}

var errNextLeaseDenied = errors.New("Next MCP lease denied")

// nextResource is private; it NEVER exposes a process owner capability to an
// observed_external caller. The only transition to ready is after a fresh
// OS birth/parent proof of a child WE JUST STARTED, followed by MCP handshake.
type nextResource struct {
	key, generation string
	cfg             ServerConfig
	policy          NextLeasePolicy
	idleTTL         time.Duration
	session         *StdioSession
	born            process.BirthIdentity
	state           string
	borrowers       map[string]bool
	inflight        int
	lastUsed        time.Time
	changed         chan struct{}
}

func (r *nextResource) signal() { close(r.changed); r.changed = make(chan struct{}) }

type NextStdioLeaseController struct {
	mu        sync.Mutex
	resources map[string]*nextResource
	capacity  int
	closed    bool
}

// Constructor is intentionally not wired to stable/Next Runtime. Only a future
// Next-specific admission path may instantiate it and pass registry-verified
// `standalone` config + its immutable generation. Capacity bounds STARTING too.
func NewNextStdioLeaseController(capacity int) (*NextStdioLeaseController, error) {
	if capacity < 1 || capacity > 32 {
		return nil, errNextLeaseDenied
	}
	return &NextStdioLeaseController{resources: make(map[string]*nextResource), capacity: capacity}, nil
}
func nextKey(owner, server string) string { return owner + "\x00" + server }
func newNextToken() (string, error) {
	var id [24]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}
func validNextID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if r <= 32 || r > 126 {
			return false
		}
	}
	return true
}
func validNextPolicy(p NextLeasePolicy, ttl time.Duration) bool {
	return (p == NextLeaseEphemeral && ttl == 0 || p == NextLeasePersistent && ttl == 0 || p == NextLeaseIdleManaged && ttl >= time.Second && ttl <= 24*time.Hour)
}

// Acquire never shares stdio across ACP owners: owner MUST be an exact Next
// logical Session owner, not an external process PID or executable name.
// Only a byte-identical ServerConfig bound to a pinned registry Generation is
// reused. A change in generation is denied until the old lease drains.
func (c *NextStdioLeaseController) Acquire(ctx context.Context, owner string, cfg ServerConfig, policy NextLeasePolicy, ttl time.Duration) (NextStdioLease, error) {
	if c == nil || ctx == nil || ctx.Err() != nil || !validNextID(owner) || !validNextID(cfg.Name) || !validNextID(cfg.Generation) || !validNextPolicy(policy, ttl) || cfg.SourceType != "standalone" || cfg.PluginName != "" || !cfg.Enabled || cfg.Transport != TransportStdio || strings.TrimSpace(cfg.Command) == "" {
		return NextStdioLease{}, errNextLeaseDenied
	}
	token, err := newNextToken()
	if err != nil {
		return NextStdioLease{}, errNextLeaseDenied
	}
	key := nextKey(owner, cfg.Name)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return NextStdioLease{}, errNextLeaseDenied
	}
	if old := c.resources[key]; old != nil {
		if old.generation != cfg.Generation || old.state != "ready" || old.policy != policy || old.idleTTL != ttl || !sameNextConfig(old.cfg, cfg) {
			c.mu.Unlock()
			return NextStdioLease{}, errNextLeaseDenied
		}
		old.borrowers[token] = true
		old.lastUsed = time.Now()
		old.signal()
		c.mu.Unlock()
		return NextStdioLease{key, token, cfg.Generation}, nil
	}
	if len(c.resources) >= c.capacity {
		c.mu.Unlock()
		return NextStdioLease{}, errNextLeaseDenied
	}
	rec := &nextResource{key: key, generation: cfg.Generation, cfg: normalizeServerConfig(cfg), policy: policy, idleTTL: ttl, state: "starting", borrowers: map[string]bool{}, changed: make(chan struct{})}
	c.resources[key] = rec
	c.mu.Unlock()
	var born process.BirthIdentity
	session, openErr := OpenStdioSessionWithStartedWitness(ctx, cfg, func(ctx context.Context, pid int) error {
		if pid <= 0 || ctx == nil || ctx.Err() != nil {
			return errNextLeaseDenied
		}
		child, err := process.ObserveBirthIdentity(ctx, pid)
		if err != nil || child.ParentPID != os.Getpid() || child.EffectiveUID != uint32(os.Geteuid()) || child.StartSec <= 0 {
			return errNextLeaseDenied
		}
		born = child
		return nil
	})
	c.mu.Lock()
	if openErr != nil || ctx.Err() != nil || c.closed || born.PID == 0 {
		// If a child was spawned, an ambiguous failed handshake/cleanup MUST
		// reserve a quarantine slot. Otherwise a retry could launch a second
		// process while the first remains alive. No PID-based kill is allowed.
		if born.PID == 0 {
			delete(c.resources, key)
		} else {
			rec.state = "cleanup_required"
		}
		rec.signal()
		c.mu.Unlock()
		if session != nil {
			_ = session.Close()
		}
		return NextStdioLease{}, errNextLeaseDenied
	}
	// Spawn witness is a preliminary proof only; verify the same kernel identity
	// after handshake and before the resource becomes usable.
	live, verifyErr := process.ObserveBirthIdentity(ctx, born.PID)
	if verifyErr != nil || live != born || session.Info().PID != born.PID {
		rec.state = "cleanup_required"
		rec.signal()
		c.mu.Unlock()
		_ = session.Close()
		return NextStdioLease{}, errNextLeaseDenied
	}
	rec.born = born
	rec.session = session
	rec.borrowers[token] = true
	rec.state = "ready"
	rec.lastUsed = time.Now()
	rec.signal()
	c.mu.Unlock()
	return NextStdioLease{key, token, cfg.Generation}, nil
}

func sameNextConfig(a, b ServerConfig) bool {
	// The full normalized immutable registry config must agree; source identity
	// includes generation, executable/args, credentials and authority dimensions.
	a = normalizeServerConfig(a)
	b = normalizeServerConfig(b)
	return reflect.DeepEqual(a, b)
}

// Call is bounded by StdioSession's existing timeout; no new requests may
// start once draining or cleanup-required. Each matching token admits one call.
func (c *NextStdioLeaseController) Call(ctx context.Context, lease NextStdioLease, tool string, args map[string]any) (map[string]any, error) {
	if c == nil || ctx == nil || ctx.Err() != nil {
		return nil, errNextLeaseDenied
	}
	c.mu.Lock()
	r := c.resources[lease.key]
	if c.closed || r == nil || r.state != "ready" || r.generation != lease.generation || !r.borrowers[lease.id] {
		c.mu.Unlock()
		return nil, errNextLeaseDenied
	}
	r.inflight++
	r.signal()
	s := r.session
	c.mu.Unlock()
	defer func() { c.mu.Lock(); r.inflight--; r.lastUsed = time.Now(); r.signal(); c.mu.Unlock() }()
	return s.Call(ctx, tool, args)
}

// Release consumes exactly one borrower token. Ephemeral final release drains
// synchronously; persistent/idle-managed do not close while idle merely because
// a caller returned from a prompt. A token cannot be replayed after release.
func (c *NextStdioLeaseController) Release(ctx context.Context, lease NextStdioLease) error {
	if c == nil || ctx == nil {
		return errNextLeaseDenied
	}
	c.mu.Lock()
	r := c.resources[lease.key]
	if r == nil || r.generation != lease.generation || !r.borrowers[lease.id] {
		c.mu.Unlock()
		return errNextLeaseDenied
	}
	delete(r.borrowers, lease.id)
	r.lastUsed = time.Now()
	drain := len(r.borrowers) == 0 && r.policy == NextLeaseEphemeral
	if drain { // Claim final release under lock, before another Acquire.
		r.state = "draining"
	}
	r.signal()
	c.mu.Unlock()
	if drain {
		return c.finishDrain(ctx, r)
	}
	return nil
}

// Drain is explicitly generation-fenced, refuses outstanding borrowers, and
// provides no process PID kill/close primitive. Timeout marks cleanup_required
// and prevents any new Call/Acquire; retry may only close this owned session.
func (c *NextStdioLeaseController) Drain(ctx context.Context, owner, server, generation string) error {
	if !validNextID(owner) || !validNextID(server) || !validNextID(generation) {
		return errNextLeaseDenied
	}
	return c.drain(ctx, nextKey(owner, server), generation)
}
func (c *NextStdioLeaseController) drain(ctx context.Context, key, generation string) error {
	if ctx == nil {
		return errNextLeaseDenied
	}
	c.mu.Lock()
	r := c.resources[key]
	if r == nil || r.generation != generation || len(r.borrowers) != 0 || r.state == "starting" {
		c.mu.Unlock()
		return errNextLeaseDenied
	}
	if r.state == "draining" {
		c.mu.Unlock()
		return errNextLeaseDenied
	}
	if r.state != "ready" && r.state != "cleanup_required" {
		c.mu.Unlock()
		return errNextLeaseDenied
	}
	r.state = "draining"
	r.signal()
	c.mu.Unlock()
	return c.finishDrain(ctx, r)
}

func (c *NextStdioLeaseController) finishDrain(ctx context.Context, r *nextResource) error {
	c.mu.Lock()
	if ctx.Err() != nil {
		r.state = "cleanup_required"
		r.signal()
		c.mu.Unlock()
		return errNextLeaseDenied
	}

	for r.inflight != 0 {
		signal := r.changed
		c.mu.Unlock()
		select {
		case <-signal:
		case <-ctx.Done():
		}
		c.mu.Lock()
		if ctx.Err() != nil {
			r.state = "cleanup_required"
			r.signal()
			c.mu.Unlock()
			return errNextLeaseDenied
		}
	}
	session := r.session
	if session == nil {
		r.state = "cleanup_required"
		r.signal()
		c.mu.Unlock()
		return errNextLeaseDenied
	}
	c.mu.Unlock()
	// The existing StdioSession is the exclusive owner of its process handle and
	// Wait. Do NOT attempt direct os.Process.Kill or infer cleanup from PID age.
	err := session.Close()
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		r.state = "cleanup_required"
		r.signal()
		return errNextLeaseDenied
	}
	delete(c.resources, r.key)
	r.state = "closed"
	r.signal()
	return nil
}

func (c *NextStdioLeaseController) ReapIdle(ctx context.Context, now time.Time) int {
	if c == nil || ctx == nil || ctx.Err() != nil || now.IsZero() {
		return 0
	}
	c.mu.Lock()
	keys := make([]string, 0)
	for key, r := range c.resources {
		if r.policy == NextLeaseIdleManaged && r.state == "ready" && len(r.borrowers) == 0 && r.inflight == 0 && !now.Before(r.lastUsed.Add(r.idleTTL)) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	generations := make(map[string]string, len(keys))
	for _, key := range keys {
		generations[key] = c.resources[key].generation
	}
	c.mu.Unlock()
	count := 0
	for _, key := range keys {
		if ctx.Err() != nil {
			break
		}
		if c.drain(ctx, key, generations[key]) == nil {
			count++
		}
	}
	return count
}

func (c *NextStdioLeaseController) Snapshot() NextLeaseCounts {
	out := NextLeaseCounts{Namespace: "next"}
	if c == nil {
		return out
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out.Managed = len(c.resources)
	for _, r := range c.resources {
		out.Borrowers += len(r.borrowers)
		out.Inflight += r.inflight
		switch r.state {
		case "ready":
			out.Ready++
		case "starting":
			out.Starting++
		case "draining":
			out.Draining++
		case "cleanup_required":
			out.CleanupRequired++
		}
	}
	return out
}

// Destroy is deliberately absent: no global close-all command, no permission to
// kill unknown descendants and no implicit shared-HTTP/Browser/ACP ownership.

// NextLeaseResourceSummary is a strictly allowlisted candidate for Next's
// diagnostics UI. It excludes command/argv, env, tokens, OAuth details, tool
// payloads/replies, raw registry revision, kernel PID and process identity.
// A manager-owned Session ID is not an OS ownership attestation by itself.
type NextLeaseResourceSummary struct {
	Namespace       string          `json:"namespace"`
	ServerID        string          `json:"server_id"`
	OwnerSessionID  string          `json:"owner_acp_session"`
	ProviderKind    string          `json:"provider_kind"`
	State           string          `json:"actual_state"`
	Policy          NextLeasePolicy `json:"lifecycle_policy"`
	Borrowers       int             `json:"borrower_count"`
	Inflight        int             `json:"inflight_count"`
	LastActiveAt    time.Time       `json:"last_active_at,omitempty"`
	CleanupRequired bool            `json:"cleanup_required"`
}

func (c *NextStdioLeaseController) ResourceSnapshot() []NextLeaseResourceSummary {
	out := make([]NextLeaseResourceSummary, 0)
	if c == nil {
		return out
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.resources {
		owner, server, _ := strings.Cut(r.key, "\x00")
		out = append(out, NextLeaseResourceSummary{Namespace: "next", ServerID: server, OwnerSessionID: owner, ProviderKind: "standalone_stdio", State: r.state, Policy: r.policy, Borrowers: len(r.borrowers), Inflight: r.inflight, LastActiveAt: r.lastUsed, CleanupRequired: r.state == "cleanup_required"})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OwnerSessionID == out[j].OwnerSessionID {
			return out[i].ServerID < out[j].ServerID
		}
		return out[i].OwnerSessionID < out[j].OwnerSessionID
	})
	return out
}
