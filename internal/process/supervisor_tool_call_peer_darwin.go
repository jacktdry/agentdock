//go:build darwin

package process

import (
	"context"
	"net"
)

// Only this exported entry can consume a signed per-call sequence.
// The Unix socket peer must still be the SAME living Core birth/UID after
// reading the entire bounded frame. Legacy and non-Darwin paths fail closed.
func AcceptVerifiedSupervisorToolCallPrototype(ctx context.Context, conn *net.UnixConn,
	handoff *ReadOnlySupervisorHandoff, frame SupervisorToolCallFrame, allowedTools map[string]struct{}) error {
	if handoff == nil || conn == nil || len(allowedTools) == 0 {
		return ErrSupervisorHandoffUnavailable
	}
	handoff.mu.Lock()
	expected := handoff.core
	supervisor := handoff.supervisor
	worker := handoff.worker
	owner := handoff.workerParent
	handoff.mu.Unlock()
	if owner != SupervisorWorkerParentSupervisor {
		return ErrSupervisorHandoffUnavailable
	}
	// The authenticated Core may use only a LIVE worker that still has the
	// original Supervisor as its direct kernel parent, with exact birth.
	// A persisted worker PID alone never suffices for tool authorization.
	child, err := ObserveDirectChildBirth(ctx, supervisor.PID, worker.PID)
	if err != nil || child != worker {
		return ErrSupervisorHandoffUnavailable
	}
	if err := VerifySupervisorHandoffPeer(ctx, conn, expected); err != nil {
		return ErrSupervisorHandoffUnavailable
	}
	return handoff.acceptVerifiedToolCall(expected, frame, allowedTools)
}
