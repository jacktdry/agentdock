package acp

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/uvwt/agentdock/internal/process"
)

const diagnosticScopeMaxLoaded = 1024
const diagnosticScopeWindow = 5 * time.Second

var errMCPDiagnosticNotQualified = errors.New("ACP MCP owner diagnostic not qualified")

// diagnosticLoadedRef is a snapshot of the Manager's own local loaded cache.
// AdapterSessionID is NOT a Codex native thread ID unless a separately pinned,
// authenticated Adapter extension explicitly proves that identity. Never
// serialize this type, nor send its IDs to logs, UI, ACP, HTTP or MCP tools.
type diagnosticLoadedRef struct {
	localSessionID, adapterSessionID string
}

type diagnosticManagerScope struct {
	adapter process.BirthIdentity
	loaded  []diagnosticLoadedRef
}

// withMCPDiagnosticScope is a SOURCE-ONLY, package-private bridge for future
// HIGH-0 diagnostic integration. There is intentionally no production caller.
// The callback MUST be trusted, synchronous, read-only and context-aware. A
// maximum 5-second *cooperative* deadline is enforced, not forcibly cancellable:
// an uncooperative callback can hold m.mu.RLock indefinitely. It must not call
// back into Manager methods requiring m.mu.Lock, nor start a process or do MCP
// work. Every failure returns a constant redacted error, never partial records.
//
// Holding the read lock across the callback prevents lifecycle map mutation
// (close/resume/rebind/adapter replacement) during evidence capture/publication.
// This is not native thread/stdio ownership proof and authorizes no cleanup.
func (m *Manager) withMCPDiagnosticScope(ctx context.Context, callback func(context.Context, diagnosticManagerScope) error) error {
	return m.withMCPDiagnosticScopePolicy(ctx, false, callback)
}

// allowEmpty only relaxes the requirement for at least one *loaded* ACP
// Session. It never creates an Adapter, bypasses transport/OS birth checks,
// or grants native MCP child ownership. Used by the Next B3 early diagnostic.
func (m *Manager) withMCPDiagnosticScopePolicy(ctx context.Context, allowEmpty bool, callback func(context.Context, diagnosticManagerScope) error) error {
	if m == nil || ctx == nil || callback == nil || ctx.Err() != nil {
		return errMCPDiagnosticNotQualified
	}
	ctx, cancel := context.WithTimeout(ctx, diagnosticScopeWindow)
	defer cancel()
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed || m.process == nil || m.process.pid <= 0 || m.process.connection == nil || m.process.command == nil || m.process.command.Process == nil || m.process.command.Process.Pid != m.process.pid || (len(m.loaded) == 0 && !allowEmpty) || len(m.loaded) > diagnosticScopeMaxLoaded {
		return errMCPDiagnosticNotQualified
	}
	connection := m.process.connection
	select {
	case <-connection.Closed():
		return errMCPDiagnosticNotQualified
	default:
	}
	adapter, err := process.ObserveBirthIdentity(ctx, m.process.pid)
	if err != nil || adapter.PID != m.process.pid || ctx.Err() != nil {
		return errMCPDiagnosticNotQualified
	}

	scope := diagnosticManagerScope{adapter: adapter, loaded: make([]diagnosticLoadedRef, 0, len(m.loaded))}
	seenRemote := make(map[string]bool, len(m.loaded))
	for localID, state := range m.loaded {
		record, ok := m.sessions[localID]
		if !ok || !diagnosticOpaqueID(localID) || record.ID != localID || !diagnosticOpaqueID(record.RemoteSessionID) ||
			m.remoteToLocal[record.RemoteSessionID] != localID || (state.SessionID != "" && state.SessionID != record.RemoteSessionID) ||
			seenRemote[record.RemoteSessionID] || (record.Status != SessionReady && record.Status != SessionRunning) {
			return errMCPDiagnosticNotQualified
		}
		if _, transitioning := m.terminalSessions[localID]; transitioning {
			return errMCPDiagnosticNotQualified
		}
		seenRemote[record.RemoteSessionID] = true
		scope.loaded = append(scope.loaded, diagnosticLoadedRef{localSessionID: localID, adapterSessionID: record.RemoteSessionID})
	}
	sort.Slice(scope.loaded, func(i, j int) bool { return scope.loaded[i].localSessionID < scope.loaded[j].localSessionID })
	// Only a package-internal trusted callback may observe the opaque IDs while
	// the Manager is read-locked. Avoid wrapping callback errors (secret leakage).
	if ctx.Err() != nil || callback(ctx, scope) != nil || ctx.Err() != nil {
		return errMCPDiagnosticNotQualified
	}
	current, err := process.ObserveBirthIdentity(ctx, adapter.PID)
	if err != nil || current != adapter || ctx.Err() != nil {
		return errMCPDiagnosticNotQualified
	}
	select {
	case <-connection.Closed():
		return errMCPDiagnosticNotQualified
	default:
	}
	return nil
}

func diagnosticOpaqueID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, b := range []byte(value) {
		if b <= ' ' || b > '~' {
			return false
		}
	}
	return true
}
