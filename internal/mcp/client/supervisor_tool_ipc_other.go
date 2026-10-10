//go:build !darwin

package client

import (
	"context"
	processcontrol "github.com/uvwt/agentdock/internal/process"
	"net"
)

type SupervisorToolCallReply struct {
	Version  int            `json:"version"`
	Sequence uint64         `json:"sequence"`
	OK       bool           `json:"ok"`
	Result   map[string]any `json:"result,omitempty"`
	Error    string         `json:"error,omitempty"`
}

func ServeSupervisorScopedMCPCallPrototype(context.Context, *net.UnixConn,
	*processcontrol.ReadOnlySupervisorHandoff, *SupervisorOwnedMCPSession, map[string]struct{}) error {
	return processcontrol.ErrSupervisorHandoffUnavailable
}
func CallSupervisorScopedMCPPrototype(context.Context, *net.UnixConn,
	processcontrol.SupervisorToolCallFrame) (SupervisorToolCallReply, error) {
	return SupervisorToolCallReply{}, processcontrol.ErrSupervisorHandoffUnavailable
}

func ServeSupervisorM8ScopedMCPCallPrototype(context.Context, *net.UnixConn,
	*processcontrol.ReadOnlySupervisorHandoff, *SupervisorOwnedMCPSession, map[string]struct{}) error {
	return processcontrol.ErrSupervisorHandoffUnavailable
}
