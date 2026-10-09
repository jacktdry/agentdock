package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TEST ONLY: count actual bytes, including partial HTTP requests that never
// reach an upgrade handler. No Edge process or profile is inspected.
type edgeEvidenceReadConn struct {
	net.Conn
	bytes *atomic.Int64
}

func (c *edgeEvidenceReadConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.bytes.Add(int64(n))
	return n, err
}

type edgeEvidenceListener struct {
	net.Listener
	bytes    atomic.Int64
	dials    atomic.Int32
	accepted chan net.Conn
	closed   chan struct{}
}

func (l *edgeEvidenceListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.dials.Add(1)
	l.accepted <- c
	return &edgeEvidenceReadConn{Conn: c, bytes: &l.bytes}, nil
}

func edgeEvidenceServer(t *testing.T) (*edgeEvidenceListener, edgeProcessPreflight) {
	t.Helper()
	upgraded := make(chan *websocket.Conn, 1)
	handlerDone := make(chan struct{})
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer close(handlerDone)
		c, err := (&websocket.Upgrader{}).Upgrade(w, req, nil)
		if err != nil {
			return
		}
		defer c.Close()
		upgraded <- c
		for {
			kind, data, err := c.ReadMessage()
			if err != nil || c.WriteMessage(kind, data) != nil {
				return
			}
		}
	}))
	l := &edgeEvidenceListener{Listener: s.Listener, accepted: make(chan net.Conn, 4), closed: make(chan struct{})}
	s.Listener = l
	// StateClosed also confirms EOF was observed on a rejected upstream socket.
	s.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			close(l.closed)
		}
	}
	s.Start()
	t.Cleanup(func() {
		select {
		case c := <-upgraded:
			_ = c.Close()
			pinnedWait(t, handlerDone)
		default:
		}
		s.Close()
	})
	return l, edgeProcessPreflight{pid: 1245, endpoint: "ws" + strings.TrimPrefix(s.URL, "http") + "/devtools/browser/fixture",
		executable: "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", userDataDir: "/private/edge-fixture",
		startToken: "501:Fri Oct 9 12:00:00 2026", observedAt: time.Now()}
}

type edgeEvidenceProbe struct {
	listener            *edgeEvidenceListener
	client, server      net.Conn
	token, calls, fault string
	cancel              context.CancelFunc
}

func (p *edgeEvidenceProbe) connections(ctx context.Context, pid int) (string, error) {
	p.calls += "c"
	if p.fault == "probe-panic" {
		panic("private probe canary")
	}
	if p.fault == "probe-deadline" {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if p.server == nil {
		select {
		case p.server = <-p.listener.accepted:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	server, client := p.server.LocalAddr(), p.server.RemoteAddr()
	if p.client != nil {
		// The recording decorator supplies the EXACT client pointer received by
		// the composition. Generate lsof's reversed tuple from that socket.
		server, client = p.client.RemoteAddr(), p.client.LocalAddr()
		if server.String() != p.server.LocalAddr().String() || client.String() != p.server.RemoteAddr().String() {
			return "", errors.New("different upstream socket")
		}
	}
	if p.fault == "pid" {
		pid++
	}
	if p.fault == "tuple" {
		server, client = client, server
	}
	if p.fault == "last-tuple" && p.calls == "ctsc" {
		client = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}
	}
	return fmt.Sprintf("p%d\nf7\nn%s->%s\nTST=ESTABLISHED\n", pid, server, client), nil
}

func (p *edgeEvidenceProbe) startToken(context.Context, int) (string, error) {
	p.calls += "t"
	if p.fault == "token" || p.fault == "last-token" && p.calls == "ctsct" {
		return "501:Fri Oct 9 12:00:01 2026", nil
	}
	return p.token, nil // Own synthetic epoch, independent of source input.
}

func (p *edgeEvidenceProbe) liveSignature(context.Context, int) error {
	p.calls += "s"
	if p.cancel != nil {
		p.cancel()
	}
	if p.fault == "signature" || p.fault == "last-signature" && p.calls == "ctscts" {
		return errors.New("private signature canary")
	}
	return nil
}

func edgeEvidenceSlotIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for len(pinnedCDPAttestationSlot) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("attestation worker retained")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestEdgeEvidencePinnedEcho(t *testing.T) {
	for _, instrument := range []bool{false, true} {
		t.Run(fmt.Sprint(instrument), func(t *testing.T) {
			l, source := edgeEvidenceServer(t)
			p := &edgeEvidenceProbe{listener: l, token: "501:Fri Oct 9 12:00:00 2026"}
			observed := pinnedTestPeer() // Fixture authority owns this before expected is supplied.
			attestor := func(_ context.Context, c net.Conn) (externalPeerIdentity, error) {
				if p.calls != "ctscts" || instrument && c != p.client {
					return externalPeerIdentity{}, errors.New("order/socket")
				}
				p.calls += "a"
				return observed, nil
			}
			var r *pinnedCDPRelay
			var err error
			if instrument {
				composed := edgeEvidencePinnedAttestor(source, p, attestor)
				r, err = newPinnedCDPRelay(context.Background(), source.endpoint, observed, func(ctx context.Context, c net.Conn) (externalPeerIdentity, error) {
					p.client = c
					return composed(ctx, c)
				})
			} else {
				r, err = newEdgeEvidencePinnedRelay(context.Background(), source.endpoint, source, p, observed, attestor)
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { r.Close(); pinnedWait(t, r.done) })
			c := pinnedAttach(t, r)
			if c.WriteMessage(websocket.TextMessage, []byte("B2l synthetic echo")) != nil {
				t.Fatal("write")
			}
			_, data, err := c.ReadMessage()
			if err != nil || string(data) != "B2l synthetic echo" || p.calls != "ctsctsa" || l.dials.Load() != 1 {
				t.Fatal("echo/order/redial")
			}
			endpoint := r.endpoint
			r.Close()
			pinnedWait(t, r.done)
			if retry, _, err := (&websocket.Dialer{HandshakeTimeout: time.Second}).Dial(endpoint, nil); err == nil {
				_ = retry.Close()
				t.Fatal("relay still listening")
			}
		})
	}
}

func TestEdgeEvidencePinnedReject(t *testing.T) {
	names := []string{"nil-probe", "typed-nil-probe", "nil-attestor", "zero-peer", "aliased-peer", "nil-context", "canceled", "stale", "future", "malformed-token", "executable", "profile", "profile-mismatch", "endpoint", "endpoint-path", "pid", "tuple", "last-tuple", "token", "last-token", "signature", "last-signature", "probe-panic", "probe-cancel", "probe-deadline", "attestor-mismatch", "attestor-zero", "attestor-error", "attestor-panic", "attestor-cancel", "attestor-deadline", "attestor-timeout"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			l, source := edgeEvidenceServer(t)
			p := &edgeEvidenceProbe{listener: l, token: "501:Fri Oct 9 12:00:00 2026", fault: name}
			var probe edgeSocketProbe = p
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var ctxArg context.Context = ctx
			observed := pinnedTestPeer()
			expected := observed
			var called atomic.Bool
			attestor := pinnedCDPAttestor(func(ctx context.Context, _ net.Conn) (externalPeerIdentity, error) {
				called.Store(true)
				// Synthetic independent policy knows its own profile; B2k only
				// validates preflight shape and cannot establish profile identity.
				if source.userDataDir != "/private/edge-fixture" {
					return pinnedTestPeer(), nil
				}
				switch name {
				case "attestor-mismatch":
					return pinnedTestPeer(), nil
				case "attestor-zero":
					return externalPeerIdentity{}, nil
				case "attestor-error":
					return observed, errors.New("private identity canary")
				case "attestor-panic":
					panic("private identity canary")
				case "attestor-cancel":
					cancel()
				case "attestor-deadline", "attestor-timeout":
					<-ctx.Done()
				}
				return observed, nil
			})
			endpoint := source.endpoint
			switch name {
			case "nil-probe":
				probe = nil
			case "typed-nil-probe":
				probe = (*edgeEvidenceProbe)(nil)
			case "nil-attestor":
				attestor = nil
			case "zero-peer":
				expected = externalPeerIdentity{}
			case "aliased-peer":
				expected.incarnation = expected.source
			case "nil-context":
				ctxArg = nil
			case "canceled":
				cancel()
			case "stale":
				source.observedAt = time.Now().Add(-3 * time.Second)
			case "future":
				source.observedAt = time.Now().Add(time.Second)
			case "malformed-token":
				source.startToken = "private malformed canary"
			case "executable":
				source.executable = "/tmp/Edge"
			case "profile":
				source.userDataDir = "/private/../edge"
			case "profile-mismatch":
				source.userDataDir = "/private/other-edge"
			case "endpoint":
				source.endpoint = "ws://127.0.0.1:1/devtools/browser/fixture"
			case "endpoint-path":
				source.endpoint += "-other"
			case "probe-cancel":
				p.cancel = cancel
			case "probe-deadline", "attestor-deadline":
				var stop context.CancelFunc
				ctxArg, stop = context.WithTimeout(ctx, 40*time.Millisecond)
				defer stop()
			}
			r, err := newEdgeEvidencePinnedRelay(ctxArg, endpoint, source, probe, expected, attestor)
			edgeEvidenceSlotIdle(t)
			if r != nil || err != errPinnedCDPUnavailable {
				if r != nil {
					r.Close()
				}
				t.Fatal("failure exposed relay/error")
			}
			noDial := name == "nil-probe" || name == "nil-attestor" || name == "zero-peer" || name == "aliased-peer" || name == "nil-context" || name == "canceled" || name == "endpoint" || name == "endpoint-path"
			if !noDial {
				pinnedWait(t, l.closed)
			}
			wantDials := int32(1)
			if noDial {
				wantDials = 0
			}
			if l.bytes.Load() != 0 || l.dials.Load() != wantDials {
				t.Fatal("handshake bytes or second dial")
			}
			secondStage := strings.HasPrefix(name, "attestor-") || name == "profile-mismatch"
			if called.Load() != secondStage {
				t.Fatal("independent attestor invoked after first-stage failure or omitted")
			}
		})
	}
}

func TestEdgeEvidencePinnedConcreteSocket(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	called := false
	attest := edgeEvidencePinnedAttestor(edgeProcessPreflight{}, &edgeEvidenceProbe{}, func(context.Context, net.Conn) (externalPeerIdentity, error) {
		called = true
		return pinnedTestPeer(), nil
	})
	for _, c := range []net.Conn{nil, (*net.TCPConn)(nil), a} {
		peer, err := attest(context.Background(), c)
		if err != errPinnedCDPUnavailable || peer != (externalPeerIdentity{}) || called {
			t.Fatal("nonconcrete socket admitted")
		}
	}
}
