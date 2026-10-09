//go:build darwin

package browser

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type macEdgeSocketProbe struct{ macEdgeProcessProbe }

// Bound allocation while collecting stdout, rather than after Output returns.
type edgeSocketOutput struct{ strings.Builder }

func (b *edgeSocketOutput) Write(p []byte) (int, error) {
	if len(p) > edgeSocketLimit-b.Len() {
		return 0, errEdgeProcessUnqualified
	}
	return b.Builder.Write(p)
}

func (macEdgeSocketProbe) connections(ctx context.Context, pid int) (string, error) {
	if ctx == nil || ctx.Err() != nil || pid <= 0 || int64(pid) > 2147483647 {
		return "", errEdgeProcessUnqualified
	}
	cmd := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-a", "-p", strconv.Itoa(pid), "-iTCP", "-sTCP:ESTABLISHED", "-FpnT")
	var out edgeSocketOutput
	cmd.Stdout = &out
	cmd.WaitDelay = 100 * time.Millisecond
	if cmd.Run() != nil || ctx.Err() != nil || out.Len() == 0 {
		return "", errEdgeProcessUnqualified
	}
	return out.String(), nil
}

// PRIVATE future attestor prerequisite; deliberately no production caller.
func inspectNativeEdgeEstablishedSocket(ctx context.Context, conn net.Conn, source edgeProcessPreflight) (edgeSocketEvidence, error) {
	return readEdgeEstablishedSocketEvidence(ctx, conn, source, macEdgeSocketProbe{})
}
