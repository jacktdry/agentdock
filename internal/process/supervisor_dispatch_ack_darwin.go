//go:build darwin

package process

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// The ACK endpoint authenticates THIS fresh socket's live Core PID, UID and
// kernel birth. A valid stale MAC with a different process peer is NOT enough.
func AcceptVerifiedSupervisorDispatchAckPrototype(ctx context.Context, conn *net.UnixConn,
	handoff *ReadOnlySupervisorHandoff, ledger *SupervisorDispatchLedger,
	request SupervisorToolCallFrame, ack SupervisorDispatchAckFrame) error {
	if ctx == nil || ctx.Err() != nil || conn == nil || handoff == nil || ledger == nil ||
		!validDispatchACKShape(ack) ||
		ack.Session != request.Session || ack.Sequence != request.Sequence ||
		ack.RequestSHA256 != dispatchACKRequestDigest(request) {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	handoff.mu.Lock()
	defer handoff.mu.Unlock()
	if handoff.phase != SupervisorCoreAttached ||
		handoff.workerParent != SupervisorWorkerParentSupervisor ||
		handoff.challenge.Session != ack.Session ||
		handoff.worker.PID != request.WorkerPID ||
		handoff.worker.StartSec != request.WorkerStartSec ||
		handoff.worker.StartUsec != request.WorkerStartUsec {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	child, err := ObserveDirectChildBirth(ctx, handoff.supervisor.PID, handoff.worker.PID)
	if err != nil || child != handoff.worker {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	if err := VerifySupervisorHandoffPeer(ctx, conn, handoff.core); err != nil {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	given, err := hex.DecodeString(ack.MAC)
	if err != nil || len(given) != sha256.Size {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	mac := hmac.New(sha256.New, handoff.key[:])
	_, _ = mac.Write([]byte(supervisorDispatchAckPayload(ack, handoff.core, handoff.supervisor, handoff.worker)))
	if subtle.ConstantTimeCompare(given, mac.Sum(nil)) != 1 {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	if err := ledger.ackVerified(request, ack, handoff.core, handoff.worker); err != nil {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	return nil
}

// ackVerified NEVER changes MCP tool admission, permission approval, or
// Core rotation. The receipt's accepted+settled contents must be proven
// safe before a third, non-authorizing "acknowledged" record is appended.
func (l *SupervisorDispatchLedger) ackVerified(request SupervisorToolCallFrame,
	ack SupervisorDispatchAckFrame, core, worker BirthIdentity) error {
	if l == nil || !validDispatchACKShape(ack) ||
		ack.Session != request.Session || ack.Sequence != request.Sequence ||
		ack.RequestSHA256 != dispatchACKRequestDigest(request) {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	name, err := supervisorReceiptName(request)
	if err != nil {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || !safeSupervisorDispatchDir(l.dir) {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	fd, err := unix.Openat(l.dir, name, unix.O_RDWR|unix.O_APPEND|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	defer unix.Close(fd)
	st, ok := safeSupervisorReceipt(fd)
	if !ok || st.Size <= 0 || st.Size > supervisorReceiptMaxBytes {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	var byName unix.Stat_t
	if unix.Fstatat(l.dir, name, &byName, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		st.Dev != byName.Dev || st.Ino != byName.Ino || byName.Mode&unix.S_IFMT != unix.S_IFREG ||
		byName.Uid != uint32(os.Geteuid()) || byName.Mode&0777 != 0600 || byName.Nlink != 1 {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	raw := make([]byte, int(st.Size))
	n, err := unix.Pread(fd, raw, 0)
	if err != nil || n != len(raw) {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	current, err := parseSupervisorDispatchReceipt(raw, request)
	if err != nil || current.State != "settled" || current.Acknowledged ||
		current.Outcome == "unknown" || current.ReplySHA256 != ack.ReplySHA256 ||
		current.RequestSHA256 != ack.RequestSHA256 || current.Core != core ||
		current.Worker != worker {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	dirCopy, err := unix.Dup(l.dir)
	if err != nil {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	// Ticket only closes its dedicated duplicated descriptors.
	fileCopy, err := unix.Dup(fd)
	if err != nil {
		_ = unix.Close(dirCopy)
		return ErrSupervisorDispatchLedgerUnavailable
	}
	ticket := &SupervisorDispatchTicket{fd: fileCopy, dir: dirCopy,
		name: name, dev: uint64(st.Dev), ino: st.Ino}
	defer ticket.Close()
	if err := ticket.writeSync(SupervisorDispatchSnapshot{State: "acknowledged",
		ReplySHA256: ack.ReplySHA256}); err != nil {
		return ErrSupervisorDispatchLedgerUnavailable
	}
	return nil
}
