//go:build !darwin

package launchregistry

import (
	"context"
	"github.com/uvwt/agentdock/internal/process"
	"net"
)

func verifyRegistrarPeer(_ context.Context, _ *net.UnixConn, _ process.BirthIdentity) bool {
	return false
}
