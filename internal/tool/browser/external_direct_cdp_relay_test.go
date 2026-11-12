package browser

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func directStart(t *testing.T, ctx context.Context, f *pinnedFixture) (*directCDPRelay, externalPeerIdentity) {
	t.Helper()
	p := pinnedTestPeer()
	r, err := newDirectCDPRelay(ctx, f.endpoint, p, pinnedTestAttestor(p))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); pinnedWait(t, r.Done()) })
	return r, p
}

func directAttach(t *testing.T, r *directCDPRelay) *websocket.Conn {
	t.Helper()
	c, _, err := (&websocket.Dialer{Proxy: nil, HandshakeTimeout: time.Second}).Dial(r.Endpoint(), nil)
	if err != nil {
		t.Fatal("downstream attach failed")
	}
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func directDenied(t *testing.T, endpoint string, headers http.Header) {
	t.Helper()
	c, resp, err := (&websocket.Dialer{Proxy: nil, HandshakeTimeout: time.Second}).Dial(endpoint, headers)
	if c != nil {
		_ = c.Close()
	}
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("unexpected claim accepted")
	}
}

// No WorkerInfo, connector bootstrap, or permit participates in direct frames.
func TestDirectCDPMultipleFramesSingleSocket(t *testing.T) {
	f := newPinnedFixture(t)
	r, p := directStart(t, context.Background(), f)
	end, _ := r.lifetime.Deadline()
	if remaining := time.Until(end); remaining <= pinnedCDPSessionLimit || remaining > directCDPMaxLifetime {
		t.Fatal("wrong lifetime")
	}
	c := directAttach(t, r)
	for i := 0; i < 6; i++ {
		want := fmt.Sprintf("command %d", i)
		if c.WriteMessage(websocket.TextMessage, []byte(want)) != nil {
			t.Fatal("write")
		}
		kind, data, err := c.ReadMessage()
		if err != nil || kind != websocket.TextMessage || string(data) != want {
			t.Fatal("sequential frame failed")
		}
	}
	// Six request/response cycles already prove repeated bidirectional traffic
	// on the same upstream socket. Do not write the server fixture connection
	// from the test goroutine: its echo handler is the sole data-frame writer.
	pong := make(chan struct{}, 1)
	c.SetPongHandler(func(data string) error {
		if data == "ping" {
			pong <- struct{}{}
		}
		return nil
	})
	if c.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(time.Second)) != nil || c.WriteMessage(websocket.TextMessage, []byte("after ping")) != nil {
		t.Fatal("control write")
	}
	if _, _, err := c.ReadMessage(); err != nil {
		t.Fatal("control forwarding")
	}
	select {
	case <-pong:
	default:
		t.Fatal("missing pong")
	}
	directDenied(t, r.Endpoint(), nil)
	// Rejected second claim does not disrupt the existing dispatcher socket.
	if c.WriteMessage(websocket.TextMessage, []byte("still live")) != nil {
		t.Fatal("write")
	}
	if _, _, err := c.ReadMessage(); err != nil {
		t.Fatal("second claim disrupted first")
	}
	if !r.MatchesPeer(p) || r.MatchesPeer(pinnedTestPeer()) || r.Lost() || f.connections.Load() != 1 || f.requests.Load() != 1 {
		t.Fatal("identity/loss/redial")
	}
}

func TestDirectCDPAdmissionAndFailedHandshake(t *testing.T) {
	f := newPinnedFixture(t)
	r, _ := directStart(t, context.Background(), f)
	directDenied(t, r.Endpoint()+"-wrong", nil)
	directDenied(t, r.Endpoint(), http.Header{"Origin": []string{"http://127.0.0.1"}})
	directDenied(t, r.Endpoint(), http.Header{"Host": []string{"localhost"}})
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	// Non-upgrade probes do not consume the claim, just like pinned relay.
	resp, err := client.Get("http" + strings.TrimPrefix(r.Endpoint(), "ws"))
	if err != nil {
		t.Fatal("HTTP probe")
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal("non-upgrade accepted")
	}
	c := directAttach(t, r)
	_ = c.Close()
	pinnedWait(t, r.Done())
	if f.connections.Load() != 1 {
		t.Fatal("redial")
	}

	f2 := newPinnedFixture(t)
	r2, _ := directStart(t, context.Background(), f2)
	req, _ := http.NewRequest(http.MethodGet, "http"+strings.TrimPrefix(r2.Endpoint(), "ws"), nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	// Exact bearer + upgrade headers consumes claim even with an invalid WS key.
	resp, err = client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	pinnedWait(t, r2.Done())
	if !r2.Lost() {
		t.Fatal("failed handshake not terminal")
	}
	directDenied(t, r2.Endpoint(), nil)
	if f2.connections.Load() != 1 {
		t.Fatal("redial after failed handshake")
	}
}

func TestDirectCDPPermanentLoss(t *testing.T) {
	for _, mode := range []string{"downstream", "upstream", "cancel", "deadline", "pre-attach-upstream", "pre-attach-cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := newPinnedFixture(t)
			r, p := directStart(t, ctx, f)
			var c *websocket.Conn
			if !strings.HasPrefix(mode, "pre-attach") {
				c = directAttach(t, r)
			}
			up := <-f.accepted
			switch mode {
			case "downstream":
				_ = c.Close()
			case "upstream", "pre-attach-upstream":
				_ = up.Close()
			case "cancel", "pre-attach-cancel":
				cancel()
			case "deadline":
				// The lifetime context is immutable; its cancellation drives the same
				// shutdown path as its named maximum deadline without a 30m test wait.
				r.cancel()
			}
			pinnedWait(t, r.Done())
			if !r.Lost() || r.MatchesPeer(p) {
				t.Fatal("loss not latched")
			}
			if c != nil {
				if _, _, err := c.ReadMessage(); err == nil {
					t.Fatal("downstream retained")
				}
			}
			directDenied(t, r.Endpoint(), nil)
			if f.connections.Load() != 1 {
				t.Fatal("redial")
			}
		})
	}
}

func TestDirectCDPUnclaimedAttachTimeout(t *testing.T) {
	f := newPinnedFixture(t)
	r, _ := directStart(t, context.Background(), f)
	select {
	case <-r.Done():
	case <-time.After(directCDPAttachTimeout + 2*time.Second):
		t.Fatal("unclaimed direct relay did not expire")
	}
	if !r.Lost() || f.connections.Load() != 1 {
		t.Fatal("unclaimed relay did not fail closed without redial")
	}
	directDenied(t, r.Endpoint(), nil)
}

func TestDirectCDPConstructorRejectsBeforeHandshake(t *testing.T) {
	for _, mode := range []string{"mismatch", "nil-context", "canceled", "zero-peer", "aliased-peer", "nil-attestor", "noncanonical"} {
		t.Run(mode, func(t *testing.T) {
			l, source := edgeEvidenceServer(t)
			observed := pinnedTestPeer()
			expected := observed
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var arg context.Context = ctx
			attest := pinnedTestAttestor(observed)
			endpoint := source.endpoint
			switch mode {
			case "mismatch":
				expected = pinnedTestPeer()
			case "nil-context":
				arg = nil
			case "canceled":
				cancel()
			case "zero-peer":
				expected = externalPeerIdentity{}
			case "aliased-peer":
				expected.incarnation = expected.source
			case "nil-attestor":
				attest = nil
			case "noncanonical":
				endpoint = "ws://localhost:1/devtools/browser/fixture"
			}
			r, err := newDirectCDPRelay(arg, endpoint, expected, attest)
			if r != nil || err != errPinnedCDPUnavailable {
				if r != nil {
					r.Close()
				}
				t.Fatal("invalid constructor admitted/leaked cause")
			}
			if mode == "mismatch" {
				pinnedWait(t, l.closed)
				if l.dials.Load() != 1 {
					t.Fatal("dial count")
				}
			} else if l.dials.Load() != 0 {
				t.Fatal("invalid input dialed")
			}
			if l.bytes.Load() != 0 {
				t.Fatal("handshake bytes before attestation")
			}
		})
	}
}

func TestEdgeEvidenceDirectComposition(t *testing.T) {
	l, source := edgeEvidenceServer(t)
	probe := &edgeEvidenceProbe{listener: l, token: "501:Fri Oct 9 12:00:00 2026"}
	observed := pinnedTestPeer()
	attest := func(_ context.Context, c net.Conn) (externalPeerIdentity, error) {
		if probe.calls != "ctscts" || c.LocalAddr().String() != probe.server.RemoteAddr().String() {
			t.Error("evidence/attestation socket order")
		}
		return observed, nil
	}
	r, err := newEdgeEvidenceDirectRelay(context.Background(), source.endpoint, source, probe, observed, attest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); pinnedWait(t, r.Done()) })
	c := directAttach(t, r)
	for i := 0; i < 2; i++ {
		if c.WriteMessage(websocket.TextMessage, []byte("direct evidence")) != nil {
			t.Fatal("write")
		}
		if _, data, err := c.ReadMessage(); err != nil || string(data) != "direct evidence" {
			t.Fatal("echo")
		}
	}
	if l.dials.Load() != 1 {
		t.Fatal("redial")
	}
	for _, mode := range []string{"nil-probe", "nil-attestor", "endpoint"} {
		var p edgeSocketProbe = probe
		a := pinnedCDPAttestor(attest)
		endpoint := source.endpoint
		switch mode {
		case "nil-probe":
			p = nil
		case "nil-attestor":
			a = nil
		case "endpoint":
			endpoint += "-wrong"
		}
		if next, err := newEdgeEvidenceDirectRelay(context.Background(), endpoint, source, p, observed, a); next != nil || err != errPinnedCDPUnavailable {
			t.Fatal("invalid composition accepted")
		}
	}
}

func TestDirectCDPMessageLimit(t *testing.T) {
	for _, upstream := range []bool{false, true} {
		t.Run(fmt.Sprint(upstream), func(t *testing.T) {
			f := newPinnedFixture(t)
			r, _ := directStart(t, context.Background(), f)
			c := directAttach(t, r)
			from := c
			if upstream {
				from = <-f.accepted
			}
			_ = from.SetWriteDeadline(time.Now().Add(time.Second))
			_ = from.WriteMessage(websocket.BinaryMessage, make([]byte, pinnedCDPLimit+1))
			pinnedWait(t, r.Done())
			if !r.Lost() {
				t.Fatal("oversized frame retained")
			}
		})
	}
}
