//go:build darwin

package process

import (
	"context"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// VerifySupervisorHandoffPeer requires peer OS PID, UID and full kernel birth.
// A verified socket peer is not itself proof that Core owns Chrome children.
func VerifySupervisorHandoffPeer(ctx context.Context, conn *net.UnixConn, expected BirthIdentity) error {
	if ctx == nil || ctx.Err() != nil || conn == nil || !validLineageBirth(expected) {
		return ErrSupervisorHandoffUnavailable
	}
	fd, err := conn.SyscallConn()
	if err != nil {
		return ErrSupervisorHandoffUnavailable
	}
	var pid int
	var uid uint32
	var peerErr error
	if err := fd.Control(func(raw uintptr) {
		pid, peerErr = unix.GetsockoptInt(int(raw), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		if peerErr != nil {
			return
		}
		cred, err := unix.GetsockoptXucred(int(raw), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if err != nil {
			peerErr = err
			return
		}
		uid = cred.Uid
	}); err != nil || peerErr != nil || pid != expected.PID ||
		uid != expected.EffectiveUID || uid != uint32(os.Geteuid()) {
		return ErrSupervisorHandoffUnavailable
	}
	before, err := ObserveBirthIdentity(ctx, pid)
	if err != nil || before != expected {
		return ErrSupervisorHandoffUnavailable
	}
	after, err := ObserveBirthIdentity(ctx, pid)
	if err != nil || after != expected {
		return ErrSupervisorHandoffUnavailable
	}
	return nil
}

// AcceptVerifiedSupervisorHandoff must be called AFTER a complete bounded
// frame is read from this same Unix socket. It binds the actual OS peer to
// immutable Core birth immediately before consuming the one-use challenge.
// A successful attach grants no process-control or crash-fence authority.
func AcceptVerifiedSupervisorHandoff(ctx context.Context, conn *net.UnixConn,
	handoff *ReadOnlySupervisorHandoff, frame SupervisorHandoffAttach) error {
	if handoff == nil {
		return ErrSupervisorHandoffUnavailable
	}
	handoff.mu.Lock()
	expected := handoff.core
	handoff.mu.Unlock()
	if err := VerifySupervisorHandoffPeer(ctx, conn, expected); err != nil {
		return ErrSupervisorHandoffUnavailable
	}
	return handoff.attachVerifiedPeer(expected, frame)
}
