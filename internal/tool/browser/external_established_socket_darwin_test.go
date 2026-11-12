//go:build darwin

package browser

import (
	"context"
	"net"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEdgeEstablishedSocketOutputBound(t *testing.T) {
	var out edgeSocketOutput
	if n, err := out.Write([]byte(strings.Repeat("x", edgeSocketLimit))); n != edgeSocketLimit || err != nil {
		t.Fatal("bounded output rejected")
	}
	if n, err := out.Write([]byte("secret overflow")); n != 0 || err != errEdgeProcessUnqualified || out.Len() != edgeSocketLimit {
		t.Fatal("overflow retained")
	}
}

// Exercise the actual macOS lsof filtering on two self-owned loopback
// connections. Never inspect Edge, the user's Profile or another process.
func TestEdgeEstablishedSocketMacProbeOnlyListsPinnedPort(t *testing.T) {
	owned := func() (*net.TCPConn, *net.TCPConn) {
		t.Helper()
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = l.Close() })
		client, err := net.DialTimeout("tcp4", l.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })
		server, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = server.Close() })
		return client.(*net.TCPConn), server.(*net.TCPConn)
	}
	client, server := owned()
	_, unrelated := owned()
	port := server.LocalAddr().(*net.TCPAddr).Port
	unrelatedPort := unrelated.LocalAddr().(*net.TCPAddr).Port
	if port == unrelatedPort {
		t.Fatal("ports unexpectedly aliased")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := (macEdgeSocketProbe{}).connections(ctx, os.Getpid(), port)
	if err != nil {
		t.Fatal("scoped socket observation failed", err)
	}
	serverAddr := netip.MustParseAddrPort(server.LocalAddr().String())
	clientAddr := netip.MustParseAddrPort(client.LocalAddr().String())
	if !edgeEstablishedSocketMatches(raw, os.Getpid(), serverAddr, clientAddr) {
		t.Fatal("exact reverse tuple missing from scoped observation")
	}
	if strings.Contains(raw, unrelated.LocalAddr().String()) {
		t.Fatal("unrelated port leaked into scoped evidence")
	}
	for _, bad := range []int{-1, 0, 65536} {
		if _, err := (macEdgeSocketProbe{}).connections(ctx, os.Getpid(), bad); err != errEdgeProcessUnqualified {
			t.Fatal("invalid port was allowed")
		}
	}
}
