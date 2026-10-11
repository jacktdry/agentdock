//go:build !darwin

package launchregistry

import (
	"context"
	"github.com/uvwt/agentdock/internal/process"
)

type TrustedScope struct{ Adapter, AppServer process.BirthIdentity }
type ScopeProvider func(context.Context) (TrustedScope, error)
type PrivateCoreListener struct{}

func StartPrivateCoreListener(_ string, _ ScopeProvider, _, _ string) (*PrivateCoreListener, error) {
	return nil, ErrUnavailable
}
func (*PrivateCoreListener) SocketPath() string      { return "" }
func (*PrivateCoreListener) Snapshot() []Observation { return nil }
func (*PrivateCoreListener) Close() error            { return nil }
