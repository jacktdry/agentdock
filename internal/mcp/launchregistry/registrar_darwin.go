//go:build darwin

package launchregistry

import (
	"bufio"
	"context"
	"net"
	"os"
	"time"

	"github.com/uvwt/agentdock/internal/process"
	"golang.org/x/sys/unix"
)

// Registrar accepts one two-stage launch report on an existing Unix socket.
// A separate Next Core authority must pin AppServer to an Adapter-owned
// generation before constructing this object; this package never installs a
// listener, assigns per-thread leases, signals a PID, or mutates P5/M8.
type Registrar struct {
	AppServer    process.BirthIdentity
	Server       string
	ConfigDigest string
}

func (r Registrar) Accept(ctx context.Context, conn *net.UnixConn) (Observation, error) {
	return r.AcceptWithCommit(ctx, conn, nil)
}

// AcceptWithCommit calls a Core-only callback AFTER kernel checks but BEFORE
// acknowledging registration to the wrapper. A failed dispatch/close fence
// must never be followed by a misleading "recorded" acknowledgement.
func (r Registrar) AcceptWithCommit(ctx context.Context, conn *net.UnixConn, commit func(Observation) error) (Observation, error) {
	if ctx == nil || conn == nil || ctx.Err() != nil || !validServer(r.Server) || !validDigest(r.ConfigDigest) {
		return Observation{}, ErrUnavailable
	}
	app, err := process.ObserveBirthIdentity(ctx, r.AppServer.PID)
	if err != nil || app != r.AppServer || app.EffectiveUID != uint32(os.Geteuid()) {
		return Observation{}, ErrUnavailable
	}
	peer, err := observePeer(ctx, conn)
	if err != nil || peer.ParentPID != app.PID || peer.EffectiveUID != app.EffectiveUID || peer.PID == app.PID {
		return Observation{}, ErrUnavailable
	}
	deadline := time.Now().Add(8 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if conn.SetDeadline(deadline) != nil {
		return Observation{}, ErrUnavailable
	}
	defer conn.SetDeadline(time.Time{})
	reader := bufio.NewReaderSize(conn, maxFrame)
	intent, err := readFrame(reader)
	if err != nil || intent.Stage != "intent" || intent.Server != r.Server || intent.ConfigDigest != r.ConfigDigest || intent.Child != nil || intent.Nonce != "" {
		return Observation{}, ErrUnavailable
	}
	// The preflight records only the wrapper peer, before the MCP child exists.
	nonce, err := newNonce()
	if err != nil {
		return Observation{}, ErrUnavailable
	}
	if writeFrame(conn, frame{Stage: "accepted", Nonce: nonce}) != nil {
		return Observation{}, ErrUnavailable
	}
	started, err := readFrame(reader)
	if err != nil || started.Stage != "started" || started.Nonce != nonce || started.Child == nil || started.Server != "" || started.ConfigDigest != "" {
		return Observation{}, ErrUnavailable
	}
	verified, err := process.ObserveBirthIdentity(ctx, started.Child.PID)
	if err != nil || verified != *started.Child || verified.ParentPID != peer.PID || verified.EffectiveUID != peer.EffectiveUID || verified.PID == peer.PID {
		return Observation{}, ErrUnavailable
	}
	// Resample both endpoints before publishing; this is diagnostic, not a
	// kernel-pinned exclusive transport proof or termination capability.
	peer2, err := observePeer(ctx, conn)
	if err != nil || peer2 != peer {
		return Observation{}, ErrUnavailable
	}
	app2, err := process.ObserveBirthIdentity(ctx, app.PID)
	if err != nil || app2 != app {
		return Observation{}, ErrUnavailable
	}
	child2, err := process.ObserveBirthIdentity(ctx, verified.PID)
	if err != nil || child2 != verified || ctx.Err() != nil {
		return Observation{}, ErrUnavailable
	}
	record := Observation{Server: r.Server, ConfigDigest: r.ConfigDigest, AppServer: app, Wrapper: peer, Child: verified}
	if commit != nil && commit(record) != nil {
		return Observation{}, ErrUnavailable
	}
	if writeFrame(conn, frame{Stage: "recorded", Nonce: nonce}) != nil {
		return Observation{}, ErrUnavailable
	}
	return record, nil
}

func observePeer(ctx context.Context, conn *net.UnixConn) (process.BirthIdentity, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return process.BirthIdentity{}, ErrUnavailable
	}
	pid := 0
	var uid uint32
	var peerErr error
	if err := raw.Control(func(fd uintptr) {
		pid, peerErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		if peerErr != nil {
			return
		}
		cred, e := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if e != nil {
			peerErr = e
			return
		}
		uid = cred.Uid
	}); err != nil || peerErr != nil || pid <= 0 || uid != uint32(os.Geteuid()) {
		return process.BirthIdentity{}, ErrUnavailable
	}
	identity, err := process.ObserveBirthIdentity(ctx, pid)
	if err != nil || identity.EffectiveUID != uid {
		return process.BirthIdentity{}, ErrUnavailable
	}
	return identity, nil
}
