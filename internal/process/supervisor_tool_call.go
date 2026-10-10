package process

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const SupervisorToolCallMaxArguments = 4096

// SupervisorPermissionProof is a MAC-bound claim minted ONLY by trusted Core
// after its M8 admission gate returned Allow. It never authorizes directly:
// Supervisor must ALSO prove OS peer, worker generation, sequence and scope.
type SupervisorPermissionProof struct {
	Version            uint8  `json:"version"`
	Effect             string `json:"effect"`
	PolicyRevision     uint64 `json:"policy_revision"`
	RuntimeEpoch       string `json:"runtime_epoch"`
	Fingerprint        string `json:"fingerprint"`
	PrincipalHash      string `json:"principal_hash"`
	ConsumedApprovalID string `json:"consumed_approval_id,omitempty"`
}

func ValidSupervisorPermissionProof(proof *SupervisorPermissionProof) bool {
	if proof == nil || proof.Version != 1 || proof.Effect != "allow" || proof.PolicyRevision == 0 ||
		len(proof.RuntimeEpoch) == 0 || len(proof.RuntimeEpoch) > 128 ||
		len(proof.PrincipalHash) == 0 || len(proof.PrincipalHash) > 128 ||
		len(proof.ConsumedApprovalID) > 128 || !strings.HasPrefix(proof.Fingerprint, "sha256:") || len(proof.Fingerprint) != 71 {
		return false
	}
	_, err := hex.DecodeString(proof.Fingerprint[7:])
	return err == nil
}

// SupervisorToolCallFrame is ONE scoped call request on a fresh, kernel-
// authenticated local Unix socket. A MAC is not a process-control capability.
type SupervisorToolCallFrame struct {
	Version         int                        `json:"version"`
	Operation       string                     `json:"operation"`
	Session         string                     `json:"session"`
	Sequence        uint64                     `json:"sequence"`
	Tool            string                     `json:"tool"`
	WorkerPID       int                        `json:"worker_pid"`
	WorkerStartSec  int64                      `json:"worker_start_sec"`
	WorkerStartUsec int64                      `json:"worker_start_usec"`
	Arguments       json.RawMessage            `json:"arguments"`
	Permission      *SupervisorPermissionProof `json:"permission,omitempty"`
	MAC             string                     `json:"mac"`
}

func validScopedToolName(name string) bool {
	if len(name) == 0 || len(name) > 128 || strings.Contains(name, "..") {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' || r == '/') {
			return false
		}
	}
	return true
}

func canonicalToolArgs(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > SupervisorToolCallMaxArguments {
		return nil, ErrSupervisorHandoffUnavailable
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil || buf.Len() > SupervisorToolCallMaxArguments {
		return nil, ErrSupervisorHandoffUnavailable
	}
	data := buf.Bytes()
	if data[0] != '{' || data[len(data)-1] != '}' {
		return nil, ErrSupervisorHandoffUnavailable
	}
	// No extra values or non-object arguments. JSON decoder also validates its
	// final tool schema; no input bytes are ever interpreted as process commands.
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil || parsed == nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	return data, nil
}

func supervisorToolCallPayload(frame SupervisorToolCallFrame, core, supervisor, worker BirthIdentity, owner SupervisorWorkerParent) string {
	digest := sha256.Sum256(frame.Arguments)
	proofBytes, _ := json.Marshal(frame.Permission)
	proofDigest := sha256.Sum256(proofBytes)
	return fmt.Sprintf("agentdock-next-supervisor-tool-call-v1|%s|%d|%s|%s|%d|%s|%d,%d,%d|%x|%x|%d,%d,%d,%d,%d,%d|%d,%d,%d,%d,%d,%d|%d,%d,%d,%d,%d,%d",
		owner, frame.Version, frame.Operation, frame.Session, frame.Sequence, frame.Tool,
		frame.WorkerPID, frame.WorkerStartSec, frame.WorkerStartUsec, digest, proofDigest,
		core.PID, core.ParentPID, core.StartSec, core.StartUsec, core.EffectiveUID, core.ProcessGroupID,
		supervisor.PID, supervisor.ParentPID, supervisor.StartSec, supervisor.StartUsec, supervisor.EffectiveUID, supervisor.ProcessGroupID,
		worker.PID, worker.ParentPID, worker.StartSec, worker.StartUsec, worker.EffectiveUID, worker.ProcessGroupID)
}

// SignSupervisorOwnedToolCallPrototype is run ONLY inside an attested Core
// which received ephemeral key material via a separately protected channel.
// Never transport that key in the request or make it available to MCP tools.
func SignSupervisorOwnedToolCallPrototype(key []byte, challenge SupervisorHandoffChallenge,
	core, supervisor, worker BirthIdentity, sequence uint64, tool string, args map[string]any) (SupervisorToolCallFrame, error) {
	if len(key) != 32 || challenge.Version != 1 || len(challenge.Session) != 32 ||
		worker.ParentPID != supervisor.PID || sequence == 0 || !validScopedToolName(tool) {
		return SupervisorToolCallFrame{}, ErrSupervisorHandoffUnavailable
	}
	if _, err := hex.DecodeString(challenge.Session); err != nil {
		return SupervisorToolCallFrame{}, ErrSupervisorHandoffUnavailable
	}
	if args == nil {
		args = map[string]any{}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return SupervisorToolCallFrame{}, ErrSupervisorHandoffUnavailable
	}
	raw, err = canonicalToolArgs(raw)
	if err != nil {
		return SupervisorToolCallFrame{}, err
	}
	frame := SupervisorToolCallFrame{
		Version: 1, Operation: "call-tool-scoped", Session: challenge.Session, Sequence: sequence,
		Tool: tool, WorkerPID: worker.PID, WorkerStartSec: worker.StartSec,
		WorkerStartUsec: worker.StartUsec, Arguments: raw,
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(supervisorToolCallPayload(frame, core, supervisor, worker, SupervisorWorkerParentSupervisor)))
	frame.MAC = hex.EncodeToString(mac.Sum(nil))
	return frame, nil
}

// AttachSupervisorPermissionProof signs the M8 Allow claim into the existing
// call MAC. A legacy unsigned-approval frame is NOT valid for the M8 endpoint.
// This helper is strictly a cryptographic operation: only trusted Core may
// construct proof from actual AdmissionGate.Admit results, never wire input.
func AttachSupervisorPermissionProof(key []byte, core, supervisor, worker BirthIdentity,
	frame SupervisorToolCallFrame, proof SupervisorPermissionProof) (SupervisorToolCallFrame, error) {
	if len(key) != 32 || !ValidSupervisorPermissionProof(&proof) ||
		frame.Version != 1 || frame.Operation != "call-tool-scoped" ||
		frame.WorkerPID != worker.PID || frame.WorkerStartSec != worker.StartSec ||
		frame.WorkerStartUsec != worker.StartUsec || worker.ParentPID != supervisor.PID {
		return SupervisorToolCallFrame{}, ErrSupervisorHandoffUnavailable
	}
	frame.Permission = &proof
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(supervisorToolCallPayload(frame, core, supervisor, worker, SupervisorWorkerParentSupervisor)))
	frame.MAC = hex.EncodeToString(mac.Sum(nil))
	return frame, nil
}

// Local-scope authorization must occur BEFORE advancing the sequence.
// A caller needs BOTH a kernel-attested Core socket AND the Supervisor's own
// explicit tool allowlist. Every accepted request consumes its sequence even
// if the tool later fails, avoiding duplicate side effects on retry.
func (h *ReadOnlySupervisorHandoff) acceptVerifiedToolCall(peer BirthIdentity, frame SupervisorToolCallFrame, allowedTools map[string]struct{}) error {
	_, err := h.acceptVerifiedToolCallPrepared(peer, frame, allowedTools, nil)
	return err
}

// The write-ahead variant first authenticates the exact Core peer and tool
// request under the same HMAC+sequence lock, then durably accepts the request
// BEFORE consuming its sequence and BEFORE exposing execution permission.
// Missing/failed durable prepare permanently quarantines this generation.
func (h *ReadOnlySupervisorHandoff) acceptVerifiedToolCallPrepared(peer BirthIdentity,
	frame SupervisorToolCallFrame, allowedTools map[string]struct{},
	ledger *SupervisorDispatchLedger) (*SupervisorDispatchTicket, error) {
	if h == nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.phase != SupervisorCoreAttached || h.workerParent != SupervisorWorkerParentSupervisor ||
		peer != h.core || frame.Version != 1 || frame.Operation != "call-tool-scoped" ||
		frame.Session != h.challenge.Session || (h.lastToolSequence == ^uint64(0) || frame.Sequence != h.lastToolSequence+1) ||
		!validScopedToolName(frame.Tool) ||
		frame.WorkerPID != h.worker.PID || frame.WorkerStartSec != h.worker.StartSec ||
		frame.WorkerStartUsec != h.worker.StartUsec {
		return nil, ErrSupervisorHandoffUnavailable
	}
	if _, ok := allowedTools[frame.Tool]; !ok {
		return nil, ErrSupervisorHandoffUnavailable
	}
	args, err := canonicalToolArgs(frame.Arguments)
	if err != nil {
		return nil, err
	}
	frame.Arguments = args
	given, err := hex.DecodeString(frame.MAC)
	if err != nil || len(given) != sha256.Size {
		return nil, ErrSupervisorHandoffUnavailable
	}
	mac := hmac.New(sha256.New, h.key[:])
	_, _ = mac.Write([]byte(supervisorToolCallPayload(frame, h.core, h.supervisor, h.worker, h.workerParent)))
	if subtle.ConstantTimeCompare(given, mac.Sum(nil)) != 1 {
		return nil, ErrSupervisorHandoffUnavailable
	}
	if ledger != nil {
		ticket, err := ledger.BeginVerified(frame, h.core, h.worker)
		if err != nil {
			h.phase = SupervisorUncertain
			return nil, ErrSupervisorHandoffUnavailable
		}
		h.lastToolSequence = frame.Sequence
		return ticket, nil
	}
	h.lastToolSequence = frame.Sequence
	return nil, nil
}
