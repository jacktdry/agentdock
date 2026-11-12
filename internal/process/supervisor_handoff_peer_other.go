//go:build !darwin

package process

import (
	"context"
	"net"
)

func VerifySupervisorHandoffPeer(context.Context, *net.UnixConn, BirthIdentity) error {
	return ErrSupervisorHandoffUnavailable
}

// The local-kernel verified handoff is unavailable off macOS.
func AcceptVerifiedSupervisorHandoff(context.Context, *net.UnixConn,
	*ReadOnlySupervisorHandoff, SupervisorHandoffAttach) error {
	return ErrSupervisorHandoffUnavailable
}
