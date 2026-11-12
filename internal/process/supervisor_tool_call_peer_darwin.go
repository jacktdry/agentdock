//go:build darwin

package process

import (
	"context"
	"net"
)

// Durable variant guarantees a private receipt fsync BEFORE returning an
// authenticated tool execution permit. A refused fsync is never retried on
// another transport without an operator-owned reconciliation contract.
func AcceptVerifiedSupervisorToolCallDurablePrototype(ctx context.Context, conn *net.UnixConn,
	handoff *ReadOnlySupervisorHandoff, frame SupervisorToolCallFrame,
	allowedTools map[string]struct{}, ledger *SupervisorDispatchLedger) (*SupervisorDispatchTicket, error) {
	if ledger == nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	peer, err := verifySupervisorToolCallPeer(ctx, conn, handoff, allowedTools)
	if err != nil {
		return nil, err
	}
	return handoff.acceptVerifiedToolCallPrepared(peer, frame, allowedTools, ledger)
}

// Only this exported entry can consume a signed per-call sequence.
// The Unix socket peer must still be the SAME living Core birth/UID after
// reading the entire bounded frame. Legacy and non-Darwin paths fail closed.
func AcceptVerifiedSupervisorToolCallPrototype(ctx context.Context, conn *net.UnixConn,
	handoff *ReadOnlySupervisorHandoff, frame SupervisorToolCallFrame, allowedTools map[string]struct{}) error {
	peer, err := verifySupervisorToolCallPeer(ctx, conn, handoff, allowedTools)
	if err != nil {
		return err
	}
	return handoff.acceptVerifiedToolCall(peer, frame, allowedTools)
}
func verifySupervisorToolCallPeer(ctx context.Context, conn *net.UnixConn,
	handoff *ReadOnlySupervisorHandoff, allowedTools map[string]struct{}) (BirthIdentity, error) {
	if handoff == nil || conn == nil || len(allowedTools) == 0 {
		return BirthIdentity{}, ErrSupervisorHandoffUnavailable
	}
	handoff.mu.Lock()
	expected := handoff.core
	supervisor := handoff.supervisor
	worker := handoff.worker
	owner := handoff.workerParent
	handoff.mu.Unlock()
	if owner != SupervisorWorkerParentSupervisor {
		return BirthIdentity{}, ErrSupervisorHandoffUnavailable
	}
	// The authenticated Core may use only a LIVE worker that still has the
	// original Supervisor as its direct kernel parent, with exact birth.
	// A persisted worker PID alone never suffices for tool authorization.
	child, err := ObserveDirectChildBirth(ctx, supervisor.PID, worker.PID)
	if err != nil || child != worker {
		return BirthIdentity{}, ErrSupervisorHandoffUnavailable
	}
	if err := VerifySupervisorHandoffPeer(ctx, conn, expected); err != nil {
		return BirthIdentity{}, ErrSupervisorHandoffUnavailable
	}
	return expected, nil
}
