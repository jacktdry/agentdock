package process

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

// ErrSupervisorHandoffUnavailable fails closed; neither authenticated messages
// nor transitions confer signal, cleanup or crash-fence removal permissions.
var ErrSupervisorHandoffUnavailable = errors.New("read-only supervisor handoff unavailable")

// SupervisorWorkerParent distinguishes the actual OS parent. A Core-owned
// provenance claim must NEVER be reinterpreted as Supervisor-owned authority.
type SupervisorWorkerParent string

const (
	SupervisorWorkerParentCore       SupervisorWorkerParent = "core"
	SupervisorWorkerParentSupervisor SupervisorWorkerParent = "supervisor"
)

type SupervisorHandoffPhase string

const (
	SupervisorAwaitingCore SupervisorHandoffPhase = "awaiting_core"
	SupervisorCoreAttached SupervisorHandoffPhase = "core_attached"
	SupervisorCoreExited   SupervisorHandoffPhase = "core_exited"
	SupervisorWorkerExited SupervisorHandoffPhase = "wrapper_exit_observed"
	SupervisorUncertain    SupervisorHandoffPhase = "uncertain"
)

type SupervisorHandoffChallenge struct {
	Version int    `json:"version"`
	Session string `json:"session"`
	Nonce   string `json:"nonce"`
}

type SupervisorHandoffAttach struct {
	Version   int    `json:"version"`
	Operation string `json:"operation"`
	Session   string `json:"session"`
	Nonce     string `json:"nonce"`
	Sequence  uint64 `json:"sequence"`
	MAC       string `json:"mac"`
}

// ReadOnlySupervisorHandoff is an advisory, per-generation session; only the
// future supervisor should own this in memory. It has no process control APIs.
// Attach frames may only arrive over a kernel-verified local Core peer.
type ReadOnlySupervisorHandoff struct {
	mu           sync.Mutex
	core         BirthIdentity
	supervisor   BirthIdentity
	worker       BirthIdentity
	workerParent SupervisorWorkerParent
	key          [32]byte
	challenge    SupervisorHandoffChallenge
	phase        SupervisorHandoffPhase
}

// Secret is delivered through a future protected inherited IPC channel, not
// argv/env, TCP, a world-readable file or the handoff Unix socket itself.
func NewReadOnlySupervisorHandoff(core, supervisor, worker BirthIdentity, secret []byte) (*ReadOnlySupervisorHandoff, error) {
	return newReadOnlySupervisorHandoff(core, supervisor, worker, secret, SupervisorWorkerParentCore)
}

// NewSupervisorOwnedReadOnlyHandoff does not take ownership of any OS process.
// It only admits a Worker born as the Supervisor's DIRECT kernel child.
func NewSupervisorOwnedReadOnlyHandoff(core, supervisor, worker BirthIdentity, secret []byte) (*ReadOnlySupervisorHandoff, error) {
	return newReadOnlySupervisorHandoff(core, supervisor, worker, secret, SupervisorWorkerParentSupervisor)
}

func newReadOnlySupervisorHandoff(core, supervisor, worker BirthIdentity, secret []byte, owner SupervisorWorkerParent) (*ReadOnlySupervisorHandoff, error) {
	if len(secret) != 32 || !validLineageBirth(core) || !validLineageBirth(supervisor) ||
		!validLineageBirth(worker) || core.PID == supervisor.PID || worker.PID == core.PID ||
		worker.PID == supervisor.PID ||
		(owner != SupervisorWorkerParentCore && owner != SupervisorWorkerParentSupervisor) ||
		core.EffectiveUID != worker.EffectiveUID || supervisor.EffectiveUID != core.EffectiveUID ||
		(owner == SupervisorWorkerParentCore && (worker.ParentPID != core.PID || !bornNoEarlier(worker, core))) ||
		(owner == SupervisorWorkerParentSupervisor && (worker.ParentPID != supervisor.PID || !bornNoEarlier(worker, supervisor))) {
		return nil, ErrSupervisorHandoffUnavailable
	}
	var session, nonce [16]byte
	if _, err := rand.Read(session[:]); err != nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	h := &ReadOnlySupervisorHandoff{
		core: core, supervisor: supervisor, worker: worker, workerParent: owner,
		challenge: SupervisorHandoffChallenge{Version: 1, Session: hex.EncodeToString(session[:]), Nonce: hex.EncodeToString(nonce[:])},
		phase:     SupervisorAwaitingCore,
	}
	copy(h.key[:], secret)
	return h, nil
}

func (h *ReadOnlySupervisorHandoff) Challenge() (SupervisorHandoffChallenge, error) {
	if h == nil {
		return SupervisorHandoffChallenge{}, ErrSupervisorHandoffUnavailable
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.phase != SupervisorAwaitingCore {
		return SupervisorHandoffChallenge{}, ErrSupervisorHandoffUnavailable
	}
	return h.challenge, nil
}

// HandoffAttachMAC is used only by a verified Core after the secret has been
// conveyed through a protected channel. It binds identity, nonce and sequence.
func HandoffAttachMAC(secret []byte, challenge SupervisorHandoffChallenge,
	core, supervisor, worker BirthIdentity) (SupervisorHandoffAttach, error) {
	return HandoffAttachMACForWorkerParent(secret, challenge, core, supervisor, worker, SupervisorWorkerParentCore)
}

func HandoffAttachMACForWorkerParent(secret []byte, challenge SupervisorHandoffChallenge,
	core, supervisor, worker BirthIdentity, owner SupervisorWorkerParent) (SupervisorHandoffAttach, error) {
	if owner != SupervisorWorkerParentCore && owner != SupervisorWorkerParentSupervisor {
		return SupervisorHandoffAttach{}, ErrSupervisorHandoffUnavailable
	}
	if (owner == SupervisorWorkerParentCore && worker.ParentPID != core.PID) ||
		(owner == SupervisorWorkerParentSupervisor && worker.ParentPID != supervisor.PID) {
		return SupervisorHandoffAttach{}, ErrSupervisorHandoffUnavailable
	}
	if len(secret) != 32 || challenge.Version != 1 ||
		len(challenge.Session) != 32 || len(challenge.Nonce) != 32 {
		return SupervisorHandoffAttach{}, ErrSupervisorHandoffUnavailable
	}
	if _, err := hex.DecodeString(challenge.Session); err != nil {
		return SupervisorHandoffAttach{}, ErrSupervisorHandoffUnavailable
	}
	if _, err := hex.DecodeString(challenge.Nonce); err != nil {
		return SupervisorHandoffAttach{}, ErrSupervisorHandoffUnavailable
	}
	frame := SupervisorHandoffAttach{Version: 1, Operation: "attach-readonly", Session: challenge.Session, Nonce: challenge.Nonce, Sequence: 1}
	payload := supervisorHandoffPayload(frame, core, supervisor, worker, owner)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	frame.MAC = hex.EncodeToString(mac.Sum(nil))
	return frame, nil
}

func supervisorHandoffPayload(frame SupervisorHandoffAttach, core, supervisor, worker BirthIdentity, owner SupervisorWorkerParent) string {
	return fmt.Sprintf("agentdock-next-supervisor-handoff-v1|%s|%d|%s|%s|%s|%d|%d,%d,%d,%d,%d,%d|%d,%d,%d,%d,%d,%d|%d,%d,%d,%d,%d,%d",
		owner, frame.Version, frame.Operation, frame.Session, frame.Nonce, frame.Sequence,
		core.PID, core.ParentPID, core.StartSec, core.StartUsec, core.EffectiveUID, core.ProcessGroupID,
		supervisor.PID, supervisor.ParentPID, supervisor.StartSec, supervisor.StartUsec, supervisor.EffectiveUID, supervisor.ProcessGroupID,
		worker.PID, worker.ParentPID, worker.StartSec, worker.StartUsec, worker.EffectiveUID, worker.ProcessGroupID)
}

// Internal-only: the exported admission requires a kernel-attested Unix peer.
func (h *ReadOnlySupervisorHandoff) attachVerifiedPeer(peer BirthIdentity, frame SupervisorHandoffAttach) error {
	if h == nil {
		return ErrSupervisorHandoffUnavailable
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.phase != SupervisorAwaitingCore || peer != h.core ||
		frame.Version != 1 || frame.Operation != "attach-readonly" || frame.Sequence != 1 ||
		frame.Session != h.challenge.Session || frame.Nonce != h.challenge.Nonce {
		return ErrSupervisorHandoffUnavailable
	}
	got, err := hex.DecodeString(frame.MAC)
	if err != nil || len(got) != sha256.Size {
		return ErrSupervisorHandoffUnavailable
	}
	mac := hmac.New(sha256.New, h.key[:])
	_, _ = mac.Write([]byte(supervisorHandoffPayload(frame, h.core, h.supervisor, h.worker, h.workerParent)))
	if subtle.ConstantTimeCompare(got, mac.Sum(nil)) != 1 {
		return ErrSupervisorHandoffUnavailable
	}
	h.phase = SupervisorCoreAttached
	return nil
}

// Internal-only OS event entry points: the future supervisor must supply
// real kernel NOTE_EXIT events; never expose state changes over the socket.
// The following updates MUST be called only on actual supervisor-observed OS
// events, not on untrusted Core socket requests. Never infer full Chrome exit.
func (h *ReadOnlySupervisorHandoff) recordCoreExit(core BirthIdentity) error {
	if h == nil {
		return ErrSupervisorHandoffUnavailable
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if core != h.core || (h.phase != SupervisorAwaitingCore && h.phase != SupervisorCoreAttached) {
		return ErrSupervisorHandoffUnavailable
	}
	h.phase = SupervisorCoreExited
	return nil
}

func (h *ReadOnlySupervisorHandoff) recordWorkerExit(worker BirthIdentity) error {
	if h == nil {
		return ErrSupervisorHandoffUnavailable
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if worker != h.worker || (h.phase != SupervisorCoreAttached && h.phase != SupervisorCoreExited) {
		return ErrSupervisorHandoffUnavailable
	}
	h.phase = SupervisorWorkerExited
	return nil
}

func (h *ReadOnlySupervisorHandoff) MarkUncertain() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.phase = SupervisorUncertain
}

func (h *ReadOnlySupervisorHandoff) Phase() SupervisorHandoffPhase {
	if h == nil {
		return SupervisorUncertain
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.phase
}
