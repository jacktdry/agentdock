//go:build !darwin

package launchregistry

import (
	"context"
	"github.com/uvwt/agentdock/internal/process"
	"net"
)

type Registrar struct {
	AppServer    process.BirthIdentity
	Server       string
	ConfigDigest string
}

func (r Registrar) Accept(_ context.Context, _ *net.UnixConn) (Observation, error) {
	return Observation{}, ErrUnavailable
}
