//go:build !darwin

package browser

import (
	"context"
	"net"
)

func inspectNativeEdgeEstablishedSocket(context.Context, net.Conn, edgeProcessPreflight) (edgeSocketEvidence, error) {
	return edgeSocketEvidence{}, errEdgeProcessUnqualified
}
