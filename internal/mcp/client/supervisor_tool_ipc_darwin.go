//go:build darwin

package client

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"time"

	processcontrol "github.com/uvwt/agentdock/internal/process"
)

const (
	supervisorToolIPCMaxFrame = 8192
	supervisorToolIPCMaxReply = 8192
	supervisorToolIPCTimeout  = 5 * time.Second
)

type SupervisorToolCallReply struct {
	Version  int            `json:"version"`
	Sequence uint64         `json:"sequence"`
	OK       bool           `json:"ok"`
	Result   map[string]any `json:"result,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// ServeSupervisorM8DurableMCPCallPrototype is a separate SOURCE-ONLY route.
// It requires M8 approval plus the Supervisor's owner-private write-ahead
// receipt BEFORE calling a tool; the SDK outcome receipt is fsynced BEFORE
// any success response can be sent. This does not enable Core recovery.
func ServeSupervisorM8DurableMCPCallPrototype(ctx context.Context, conn *net.UnixConn,
	handoff *processcontrol.ReadOnlySupervisorHandoff,
	session *SupervisorOwnedMCPSession, allowedTools map[string]struct{},
	ledger *processcontrol.SupervisorDispatchLedger) error {
	if ledger == nil {
		if conn != nil {
			_ = conn.Close()
		}
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	return serveSupervisorScopedMCPCall(ctx, conn, handoff, session, allowedTools, true, ledger)
}

// ServeSupervisorM8ScopedMCPCallPrototype extends the authenticated, scoped
// prototype with a mandatory MAC-bound M8 Allow claim. It remains source-only
// until the real Next Core admission path and principal provider are wired.
func ServeSupervisorM8ScopedMCPCallPrototype(ctx context.Context, conn *net.UnixConn,
	handoff *processcontrol.ReadOnlySupervisorHandoff,
	session *SupervisorOwnedMCPSession, allowedTools map[string]struct{}) error {
	return serveSupervisorScopedMCPCall(ctx, conn, handoff, session, allowedTools, true, nil)
}

// ServeSupervisorScopedMCPCallPrototype is a ONE-REQUEST Unix socket endpoint
// run only INSIDE the independent Supervisor process. It never transfers
// raw stdio or an SDK session handle to the Core and never affects PID cleanup.
// The allowedTools map MUST be configured locally by the Supervisor and not
// supplied through the Core request. No actual BrowserRegistry wiring exists.
func ServeSupervisorScopedMCPCallPrototype(ctx context.Context, conn *net.UnixConn,
	handoff *processcontrol.ReadOnlySupervisorHandoff,
	session *SupervisorOwnedMCPSession, allowedTools map[string]struct{}) error {
	return serveSupervisorScopedMCPCall(ctx, conn, handoff, session, allowedTools, false, nil)
}

func serveSupervisorScopedMCPCall(ctx context.Context, conn *net.UnixConn,
	handoff *processcontrol.ReadOnlySupervisorHandoff,
	session *SupervisorOwnedMCPSession, allowedTools map[string]struct{}, requireM8 bool,
	ledger *processcontrol.SupervisorDispatchLedger) error {
	if ctx == nil || ctx.Err() != nil || conn == nil || handoff == nil || session == nil ||
		len(allowedTools) == 0 || !handoff.MatchesSupervisorOwnedWorker(session.workerBirth) {
		return processcontrol.ErrSupervisorHandoffUnavailable
	}
	_ = conn.SetDeadline(time.Now().Add(supervisorToolIPCTimeout))
	defer conn.Close()
	var frame processcontrol.SupervisorToolCallFrame
	dec := json.NewDecoder(io.LimitReader(conn, supervisorToolIPCMaxFrame+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&frame); err != nil {
		return processcontrol.ErrSupervisorHandoffUnavailable
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return processcontrol.ErrSupervisorHandoffUnavailable
	}
	// M8-gated endpoint does not accept earlier allowlist-only frames.
	// Permission claims are in the call HMAC, verified again by process gate.
	if requireM8 && !processcontrol.ValidSupervisorPermissionProof(frame.Permission) {
		_ = writeSupervisorToolReply(conn, SupervisorToolCallReply{Version: 1, Sequence: frame.Sequence, OK: false, Error: "denied"})
		return processcontrol.ErrSupervisorHandoffUnavailable
	}
	// Before a tool can run, bind the same socket to the actual kernel Core
	// UID/PID/birth AND consume exactly one signed, locally scoped sequence.
	var ticket *processcontrol.SupervisorDispatchTicket
	var admitErr error
	if ledger != nil {
		ticket, admitErr = processcontrol.AcceptVerifiedSupervisorToolCallDurablePrototype(ctx, conn, handoff, frame, allowedTools, ledger)
	} else {
		admitErr = processcontrol.AcceptVerifiedSupervisorToolCallPrototype(ctx, conn, handoff, frame, allowedTools)
	}
	if admitErr != nil {
		// A denial is not an MCP tool result. Never include auth/key/argv details.
		_ = writeSupervisorToolReply(conn, SupervisorToolCallReply{
			Version: 1, Sequence: frame.Sequence, OK: false, Error: "denied",
		})
		return processcontrol.ErrSupervisorHandoffUnavailable
	}
	if ticket != nil {
		defer ticket.Close()
	}
	var args map[string]any
	if err := json.Unmarshal(frame.Arguments, &args); err != nil || args == nil {
		if ticket != nil {
			handoff.MarkUncertain()
		}
		return processcontrol.ErrSupervisorHandoffUnavailable
	}
	result, err := session.Call(ctx, frame.Tool, args)
	response := SupervisorToolCallReply{Version: 1, Sequence: frame.Sequence, OK: err == nil}
	if err != nil {
		response.Error = "tool_failed"
	} else {
		response.Result = result
	}
	if ticket != nil {
		raw, marshalErr := json.Marshal(response)
		if marshalErr != nil || len(raw) > supervisorToolIPCMaxReply {
			_ = ticket.Finish("unknown", nil)
			handoff.MarkUncertain()
			return processcontrol.ErrSupervisorDispatchLedgerUnavailable
		}
		outcome := "succeeded"
		if err != nil {
			outcome = "failed"
		}
		if finishErr := ticket.Finish(outcome, raw); finishErr != nil {
			handoff.MarkUncertain()
			return processcontrol.ErrSupervisorDispatchLedgerUnavailable
		}
	}
	if replyErr := writeSupervisorToolReply(conn, response); replyErr != nil {
		// Supervisor's SDK outcome may be durable while Core has not ACKed.
		// Never mark this disconnected attempt as safe to retry.
		if ticket != nil {
			handoff.MarkUncertain()
		}
		return replyErr
	}
	return nil
}
func writeSupervisorToolReply(conn *net.UnixConn, response SupervisorToolCallReply) error {
	raw, err := json.Marshal(response)
	if err != nil || len(raw) > supervisorToolIPCMaxReply {
		return processcontrol.ErrSupervisorHandoffUnavailable
	}
	raw = append(raw, '\n')
	n, err := conn.Write(raw)
	if err != nil || n != len(raw) {
		return processcontrol.ErrSupervisorHandoffUnavailable
	}
	return nil
}

// Core-side, ONE frame per connection. The socket must be created only over
// a verified private endpoint; the prior authenticated handoff is separate.
// This prototype is not production identity/authorization management.
func CallSupervisorScopedMCPPrototype(ctx context.Context, conn *net.UnixConn,
	frame processcontrol.SupervisorToolCallFrame) (SupervisorToolCallReply, error) {
	if ctx == nil || ctx.Err() != nil || conn == nil {
		return SupervisorToolCallReply{}, processcontrol.ErrSupervisorHandoffUnavailable
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(supervisorToolIPCTimeout))
	raw, err := json.Marshal(frame)
	if err != nil || len(raw) > supervisorToolIPCMaxFrame {
		return SupervisorToolCallReply{}, processcontrol.ErrSupervisorHandoffUnavailable
	}
	raw = append(raw, '\n')
	if _, err := conn.Write(raw); err != nil {
		return SupervisorToolCallReply{}, err
	}
	// The request is exactly one bounded JSON record. Half-close write before
	// the Supervisor verifies trailing EOF, while leaving response readable.
	if err := conn.CloseWrite(); err != nil {
		return SupervisorToolCallReply{}, err
	}
	decoder := json.NewDecoder(io.LimitReader(conn, supervisorToolIPCMaxReply+1))
	decoder.DisallowUnknownFields()
	var reply SupervisorToolCallReply
	if err := decoder.Decode(&reply); err != nil {
		return reply, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return reply, processcontrol.ErrSupervisorHandoffUnavailable
	}
	if reply.Version != 1 || reply.Sequence != frame.Sequence ||
		(reply.OK && reply.Error != "") || (!reply.OK && reply.Error == "") {
		return reply, processcontrol.ErrSupervisorHandoffUnavailable
	}
	return reply, nil
}
