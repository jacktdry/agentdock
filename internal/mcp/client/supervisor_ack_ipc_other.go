//go:build !darwin

package client

import (
	"context"
	processcontrol "github.com/uvwt/agentdock/internal/process"
	"net"
)

type SupervisorCoreACKReply struct {
	Version      int
	Sequence     uint64
	Acknowledged bool
}

func SendSupervisorCoreACKPrototype(context.Context, *net.UnixConn,
	processcontrol.SupervisorToolCallFrame, processcontrol.SupervisorDispatchAckFrame) error {
	return processcontrol.ErrSupervisorDispatchLedgerUnavailable
}
func ServeSupervisorCoreACKPrototype(context.Context, *net.UnixConn,
	*processcontrol.ReadOnlySupervisorHandoff, *processcontrol.SupervisorDispatchLedger) error {
	return processcontrol.ErrSupervisorDispatchLedgerUnavailable
}
