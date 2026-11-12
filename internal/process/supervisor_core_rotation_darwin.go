//go:build darwin

package process

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"os"
	"sync"
)

// SupervisorCoreRotation is a SOURCE-ONLY, single-Supervisor process-lifetime
// generation gate, NOT a persistent crash recovery service. It holds no
// authority to terminate Chrome, unstick a guard, or clear an M8 approval.
// A missing durable request journal means ANY accepted tool call in a retired
// Core generation makes new Core admission permanently unavailable.
type SupervisorCoreRotation struct {
	mu           sync.Mutex
	supervisor   BirthIdentity
	worker       BirthIdentity
	active       *ReadOnlySupervisorHandoff
	watched      *ExitWitness
	generation   uint64
	exited       bool
	waiting      bool
	uncertain    bool
	consumedKeys [][32]byte
}

// NewSupervisorCoreRotation may only be called by the ACTUAL Supervisor
// owning this direct Worker child. It validates each live birth and registers
// Core NOTE_EXIT before returning an attachable handoff. No daemon is installed.
func NewSupervisorCoreRotation(ctx context.Context, core, supervisor, worker BirthIdentity,
	key []byte) (*SupervisorCoreRotation, error) {
	if ctx == nil || ctx.Err() != nil || len(key) != 32 ||
		!validLineageBirth(core) || !validLineageBirth(supervisor) || !validLineageBirth(worker) ||
		core.PID == supervisor.PID || core.PID == worker.PID {
		return nil, ErrSupervisorHandoffUnavailable
	}
	if supervisor.PID != os.Getpid() {
		return nil, ErrSupervisorHandoffUnavailable
	}
	liveSupervisor, err := ObserveBirthIdentity(ctx, supervisor.PID)
	if err != nil || liveSupervisor != supervisor {
		return nil, ErrSupervisorHandoffUnavailable
	}
	liveWorker, err := ObserveDirectChildBirth(ctx, supervisor.PID, worker.PID)
	if err != nil || liveWorker != worker {
		return nil, ErrSupervisorHandoffUnavailable
	}
	liveCore, err := ObserveBirthIdentity(ctx, core.PID)
	if err != nil || liveCore != core {
		return nil, ErrSupervisorHandoffUnavailable
	}
	h, err := NewSupervisorOwnedReadOnlyHandoff(core, supervisor, worker, key)
	if err != nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	watch, err := WatchVerifiedExit(ctx, core)
	if err != nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	digest := sha256.Sum256(key)
	return &SupervisorCoreRotation{
		supervisor: supervisor, worker: worker, active: h, watched: watch,
		generation: 1, consumedKeys: [][32]byte{digest},
	}, nil
}

// CurrentHandoff only exposes the current one-use Core attach session. The
// actual Unix peer kernel attestation and M8 AdmissionGate remain mandatory.
func (r *SupervisorCoreRotation) CurrentHandoff() (*ReadOnlySupervisorHandoff, error) {
	if r == nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.uncertain || r.exited || r.active == nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	return r.active, nil
}

func (r *SupervisorCoreRotation) Generation() uint64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.generation
}

// WitnessCoreExit consumes only the OS kernel NOTE_EXIT registered while the
// Core was still alive. It does NOT assert that an already accepted MCP tool
// completed, nor that detached Chrome processes have exited. A cancellation
// or ambiguous watch failure locks out future Core generations.
func (r *SupervisorCoreRotation) WitnessCoreExit(ctx context.Context) error {
	if r == nil || ctx == nil {
		return ErrSupervisorHandoffUnavailable
	}
	r.mu.Lock()
	if r.uncertain || r.exited || r.waiting || r.watched == nil || r.active == nil {
		r.mu.Unlock()
		return ErrSupervisorHandoffUnavailable
	}
	r.waiting = true
	watcher := r.watched
	prior := r.active
	r.mu.Unlock()

	// Waiting for a live Core to exit may take a long time. Never hold the
	// manager mutex during the kernel wait: active Core calls and live
	// Supervisor health checks must not be blocked by the observer.
	err := watcher.Wait(ctx)
	closeErr := watcher.Close()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.waiting = false
	r.watched = nil
	if r.uncertain || r.active != prior || err != nil || closeErr != nil {
		r.uncertain = true
		prior.MarkUncertain()
		return ErrSupervisorHandoffUnavailable
	}
	if err := prior.recordCoreExit(prior.core); err != nil {
		r.uncertain = true
		prior.MarkUncertain()
		return ErrSupervisorHandoffUnavailable
	}
	r.exited = true
	prior.mu.Lock()
	calls := prior.lastToolSequence
	prior.mu.Unlock()
	if calls != 0 {
		// The old Core may have died after a tool dispatched but before
		// reply. Without durable receipts, never restart a new generation.
		r.uncertain = true
		prior.MarkUncertain()
		return ErrSupervisorHandoffUnavailable
	}
	return nil
}

// AdmitSuccessor creates a FRESH per-Core HMAC epoch only after witnessed
// Core death and ONLY if no tool request was accepted in its old generation.
// It refuses key reuse, Core PID/birth reuse, worker replacement, and any
// ambiguous historical generation. The caller must privately provision the
// new secret to the new Core AFTER creating the authenticated session.
func (r *SupervisorCoreRotation) AdmitSuccessor(ctx context.Context, core BirthIdentity, key []byte) (
	*ReadOnlySupervisorHandoff, error) {
	if r == nil || ctx == nil || ctx.Err() != nil || len(key) != 32 {
		return nil, ErrSupervisorHandoffUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.uncertain || !r.exited || r.active == nil || r.watched != nil || r.waiting ||
		r.generation >= 16 || core == r.active.core ||
		!bornNoEarlier(core, r.active.core) ||
		core.PID == r.worker.PID || core.PID == r.supervisor.PID {
		return nil, ErrSupervisorHandoffUnavailable
	}
	digest := sha256.Sum256(key)
	for _, old := range r.consumedKeys {
		if subtle.ConstantTimeCompare(digest[:], old[:]) == 1 {
			return nil, ErrSupervisorHandoffUnavailable
		}
	}
	liveSupervisor, err := ObserveBirthIdentity(ctx, r.supervisor.PID)
	if err != nil || liveSupervisor != r.supervisor {
		return nil, ErrSupervisorHandoffUnavailable
	}
	liveWorker, err := ObserveDirectChildBirth(ctx, r.supervisor.PID, r.worker.PID)
	if err != nil || liveWorker != r.worker {
		return nil, ErrSupervisorHandoffUnavailable
	}
	liveCore, err := ObserveBirthIdentity(ctx, core.PID)
	if err != nil || liveCore != core {
		return nil, ErrSupervisorHandoffUnavailable
	}
	h, err := NewSupervisorOwnedReadOnlyHandoff(core, r.supervisor, r.worker, key)
	if err != nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	watch, err := WatchVerifiedExit(ctx, core)
	if err != nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	if h.challenge.Session == r.active.challenge.Session || h.challenge.Nonce == r.active.challenge.Nonce {
		_ = watch.Close()
		r.uncertain = true
		r.active.MarkUncertain()
		return nil, ErrSupervisorHandoffUnavailable
	}
	r.active = h
	r.watched = watch
	r.generation++
	r.exited = false
	r.consumedKeys = append(r.consumedKeys, digest)
	return h, nil
}

// MarkUncertain is irreversible. It never performs process cleanup or
// attempts a fresh generation without OS evidence and durable tool receipts.
func (r *SupervisorCoreRotation) MarkUncertain() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.uncertain = true
	if r.active != nil {
		r.active.MarkUncertain()
	}
}
