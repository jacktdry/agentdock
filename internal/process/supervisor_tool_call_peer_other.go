//go:build !darwin

package process

import (
	"context"
	"net"
)

func AcceptVerifiedSupervisorToolCallPrototype(context.Context, *net.UnixConn,
	*ReadOnlySupervisorHandoff, SupervisorToolCallFrame, map[string]struct{}) error {
	return ErrSupervisorHandoffUnavailable
}
