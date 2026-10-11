package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/permission"
)

var ErrNextM8NotAllowed = errors.New("Next-owned MCP request not authorized")

// NextStdioM8Context is ONLY constructed by trusted Next Core after verifying
// auth principal, ACP-session ownership, runtime epoch, and tool provenance.
// This value must not be hydrated from a client/ACP JSON request.
type NextStdioM8Context struct {
	CoreSupervisorM8Context
}

func (NextStdioM8Context) MarshalJSON() ([]byte, error) { return nil, ErrNextM8NotAllowed }

// CallAdmitted requires an exact M8 Allow BEFORE dispatch and binds a proposed
// side effect to the P5 Registry revision, lease capability, tool and canonical
// JSON args. It is intentionally source-only: it does NOT make separate P5 and
// M8 stores transactionally atomic across OS processes. A late revocation can
// suppress the reply but cannot undo an already dispatched tool side effect.
func (g *NextStdioRegistryGate) CallAdmitted(ctx context.Context, auth NextStdioM8Context, lease NextRegistryLease, tool string, args map[string]any) (result map[string]any, admitted permission.Admission, err error) {
	if g == nil || g.leases == nil || ctx == nil || ctx.Err() != nil || lease.lease.id == "" || lease.revision == "" || !validNextID(tool) || auth.Gate == nil || auth.Gate.Store() == nil ||
		!auth.Binding.Principal.Authenticated || !auth.Binding.Principal.Stable || !validNextID(auth.Binding.Principal.ID) || !validNextID(auth.Binding.Principal.Kind) ||
		auth.Binding.RuntimeEpoch == "" || auth.Binding.RuntimeEpoch != auth.Gate.Store().RuntimeEpoch() || auth.Binding.ACPSessionID == "" || !validNextID(auth.Audit.CallID) {
		return nil, permission.Admission{}, ErrNextM8NotAllowed
	}
	owner, server, ok := strings.Cut(lease.lease.key, "\x00")
	if !ok || !validNextID(owner) || owner != auth.Binding.ACPSessionID || server != lease.cfg.Name || lease.lease.generation != lease.cfg.Generation {
		return nil, permission.Admission{}, ErrNextM8NotAllowed
	}
	canonical, marshalErr := json.Marshal(args)
	if marshalErr != nil || len(canonical) > 64<<10 {
		return nil, permission.Admission{}, ErrNextM8NotAllowed
	}
	// Never pass raw args, tokens, environment or executable into permissions.
	fingerprintHash := sha256.New()
	for _, piece := range [][]byte{[]byte("agentdock-next-stdio-m8-v1"), []byte(lease.lease.id), []byte(owner), []byte(server), []byte(lease.revision), []byte(lease.lease.generation), []byte(tool), canonical} {
		_, _ = fingerprintHash.Write(piece)
		_, _ = fingerprintHash.Write([]byte{0})
	}
	fingerprint := "sha256:" + hex.EncodeToString(fingerprintHash.Sum(nil))
	scopedTool := "next_stdio_mcp/" + server + "/" + tool
	facts := permission.PermissionFacts{Binding: auth.Binding, Tool: scopedTool, Action: "call", MCP: true, Network: true, Other: true, ReadOnly: false, EffectsKnown: false, OpaqueProviderExecution: true, OneShotEligible: true, Filesystem: permission.FileNone, Reason: "Next owned MCP provider effects remain opaque"}
	prepared := permission.PreparedRequest{Fingerprint: fingerprint, Binding: auth.Binding, Tool: scopedTool, Action: "call", RuntimeEpoch: auth.Binding.RuntimeEpoch, Generations: map[string]string{"server": lease.lease.generation, "registry": lease.revision, "lease": nextM8LeaseDigest(lease.lease.id), "session": owner}}
	admitted, err = auth.Gate.Admit(ctx, permission.AdmissionRequest{Audit: auth.Audit, Facts: facts, Prepared: prepared, Summary: "Next MCP tool " + server + "/" + tool, Scope: "Next-owned standalone stdio MCP"})
	if err != nil || admitted.Decision.Effect != permission.Allow {
		return nil, admitted, ErrNextM8NotAllowed
	}
	// A consumed approved-once grant must ALWAYS be settled, including when
	// Registry check fails or tool outcome is ambiguous.
	outcome := permission.Unknown
	defer func() {
		if settleErr := SettleSupervisorM8Dispatch(context.Background(), auth.Gate, admitted, outcome); settleErr != nil {
			result = nil
			err = errors.Join(ErrNextM8NotAllowed, settleErr)
		}
	}()
	// Recheck M8 generation before entering the existing P5 checked call.
	policyRevision := admitted.Decision.PolicyRevision
	if ctx.Err() != nil || auth.Gate.Store().RuntimeEpoch() != auth.Binding.RuntimeEpoch || auth.Gate.Store().Policy().Revision != policyRevision {
		return nil, admitted, ErrNextM8NotAllowed
	}
	result, callErr := g.callChecked(ctx, lease, tool, args)
	// A delayed policy revocation is a failure to disclose a valid result.
	// It is not a process-control permission and is NOT an atomic dispatch
	// fence; see P0 gate documentation.
	if auth.Gate.Store().RuntimeEpoch() != auth.Binding.RuntimeEpoch || auth.Gate.Store().Policy().Revision != policyRevision {
		g.leases.invalidate(lease.lease)
		return nil, admitted, ErrNextM8NotAllowed
	}
	if callErr != nil {
		return nil, admitted, callErr
	}
	outcome = permission.Succeeded
	return result, admitted, nil
}

// The internal cryptographic borrower token must not be serialized to the
// M8 approval state or UI, even within exact generation metadata.
func nextM8LeaseDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// AcquireAdmitted gates PROCESS SPAWN, which is a side effect of its own.
// Core MUST pass a verified principal and exact ACP session owner. An Ask
// creates an approval but does not start a native process. An Allow reserves
// a P5 incarnation; on uncertainty only our freshly created child is closed.
// Source-only: no live Next Runtime caller and no claim of cross-process
// transaction atomicity across P5 and M8 state stores.
func (g *NextStdioRegistryGate) AcquireAdmitted(ctx context.Context, auth NextStdioM8Context, owner, server, revision, generation string, policy NextLeasePolicy, ttl time.Duration) (lease NextRegistryLease, admitted permission.Admission, err error) {
	if g == nil || g.leases == nil || ctx == nil || ctx.Err() != nil || auth.Gate == nil || auth.Gate.Store() == nil ||
		!auth.Binding.Principal.Authenticated || !auth.Binding.Principal.Stable || !validNextID(auth.Binding.Principal.ID) || !validNextID(auth.Binding.Principal.Kind) ||
		auth.Binding.RuntimeEpoch == "" || auth.Binding.RuntimeEpoch != auth.Gate.Store().RuntimeEpoch() || auth.Binding.ACPSessionID != owner || !validNextID(owner) || !validNextID(auth.Audit.CallID) || !validNextPolicy(policy, ttl) {
		return NextRegistryLease{}, permission.Admission{}, ErrNextM8NotAllowed
	}
	cfg, err := g.checkedConfig(server, revision, generation)
	if err != nil {
		return NextRegistryLease{}, permission.Admission{}, ErrNextM8NotAllowed
	}
	hash := sha256.New()
	for _, piece := range []string{"agentdock-next-stdio-m8-start-v1", owner, server, revision, generation, string(policy), strconv.FormatInt(int64(ttl), 10)} {
		_, _ = hash.Write([]byte(piece))
		_, _ = hash.Write([]byte{0})
	}
	toolID := "next_stdio_mcp/" + server + "/start"
	binding := auth.Binding
	facts := permission.PermissionFacts{Binding: binding, Tool: toolID, Action: "start", MCP: true, Commands: true, Management: true, Other: true, EffectsKnown: false, OpaqueProviderExecution: true, ReadOnly: false, Filesystem: permission.FileNone, OneShotEligible: true, Reason: "Next MCP direct-child stdio launch has opaque provider side effects"}
	prepared := permission.PreparedRequest{Fingerprint: "sha256:" + hex.EncodeToString(hash.Sum(nil)), Binding: binding, Tool: toolID, Action: "start", RuntimeEpoch: binding.RuntimeEpoch, Generations: map[string]string{"server": generation, "registry": revision, "session": owner}}
	admitted, err = auth.Gate.Admit(ctx, permission.AdmissionRequest{Audit: auth.Audit, Facts: facts, Prepared: prepared, Summary: "Next MCP start " + server, Scope: "Next-owned standalone stdio MCP launch"})
	if err != nil || admitted.Decision.Effect != permission.Allow {
		return NextRegistryLease{}, admitted, ErrNextM8NotAllowed
	}
	outcome := permission.Unknown
	defer func() {
		if settleErr := SettleSupervisorM8Dispatch(context.Background(), auth.Gate, admitted, outcome); settleErr != nil {
			if lease.lease.id != "" {
				g.leases.invalidate(lease.lease)
				_ = g.Release(context.Background(), lease)
			}
			lease = NextRegistryLease{}
			err = errors.Join(ErrNextM8NotAllowed, settleErr)
		}
	}()
	policyRevision := admitted.Decision.PolicyRevision
	if ctx.Err() != nil || auth.Gate.Store().RuntimeEpoch() != binding.RuntimeEpoch || auth.Gate.Store().Policy().Revision != policyRevision {
		return NextRegistryLease{}, admitted, ErrNextM8NotAllowed
	}
	lease, err = g.acquireChecked(ctx, owner, cfg.Name, revision, generation, policy, ttl)
	if err != nil {
		return NextRegistryLease{}, admitted, ErrNextM8NotAllowed
	}
	if ctx.Err() != nil || auth.Gate.Store().RuntimeEpoch() != binding.RuntimeEpoch || auth.Gate.Store().Policy().Revision != policyRevision {
		g.leases.invalidate(lease.lease)
		_ = g.Release(context.Background(), lease)
		lease = NextRegistryLease{}
		return lease, admitted, ErrNextM8NotAllowed
	}
	outcome = permission.Succeeded
	return lease, admitted, nil
}
