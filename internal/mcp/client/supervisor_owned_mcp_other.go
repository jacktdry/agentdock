//go:build !darwin

package client

import (
	"context"
	processcontrol "github.com/uvwt/agentdock/internal/process"
)

type SupervisorOwnedMCPSession struct{}

func ConnectSupervisorOwnedMCPPrototype(context.Context, ServerConfig, *processcontrol.SupervisorOwnedStdio) (*SupervisorOwnedMCPSession, error) {
	return nil, processcontrol.ErrSupervisorHandoffUnavailable
}
func (*SupervisorOwnedMCPSession) Info() SessionInfo      { return SessionInfo{} }
func (*SupervisorOwnedMCPSession) Tools() map[string]Tool { return nil }
func (*SupervisorOwnedMCPSession) Call(context.Context, string, map[string]any) (map[string]any, error) {
	return nil, processcontrol.ErrSupervisorHandoffUnavailable
}
func (*SupervisorOwnedMCPSession) Close() error {
	return processcontrol.ErrSupervisorHandoffUnavailable
}
