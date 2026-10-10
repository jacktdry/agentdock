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

// SupervisorToolCallFrame is ONE scoped call request on a fresh, kernel-
// authenticated local Unix socket. A MAC is not a process-control capability.
type SupervisorToolCallFrame struct {
	Version         int             `json:"version"`
	Operation       string          `json:"operation"`
	Session         string          `json:"session"`
	Sequence        uint64          `json:"sequence"`
	Tool            string          `json:"tool"`
	WorkerPID       int             `json:"worker_pid"`
	WorkerStartSec  int64           `json:"worker_start_sec"`
	WorkerStartUsec int64           `json:"worker_start_usec"`
	Arguments       json.RawMessage `json:"arguments"`
	MAC             string          `json:"mac"`
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
	return fmt.Sprintf("agentdock-next-supervisor-tool-call-v1|%s|%d|%s|%s|%d|%s|%d,%d,%d|%x|%d,%d,%d,%d,%d,%d|%d,%d,%d,%d,%d,%d|%d,%d,%d,%d,%d,%d",
		owner, frame.Version, frame.Operation, frame.Session, frame.Sequence, frame.Tool,
		frame.WorkerPID, frame.WorkerStartSec, frame.WorkerStartUsec, digest,
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

// Local-scope authorization must occur BEFORE advancing the sequence.
// A caller needs BOTH a kernel-attested Core socket AND the Supervisor's own
// explicit tool allowlist. Every accepted request consumes its sequence even
// if the tool later fails, avoiding duplicate side effects on retry.
func (h *ReadOnlySupervisorHandoff) acceptVerifiedToolCall(peer BirthIdentity, frame SupervisorToolCallFrame, allowedTools map[string]struct{}) error {
	if h == nil {
		return ErrSupervisorHandoffUnavailable
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.phase != SupervisorCoreAttached || h.workerParent != SupervisorWorkerParentSupervisor ||
		peer != h.core || frame.Version != 1 || frame.Operation != "call-tool-scoped" ||
		frame.Session != h.challenge.Session || (h.lastToolSequence == ^uint64(0) || frame.Sequence != h.lastToolSequence+1) ||
		!validScopedToolName(frame.Tool) ||
		frame.WorkerPID != h.worker.PID || frame.WorkerStartSec != h.worker.StartSec ||
		frame.WorkerStartUsec != h.worker.StartUsec {
		return ErrSupervisorHandoffUnavailable
	}
	if _, ok := allowedTools[frame.Tool]; !ok {
		return ErrSupervisorHandoffUnavailable
	}
	args, err := canonicalToolArgs(frame.Arguments)
	if err != nil {
		return err
	}
	frame.Arguments = args
	given, err := hex.DecodeString(frame.MAC)
	if err != nil || len(given) != sha256.Size {
		return ErrSupervisorHandoffUnavailable
	}
	mac := hmac.New(sha256.New, h.key[:])
	_, _ = mac.Write([]byte(supervisorToolCallPayload(frame, h.core, h.supervisor, h.worker, h.workerParent)))
	if subtle.ConstantTimeCompare(given, mac.Sum(nil)) != 1 {
		return ErrSupervisorHandoffUnavailable
	}
	h.lastToolSequence = frame.Sequence
	return nil
}
