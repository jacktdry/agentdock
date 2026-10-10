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

const maxSupervisorACKFrame = 12288

type SupervisorCoreACKReply struct {
	Version      int    `json:"version"`
	Sequence     uint64 `json:"sequence"`
	Acknowledged bool   `json:"acknowledged"`
}

type supervisorACKEnvelope struct {
	Request processcontrol.SupervisorToolCallFrame    `json:"request"`
	ACK     processcontrol.SupervisorDispatchAckFrame `json:"ack"`
}

// One request on a NEW private Unix socket after Core has already received
// and validated the original durable tool reply. No ACP/browser routing.
func SendSupervisorCoreACKPrototype(ctx context.Context, conn *net.UnixConn,
	request processcontrol.SupervisorToolCallFrame, ack processcontrol.SupervisorDispatchAckFrame) error {
	if ctx == nil || ctx.Err() != nil || conn == nil {
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(supervisorToolIPCTimeout))
	payload, err := json.Marshal(supervisorACKEnvelope{Request: request, ACK: ack})
	if err != nil || len(payload) > maxSupervisorACKFrame {
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	payload = append(payload, byte(10))
	n, err := conn.Write(payload)
	if err != nil || n != len(payload) {
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	if err := conn.CloseWrite(); err != nil {
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	dec := json.NewDecoder(io.LimitReader(conn, 1025))
	dec.DisallowUnknownFields()
	var response SupervisorCoreACKReply
	if err := dec.Decode(&response); err != nil {
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	var more any
	if err := dec.Decode(&more); err != io.EOF {
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	if response.Version != 1 || response.Sequence != request.Sequence || !response.Acknowledged {
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	return nil
}

// Source-only Supervisor endpoint: a separate kernel Core peer+MAC auth,
// NOT a new tool execution. Success only means receipt was appended/fsynced.
func ServeSupervisorCoreACKPrototype(ctx context.Context, conn *net.UnixConn,
	handoff *processcontrol.ReadOnlySupervisorHandoff, ledger *processcontrol.SupervisorDispatchLedger) error {
	if ctx == nil || ctx.Err() != nil || conn == nil || handoff == nil || ledger == nil {
		if conn != nil {
			_ = conn.Close()
		}
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	dec := json.NewDecoder(io.LimitReader(conn, maxSupervisorACKFrame+1))
	dec.DisallowUnknownFields()
	var incoming supervisorACKEnvelope
	if err := dec.Decode(&incoming); err != nil {
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	if err := processcontrol.AcceptVerifiedSupervisorDispatchAckPrototype(ctx, conn, handoff, ledger,
		incoming.Request, incoming.ACK); err != nil {
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	raw, err := json.Marshal(SupervisorCoreACKReply{Version: 1, Sequence: incoming.ACK.Sequence, Acknowledged: true})
	if err != nil {
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	raw = append(raw, byte(10))
	n, err := conn.Write(raw)
	if err != nil || n != len(raw) {
		return processcontrol.ErrSupervisorDispatchLedgerUnavailable
	}
	return nil
}
