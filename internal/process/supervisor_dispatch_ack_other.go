//go:build !darwin

package process

import (
	"context"
	"net"
)

func AcceptVerifiedSupervisorDispatchAckPrototype(context.Context, *net.UnixConn,
	*ReadOnlySupervisorHandoff, *SupervisorDispatchLedger, SupervisorToolCallFrame, SupervisorDispatchAckFrame) error {
	return ErrSupervisorDispatchLedgerUnavailable
}
