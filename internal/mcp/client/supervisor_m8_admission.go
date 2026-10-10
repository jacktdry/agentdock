package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/uvwt/agentdock/internal/permission"
	processcontrol "github.com/uvwt/agentdock/internal/process"
)

var ErrSupervisorM8NotAllowed = errors.New("Supervisor MCP request not admitted by M8")

// CoreSupervisorM8Context is populated by TRUSTED Core runtime code, never
// unmarshaled from a Core->Supervisor request, ACP payload or user-supplied
// ToolCall arguments. Caller must have already derived the principal from a
// verified Core auth context and registered its runtime epoch.
type CoreSupervisorM8Context struct {
	Gate    *permission.AdmissionGate
	Binding permission.PermissionBinding
	Audit   permission.AuditBinding
}

// AdmitAndSignSupervisorM8Call is the Core-side permission boundary for the
// source-only per-call Unix socket relay. It NEVER signs an Ask or Deny.
// M8's ApprovalStore records Ask and consumes an exact approved-once retry
// atomically before returning Allow; callers must settle consumed dispatches.
func AdmitAndSignSupervisorM8Call(ctx context.Context, admission CoreSupervisorM8Context,
	key []byte, challenge processcontrol.SupervisorHandoffChallenge,
	core, supervisor, worker processcontrol.BirthIdentity,
	sequence uint64, tool string, args map[string]any) (processcontrol.SupervisorToolCallFrame, permission.Admission, error) {
	var zero processcontrol.SupervisorToolCallFrame
	if ctx == nil || ctx.Err() != nil || admission.Gate == nil || admission.Gate.Store() == nil ||
		!admission.Binding.Principal.Authenticated || !admission.Binding.Principal.Stable ||
		strings.TrimSpace(admission.Binding.Principal.Kind) == "" || strings.TrimSpace(admission.Binding.Principal.ID) == "" ||
		strings.TrimSpace(admission.Binding.RuntimeEpoch) == "" ||
		admission.Binding.RuntimeEpoch != admission.Gate.Store().RuntimeEpoch() ||
		strings.TrimSpace(admission.Audit.CallID) == "" {
		return zero, permission.Admission{}, ErrSupervisorM8NotAllowed
	}
	// Build the EXACT frame before evaluating permissions; no syntactically
	// invalid or unbound frame can create a pending approval.
	frame, err := processcontrol.SignSupervisorOwnedToolCallPrototype(key, challenge, core, supervisor, worker, sequence, tool, args)
	if err != nil {
		return zero, permission.Admission{}, err
	}
	// Different tool identity than ordinary Core tools prevents a generic
	// rule granting a broad non-MCP operation from widening MCP permissions.
	toolID := "supervisor_mcp/" + tool
	hash := sha256.New()
	_, _ = hash.Write([]byte("agentdock-supervisor-m8-request-v1\x00"))
	for _, piece := range []string{toolID, challenge.Session,
		fmt.Sprintf("%d:%d:%d", worker.PID, worker.StartSec, worker.StartUsec),
		string(frame.Arguments)} {
		_, _ = hash.Write([]byte(piece))
		_, _ = hash.Write([]byte{0})
	}
	fingerprint := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	facts := permission.PermissionFacts{
		Binding: admission.Binding, Tool: toolID, Action: "call",
		EffectsKnown: false, OpaqueProviderExecution: true, ReadOnly: false,
		MCP: true, Network: true, Other: true, Filesystem: permission.FileNone,
		OneShotEligible: true, Reason: "Supervisor-owned MCP provider effects are opaque",
	}
	prepared := permission.PreparedRequest{
		Fingerprint: fingerprint, Binding: admission.Binding,
		Tool: toolID, Action: "call", RuntimeEpoch: admission.Binding.RuntimeEpoch,
		Generations: map[string]string{
			"worker":  fmt.Sprintf("%d:%d:%d", worker.PID, worker.StartSec, worker.StartUsec),
			"session": challenge.Session,
		},
	}
	result, err := admission.Gate.Admit(ctx, permission.AdmissionRequest{
		Audit: admission.Audit, Facts: facts, Prepared: prepared,
		Summary: "Supervisor MCP tool " + tool, Scope: "Next Core scoped Supervisor-owned MCP request",
	})
	if err != nil {
		return zero, permission.Admission{}, err
	}
	if result.Decision.Effect != permission.Allow {
		return zero, result, ErrSupervisorM8NotAllowed
	}
	audit := permission.AuditBindingFor(admission.Binding)
	proof := processcontrol.SupervisorPermissionProof{
		Version: 1, Effect: permission.Allow, PolicyRevision: result.Decision.PolicyRevision,
		RuntimeEpoch: admission.Binding.RuntimeEpoch, Fingerprint: fingerprint,
		PrincipalHash:      audit.PrincipalHash,
		ConsumedApprovalID: result.ConsumedApprovalID,
	}
	frame, err = processcontrol.AttachSupervisorPermissionProof(key, core, supervisor, worker, frame, proof)
	if err != nil {
		return zero, result, err
	}
	return frame, result, nil
}

// SettleSupervisorM8Dispatch is for approved-once calls only. The Core must
// settle Unknown when the IPC transport fails ambiguously, never retry a
// possibly dispatched action with the same approval.
func SettleSupervisorM8Dispatch(ctx context.Context, gate *permission.AdmissionGate,
	admission permission.Admission, outcome string) error {
	if admission.ConsumedApprovalID == "" {
		return nil
	}
	if gate == nil || gate.Store() == nil {
		return ErrSupervisorM8NotAllowed
	}
	switch outcome {
	case permission.Succeeded, permission.Failed, permission.Unknown:
	default:
		return ErrSupervisorM8NotAllowed
	}
	_, err := gate.Store().SettleDispatch(ctx, admission.ConsumedApprovalID, outcome)
	return err
}

// Explicitly reject direct serialization of Core authorization contexts:
// the Supervisor only sees bounded MAC-bound permission metadata, not the
// authenticated principal or M8 store internals.
func (CoreSupervisorM8Context) MarshalJSON() ([]byte, error) {
	return nil, ErrSupervisorM8NotAllowed
}

var _ = json.Valid
