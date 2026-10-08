//go:build darwin && !cgo

package browser

import (
	"context"
	"os"
	"testing"
)

func TestEdgeLiveSignatureNoCGORejects(t *testing.T) {
	f := edgeFixture()
	f.liveSignatureErr = (macEdgeProcessProbe{}).liveSignature(context.Background(), os.Getpid())
	e, err := readEdgeProcessPreflight(context.Background(), "ws://127.0.0.1:9222/devtools/browser/candidate", "/tmp/edge-fixture", f)
	if err != errEdgeProcessUnqualified || e != (edgeProcessPreflight{}) || f.liveSignatureCalls != 1 || f.listenerCalls != 1 {
		t.Fatal("no-cgo fallback qualified or exposed process evidence")
	}
}
