//go:build !darwin || !cgo

package desktopcontrol

import (
	"context"
	"net"
)

// No supported kernel-process + Next GUI proof on this platform/build.
func VerifiedNextSender(context.Context) bool                             { return false }
func acceptedPeerContext(ctx context.Context, _ net.Conn) context.Context { return ctx }
