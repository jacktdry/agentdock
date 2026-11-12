package mcpowneraudit

import (
	"context"
	"crypto/rand"
	"sort"
	"time"

	"github.com/uvwt/agentdock/internal/process"
)

const correlationScope = "agentdock_next_codex_stdio_v1"
const maxCorrelationRows = 1024
const correlationWindow = 5 * time.Second

type evidenceProvenance uint8

const (
	authenticatedAdapterDiagnostic evidenceProvenance = iota + 1
	kernelStdioPipeObservation
)

// All inputs are package-private capabilities, not wire DTOs. There is NO live
// implementation today. A future bridge must authenticate its diagnostic peer
// against the manager's pinned adapter incarnation, not accept an authenticated
// boolean from that peer. It must capture records under the manager's read lock
// and fence generation changes until publication. RemoteSessionID is a native
// thread ID only when the pinned adapter explicitly guarantees that equivalence.
type adapterBinding struct {
	incarnation     uint64
	adapter, native process.BirthIdentity
}

// Projection of SessionRecord.ID/RemoteSessionID; no title, cwd or other PII.
type managedThread struct {
	local, thread string
	incarnation   uint64
}

type diagnosticRequest struct {
	binding   adapterBinding
	challenge [32]byte
	issued    time.Time
}

type evidenceStamp struct {
	binding    adapterBinding
	provenance evidenceProvenance
	challenge  [32]byte
	scope      string
	observed   time.Time
	complete   bool
}

// Pipe identities are kernel object identities (not FD numbers or guessed
// labels). The future OS observer must prove the native process holds the
// opposite endpoints of these exact child's stdin/stdout pipes, exclusively
// bound to this transport within this complete snapshot.
type stdioPipes struct{ stdin, stdout uint64 }
type transportChild struct {
	transport, thread string
	child             process.BirthIdentity
	pipes             stdioPipes
}
type nativeEvidence struct {
	stamp    evidenceStamp
	loaded   []string
	children []transportChild
}
type kernelTransport struct {
	child process.BirthIdentity
	pipes stdioPipes
}
type pipeEvidence struct {
	stamp    evidenceStamp
	children []kernelTransport
}

// Implementations must return immutable, bounded snapshots and respect ctx.
// Private methods prevent an arbitrary external wire source from implementing
// this contract. These interfaces encode a future trusted integration boundary;
// they do not themselves authenticate data or provide an OS pipe observer.
type nativeDiagnosticSource interface {
	nativeSnapshot(context.Context, diagnosticRequest) (nativeEvidence, error)
}
type kernelPipeSource interface {
	pipeSnapshot(context.Context, diagnosticRequest, []transportChild) (pipeEvidence, error)
}

type diagnosticCorrelation struct {
	Owner           string `json:"owner"`
	Qualification   string `json:"qualification"`
	ManagedMappings int    `json:"managed_mappings"`
	CorrelatedPIDs  []int  `json:"correlated_pids"`
}

// correlate is deliberately not called by Observe or any CLI/HTTP/ACP handler.
// Its result is diagnostic only, never a close/kill capability. No input is
// stored or mutated, and failures publish no partial correlation or raw errors.
func correlate(ctx context.Context, binding adapterBinding, records []managedThread, native nativeDiagnosticSource, pipes kernelPipeSource, kernel source, now func() time.Time) diagnosticCorrelation {
	fail := diagnosticCorrelation{Owner: "owner_unverified", Qualification: "not_qualified", CorrelatedPIDs: []int{}}
	if ctx == nil || native == nil || pipes == nil || kernel.table == nil || kernel.birth == nil || now == nil || binding.incarnation == 0 || !validIdentity(binding.adapter) || !validIdentity(binding.native) || binding.native.ParentPID != binding.adapter.PID || binding.native.PID == binding.adapter.PID || binding.native.EffectiveUID != binding.adapter.EffectiveUID || len(records) > maxCorrelationRows {
		return fail
	}
	ctx, cancel := context.WithTimeout(ctx, correlationWindow)
	defer cancel()
	req := diagnosticRequest{binding: binding, issued: now()}
	if _, err := rand.Read(req.challenge[:]); err != nil {
		return fail
	}
	locals, managed := map[string]bool{}, map[string]bool{}
	for _, r := range records {
		if !validOpaqueID(r.local) || !validOpaqueID(r.thread) || r.incarnation != binding.incarnation || locals[r.local] || managed[r.thread] {
			return fail
		}
		locals[r.local], managed[r.thread] = true, true
	}
	n, err := native.nativeSnapshot(ctx, req)
	if err != nil || !validStamp(n.stamp, req, now(), authenticatedAdapterDiagnostic) || len(n.loaded) > maxCorrelationRows || len(n.children) > maxCorrelationRows {
		return fail
	}
	p, err := pipes.pipeSnapshot(ctx, req, n.children)
	if err != nil || !validStamp(p.stamp, req, now(), kernelStdioPipeObservation) || len(p.children) > maxCorrelationRows || len(p.children) != len(n.children) {
		return fail
	}
	loaded := map[string]bool{}
	for _, id := range n.loaded {
		if !validOpaqueID(id) || loaded[id] {
			return fail
		}
		loaded[id] = true
	}
	pipeByPID := map[int]kernelTransport{}
	for _, child := range p.children {
		if _, exists := pipeByPID[child.child.PID]; exists {
			return fail
		}
		pipeByPID[child.child.PID] = child
	}
	// Reuse the read-only kernel collector and its final birth/relation rechecks.
	r := collect(ctx, Options{RootPID: binding.adapter.PID, MaxNodes: maxCorrelationRows, MaxDepth: 32, Timeout: correlationWindow}, kernel)
	if r.Status != "observed" || ctx.Err() != nil || !validStamp(n.stamp, req, now(), authenticatedAdapterDiagnostic) || !validStamp(p.stamp, req, now(), kernelStdioPipeObservation) {
		return fail
	}
	identities := map[int]process.BirthIdentity{}
	for _, node := range r.Nodes {
		identities[node.PID] = process.BirthIdentity{PID: node.PID, ParentPID: node.PPID, StartSec: node.BirthSec, StartUsec: node.BirthUsec, EffectiveUID: node.UID, ProcessGroupID: node.PGID}
	}
	if identities[binding.adapter.PID] != binding.adapter || identities[binding.native.PID] != binding.native {
		return fail
	}
	transports, children, endpoints := map[string]bool{}, map[int]bool{}, map[uint64]bool{}
	result := fail
	result.ManagedMappings = len(records)
	for _, c := range n.children {
		kp, exists := pipeByPID[c.child.PID]
		if !validOpaqueID(c.transport) || !loaded[c.thread] || transports[c.transport] || children[c.child.PID] || !validIdentity(c.child) || c.child.ParentPID != binding.native.PID || c.child.EffectiveUID != binding.native.EffectiveUID || identities[c.child.PID] != c.child || !exists || kp.child != c.child || kp.pipes != c.pipes || c.pipes.stdin == 0 || c.pipes.stdout == 0 || c.pipes.stdin == c.pipes.stdout || endpoints[c.pipes.stdin] || endpoints[c.pipes.stdout] {
			return fail
		}
		transports[c.transport], children[c.child.PID] = true, true
		endpoints[c.pipes.stdin], endpoints[c.pipes.stdout] = true, true
		if managed[c.thread] {
			result.CorrelatedPIDs = append(result.CorrelatedPIDs, c.child.PID)
		}
	}
	if len(result.CorrelatedPIDs) > 0 {
		result.Qualification = "verified_diagnostic_correlation"
	}
	sort.Ints(result.CorrelatedPIDs)
	if ctx.Err() != nil || !validStamp(n.stamp, req, now(), authenticatedAdapterDiagnostic) || !validStamp(p.stamp, req, now(), kernelStdioPipeObservation) {
		return fail
	}
	return result
}

func validStamp(s evidenceStamp, r diagnosticRequest, now time.Time, provenance evidenceProvenance) bool {
	return s.provenance == provenance && s.complete && s.scope == correlationScope && s.binding == r.binding && s.challenge == r.challenge && !r.issued.IsZero() && !s.observed.Before(r.issued) && !s.observed.After(now) && !now.Before(r.issued) && now.Sub(r.issued) <= correlationWindow
}

func validOpaqueID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if c <= ' ' || c > '~' {
			return false
		}
	}
	return true
}
