package launchregistry

import (
	"context"
	"github.com/uvwt/agentdock/internal/process"
	"sync"
)

// GenerationPin is a nonblocking, read-only snapshot captured BEFORE Codex's
// first Session. The original snapshot MUST come from an authenticated Adapter
// challenge and independent kernel birth verification. Native MCP spawn cannot
// synchronously call into the Adapter while a thread/start waits for that MCP.
// Kernel parent/birth/UID are reverified by PrivateCoreListener on each call.
type GenerationPin struct {
	mu      sync.RWMutex
	scope   TrustedScope
	revoked bool
}

func NewGenerationPin(adapter, native process.BirthIdentity) (*GenerationPin, error) {
	if adapter.PID <= 0 || native.PID <= 0 || native.PID == adapter.PID || native.ParentPID != adapter.PID || native.EffectiveUID != adapter.EffectiveUID || adapter.StartSec <= 0 || native.StartSec <= 0 {
		return nil, ErrUnavailable
	}
	return &GenerationPin{scope: TrustedScope{Adapter: adapter, AppServer: native}}, nil
}

func (p *GenerationPin) Scope(ctx context.Context) (TrustedScope, error) {
	if p == nil || ctx == nil || ctx.Err() != nil {
		return TrustedScope{}, ErrUnavailable
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.revoked {
		return TrustedScope{}, ErrUnavailable
	}
	return p.scope, nil
}
func (p *GenerationPin) Revoke() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.revoked = true
	p.mu.Unlock()
}
