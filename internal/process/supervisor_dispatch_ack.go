package process

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// SupervisorDispatchAckFrame proves only that the same authenticated Core
// processed a specific Supervisor tool reply. It conveys NO permission to
// redispatch tools, rotate generations, restart workers, or clear fences.
type SupervisorDispatchAckFrame struct {
	Version       int    `json:"version"`
	Operation     string `json:"operation"`
	Session       string `json:"session"`
	Sequence      uint64 `json:"sequence"`
	RequestSHA256 string `json:"request_sha256"`
	ReplySHA256   string `json:"reply_sha256"`
	MAC           string `json:"mac"`
}

func supervisorDispatchAckPayload(f SupervisorDispatchAckFrame, core, supervisor, worker BirthIdentity) string {
	return fmt.Sprintf("agentdock-next-core-received-supervisor-dispatch-v1|%d|%s|%s|%d|%s|%s|%d,%d,%d,%d,%d,%d|%d,%d,%d,%d,%d,%d|%d,%d,%d,%d,%d,%d",
		f.Version, f.Operation, f.Session, f.Sequence, f.RequestSHA256, f.ReplySHA256,
		core.PID, core.ParentPID, core.StartSec, core.StartUsec, core.EffectiveUID, core.ProcessGroupID,
		supervisor.PID, supervisor.ParentPID, supervisor.StartSec, supervisor.StartUsec, supervisor.EffectiveUID, supervisor.ProcessGroupID,
		worker.PID, worker.ParentPID, worker.StartSec, worker.StartUsec, worker.EffectiveUID, worker.ProcessGroupID)
}

func validDispatchSHA(v string) bool {
	if len(v) != sha256.Size*2 {
		return false
	}
	for _, r := range v {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// SignSupervisorDispatchAckPrototype must be called by authenticated Core
// ONLY after validating the original IPC response. It signs canonical JSON
// bytes of the observed reply, not arbitrary Core-provided success claims.
func SignSupervisorDispatchAckPrototype(key []byte, core, supervisor, worker BirthIdentity,
	request SupervisorToolCallFrame, reply any) (SupervisorDispatchAckFrame, error) {
	if len(key) != 32 || !validLineageBirth(core) || !validLineageBirth(supervisor) ||
		!validLineageBirth(worker) || worker.ParentPID != supervisor.PID || request.Sequence == 0 {
		return SupervisorDispatchAckFrame{}, ErrSupervisorHandoffUnavailable
	}
	encoded, err := json.Marshal(reply)
	if err != nil || len(encoded) == 0 || len(encoded) > 8192 {
		return SupervisorDispatchAckFrame{}, ErrSupervisorHandoffUnavailable
	}
	requestDigest := dispatchACKRequestDigest(request)
	if !validDispatchSHA(requestDigest) {
		return SupervisorDispatchAckFrame{}, ErrSupervisorHandoffUnavailable
	}
	digest := sha256.Sum256(encoded)
	f := SupervisorDispatchAckFrame{
		Version: 1, Operation: "ack-core-received", Session: request.Session, Sequence: request.Sequence,
		RequestSHA256: requestDigest, ReplySHA256: hex.EncodeToString(digest[:]),
	}
	if !validDispatchACKShape(f) {
		return SupervisorDispatchAckFrame{}, ErrSupervisorHandoffUnavailable
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(supervisorDispatchAckPayload(f, core, supervisor, worker)))
	f.MAC = hex.EncodeToString(mac.Sum(nil))
	return f, nil
}

func validDispatchACKShape(f SupervisorDispatchAckFrame) bool {
	if f.Version != 1 || f.Operation != "ack-core-received" || f.Sequence == 0 ||
		!validDispatchSHA(f.RequestSHA256) || !validDispatchSHA(f.ReplySHA256) ||
		len(f.Session) != 32 {
		return false
	}
	for _, r := range f.Session {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// Build-independent canonical signed request digest. Darwin ledger uses the
// same canonical json.Marshal semantics in dispatchRequestDigest.
func dispatchACKRequestDigest(frame SupervisorToolCallFrame) string {
	raw, err := json.Marshal(frame)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
