package browser

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func pinnedTestPeer() externalPeerIdentity {
	return externalPeerIdentity{source: &connectorEvidenceIdentity{}, incarnation: &connectorEvidenceIdentity{}}
}

// Captured fixture identity is independent of the expected argument at the
// constructor boundary. It is synthetic; this is NOT an OS owner attestor.
func pinnedTestAttestor(peer externalPeerIdentity) pinnedCDPAttestor {
	return func(_ context.Context, c net.Conn) (externalPeerIdentity, error) {
		if _, ok := c.LocalAddr().(*net.TCPAddr); !ok {
			return externalPeerIdentity{}, errors.New("not TCP")
		}
		return peer, nil
	}
}

type pinnedFixture struct {
	endpoint    string
	accepted    chan *websocket.Conn
	requests    atomic.Int32
	connections atomic.Int32
}

func newPinnedFixture(t *testing.T) *pinnedFixture {
	t.Helper()
	f := &pinnedFixture{accepted: make(chan *websocket.Conn, 1)}
	var wg sync.WaitGroup
	var wgMu sync.Mutex
	closing := false
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Add is serialized against cleanup's terminal Wait. Otherwise a
		// late accepted HTTP handler can increment the WaitGroup after Wait.
		wgMu.Lock()
		if closing {
			wgMu.Unlock()
			_ = c.Close()
			return
		}
		wg.Add(1)
		wgMu.Unlock()
		defer wg.Done()
		defer c.Close()
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		f.accepted <- c
		for {
			kind, data, err := c.ReadMessage()
			if err != nil || c.WriteMessage(kind, data) != nil {
				return
			}
		}
	}))
	s.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			f.connections.Add(1)
		}
	}
	s.Start()
	f.endpoint = "ws" + strings.TrimPrefix(s.URL, "http") + "/devtools/browser/fixture"
	t.Cleanup(func() {
		wgMu.Lock()
		closing = true
		wgMu.Unlock()
		s.Close()
		wg.Wait()
	})
	return f
}

func pinnedWait(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("relay workers did not exit")
	}
}

func pinnedStart(t *testing.T, ctx context.Context, f *pinnedFixture) (*pinnedCDPRelay, externalPeerIdentity) {
	t.Helper()
	p := pinnedTestPeer()
	r, err := newPinnedCDPRelay(ctx, f.endpoint, p, pinnedTestAttestor(p))
	if err != nil {
		t.Fatalf("synthetic relay: %v", err)
	}
	t.Cleanup(func() { r.Close(); pinnedWait(t, r.done) })
	return r, p
}

func pinnedAttach(t *testing.T, r *pinnedCDPRelay) *websocket.Conn {
	t.Helper()
	c, _, err := (&websocket.Dialer{HandshakeTimeout: time.Second}).Dial(r.endpoint, nil)
	if err != nil {
		t.Fatal("synthetic downstream attach failed")
	}
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestPinnedCDPEchoAndAdmission(t *testing.T) {
	f := newPinnedFixture(t)
	r, peer := pinnedStart(t, context.Background(), f)
	c := pinnedAttach(t, r)
	for _, kind := range []int{websocket.TextMessage, websocket.BinaryMessage} {
		want := []byte("synthetic CDP")
		if c.WriteMessage(kind, want) != nil {
			t.Fatal("write failed")
		}
		gotKind, got, err := c.ReadMessage()
		if err != nil || gotKind != kind || !bytes.Equal(got, want) {
			t.Fatal("frame was not echoed")
		}
	}
	called := 0
	if r.withPinnedPeer(peer, func() error { called++; return nil }) != nil || called != 1 {
		t.Fatal("live pinned peer denied")
	}
	if r.withPinnedPeer(pinnedTestPeer(), func() error { called++; return nil }) != errPinnedCDPUnavailable || called != 1 {
		t.Fatal("foreign peer callback executed")
	}
	second, response, err := (&websocket.Dialer{HandshakeTimeout: time.Second}).Dial(r.endpoint, nil)
	if second != nil {
		second.Close()
	}
	if response != nil {
		response.Body.Close()
	}
	if err == nil || f.requests.Load() != 1 {
		t.Fatal("second session admitted or upstream redialed")
	}
	// Ping is forwarded upstream and its pong travels back through the relay.
	pong := make(chan struct{}, 1)
	c.SetPongHandler(func(data string) error {
		if data == "fixture" {
			pong <- struct{}{}
		}
		return nil
	})
	if c.WriteControl(websocket.PingMessage, []byte("fixture"), time.Now().Add(time.Second)) != nil ||
		c.WriteMessage(websocket.TextMessage, []byte("after ping")) != nil {
		t.Fatal("control write failed")
	}
	if _, _, err := c.ReadMessage(); err != nil {
		t.Fatal("control forwarding failed")
	}
	select {
	case <-pong:
	default:
		t.Fatal("forwarded pong missing")
	}
}

// A real CDP browser may emit an event immediately after the WS handshake,
// before the MCP connector has established its downstream socket. That event
// must be delivered after attach without closing or replacing the upstream.
func TestPinnedCDPEventBeforeDownstreamAttach(t *testing.T) {
	sent := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, req, nil)
		if err != nil {
			return
		}
		defer c.Close()
		if c.WriteMessage(websocket.TextMessage, []byte(`{"method":"Target.targetCreated"}`)) != nil {
			return
		}
		close(sent)
		for {
			kind, payload, readErr := c.ReadMessage()
			if readErr != nil || c.WriteMessage(kind, payload) != nil {
				return
			}
		}
	}))
	defer s.Close()
	peer := pinnedTestPeer()
	endpoint := "ws" + strings.TrimPrefix(s.URL, "http") + "/devtools/browser/fixture"
	relay, err := newPinnedCDPRelay(context.Background(), endpoint, peer, pinnedTestAttestor(peer))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { relay.Close(); pinnedWait(t, relay.done) }()
	select {
	case <-sent:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never emitted early CDP event")
	}
	down := pinnedAttach(t, relay)
	kind, event, err := down.ReadMessage()
	if err != nil || kind != websocket.TextMessage || string(event) != `{"method":"Target.targetCreated"}` {
		t.Fatalf("early event lost or relay closed: kind=%d event=%s err=%v", kind, event, err)
	}
	if err := down.WriteMessage(websocket.TextMessage, []byte(`{"id":1,"method":"Browser.getVersion"}`)); err != nil {
		t.Fatal(err)
	}
	kind, response, err := down.ReadMessage()
	if err != nil || kind != websocket.TextMessage || string(response) != `{"id":1,"method":"Browser.getVersion"}` {
		t.Fatal("early CDP event broke the pinned upstream socket", err)
	}
}

func TestPinnedCDPInvalidInputNoConnect(t *testing.T) {
	f := newPinnedFixture(t)
	p := pinnedTestPeer()
	for _, endpoint := range []string{"", "ws://localhost:1234/devtools/browser/x", "ws://127.0.0.1:1234/devtools/page/x",
		"ws://127.0.0.1:1234/devtools/browser", f.endpoint + "?secret", f.endpoint + "?", f.endpoint + "#secret",
		"wss://127.0.0.1:1234/devtools/browser/x", "ws://127.0.0.2:1234/devtools/browser/x", "ws://192.0.2.1:1234/devtools/browser/x", "ws://127.0.0.1:0/devtools/browser/x"} {
		r, err := newPinnedCDPRelay(context.Background(), endpoint, p, pinnedTestAttestor(p))
		if r != nil || err != errPinnedCDPUnavailable {
			t.Fatal("invalid endpoint accepted")
		}
	}
	for _, peer := range []externalPeerIdentity{{}, {source: p.source}, {source: p.source, incarnation: p.source}} {
		if r, err := newPinnedCDPRelay(context.Background(), f.endpoint, peer, pinnedTestAttestor(p)); r != nil || err != errPinnedCDPUnavailable {
			t.Fatal("invalid identity accepted")
		}
	}
	if r, err := newPinnedCDPRelay(context.Background(), f.endpoint, p, nil); r != nil || err != errPinnedCDPUnavailable {
		t.Fatal("nil attestor accepted")
	}
	if !pinnedCDPEndpointValid("ws://[::1]:1234/devtools/browser/ipv6") {
		t.Fatal("IPv6 loopback rejected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r, err := newPinnedCDPRelay(ctx, f.endpoint, p, pinnedTestAttestor(p)); r != nil || err != errPinnedCDPUnavailable {
		t.Fatal("cancelled constructor accepted")
	}
	if f.requests.Load() != 0 || f.connections.Load() != 0 {
		t.Fatal("invalid input reached WS handshake")
	}
}

func TestPinnedCDPAttestorExactSocketZeroData(t *testing.T) {
	for _, mode := range []string{"mismatch", "error", "panic", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			l, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			data := make(chan []byte, 1)
			go func() {
				c, err := l.Accept()
				if err != nil {
					data <- nil
					return
				}
				defer c.Close()
				_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
				b, _ := io.ReadAll(c)
				data <- b
			}()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			observed := false
			attest := func(_ context.Context, c net.Conn) (externalPeerIdentity, error) {
				observed = c.RemoteAddr().String() == l.Addr().String()
				if _, ok := c.LocalAddr().(*net.TCPAddr); !ok {
					return externalPeerIdentity{}, errors.New("bad addr")
				}
				switch mode {
				case "error":
					return externalPeerIdentity{}, errors.New("secret canary")
				case "panic":
					panic("secret canary")
				case "cancel":
					cancel()
				}
				return pinnedTestPeer(), nil
			}
			r, err := newPinnedCDPRelay(ctx, "ws://"+l.Addr().String()+"/devtools/browser/fixture", pinnedTestPeer(), attest)
			if r != nil || err != errPinnedCDPUnavailable || !observed {
				t.Fatal("verification did not fail closed on actual socket")
			}
			select {
			case b := <-data:
				if len(b) != 0 {
					t.Fatal("handshake/data sent before peer verification")
				}
			case <-time.After(4 * time.Second):
				t.Fatal("rejected socket leaked")
			}
		})
	}
}

func TestPinnedCDPWrongTokenAndProbes(t *testing.T) {
	r, _ := pinnedStart(t, context.Background(), newPinnedFixture(t))
	for _, endpoint := range []string{r.endpoint + "wrong", r.endpoint + "?query", strings.Replace(r.endpoint, "/devtools/browser/", "/devtools/page/", 1)} {
		c, resp, err := (&websocket.Dialer{HandshakeTimeout: time.Second}).Dial(endpoint, nil)
		if c != nil {
			c.Close()
		}
		if resp != nil {
			resp.Body.Close()
			if resp.StatusCode != 404 {
				t.Fatal("wrong route response")
			}
		}
		if err == nil {
			t.Fatal("unauthorized WS admitted")
		}
	}
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	resp, err := client.Get("http" + strings.TrimPrefix(r.endpoint, "ws"))
	if err != nil {
		t.Fatal("probe request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("HTTP probe admitted")
	}
	pinnedAttach(t, r) // Rejected probes did not consume the single session.
}

func TestPinnedCDPLossIrreversible(t *testing.T) {
	for _, mode := range []string{"downstream", "upstream", "cancel", "concurrent-close"} {
		t.Run(mode, func(t *testing.T) {
			f := newPinnedFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r, p := pinnedStart(t, ctx, f)
			c := pinnedAttach(t, r)
			up := <-f.accepted
			switch mode {
			case "downstream":
				c.Close()
			case "upstream":
				up.Close()
			case "cancel":
				cancel()
			case "concurrent-close":
				var wg sync.WaitGroup
				for i := 0; i < 16; i++ {
					wg.Add(1)
					go func() { defer wg.Done(); r.Close() }()
				}
				wg.Wait()
			}
			pinnedWait(t, r.done)
			if r.withPinnedPeer(p, func() error { t.Error("lost callback executed"); return nil }) != errPinnedCDPUnavailable {
				t.Fatal("lost fence succeeded")
			}
			_ = c.SetReadDeadline(time.Now().Add(time.Second))
			if _, _, err := c.ReadMessage(); err == nil {
				t.Fatal("other socket remained open")
			}
			next, resp, err := (&websocket.Dialer{HandshakeTimeout: time.Second}).Dial(r.endpoint, nil)
			if next != nil {
				next.Close()
			}
			if resp != nil {
				resp.Body.Close()
			}
			if err == nil || f.requests.Load() != 1 {
				t.Fatal("lost relay reattached/redialed")
			}
		})
	}
}

func TestPinnedCDPRedirectAndProxy(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer s.Close()
	t.Setenv("HTTP_PROXY", target.URL)
	t.Setenv("HTTPS_PROXY", target.URL)
	t.Setenv("ALL_PROXY", target.URL)
	p := pinnedTestPeer()
	r, err := newPinnedCDPRelay(context.Background(), "ws"+strings.TrimPrefix(s.URL, "http")+"/devtools/browser/fixture", p, pinnedTestAttestor(p))
	if r != nil || err != errPinnedCDPUnavailable || redirected.Load() != 0 {
		t.Fatal("redirect/proxy followed")
	}
}

func TestPinnedCDPWorkerLeakBound(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 8; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			f := newPinnedFixture(t)
			r, _ := pinnedStart(t, context.Background(), f)
			c := pinnedAttach(t, r)
			c.Close()
			r.Close()
			pinnedWait(t, r.done)
		}) // Close each synthetic HTTP fixture before measuring worker growth.
	}
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before+4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if runtime.NumGoroutine() > before+4 {
		t.Fatal("relay workers accumulated")
	}
}

func TestPinnedCDPMessageLimit(t *testing.T) {
	for _, upstream := range []bool{false, true} {
		t.Run(strconv.FormatBool(upstream), func(t *testing.T) {
			f := newPinnedFixture(t)
			r, p := pinnedStart(t, context.Background(), f)
			c := pinnedAttach(t, r)
			from := c
			if upstream {
				from = <-f.accepted
			}
			_ = from.SetWriteDeadline(time.Now().Add(time.Second))
			_ = from.WriteMessage(websocket.BinaryMessage, make([]byte, pinnedCDPLimit+1))
			pinnedWait(t, r.done)
			if r.withPinnedPeer(p, func() error { t.Error("oversized session still live"); return nil }) != errPinnedCDPUnavailable {
				t.Fatal("message limit did not latch loss")
			}
		})
	}
}

func TestPinnedCDPCancelBeforeAttach(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r, _ := pinnedStart(t, ctx, newPinnedFixture(t))
	cancel()
	pinnedWait(t, r.done)
}

func TestPinnedCDPBlockedAttestorBound(t *testing.T) {
	f := newPinnedFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer func() {
		close(release)
		deadline := time.Now().Add(time.Second)
		for len(pinnedCDPAttestationSlot) != 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if len(pinnedCDPAttestationSlot) != 0 {
			t.Error("attestor slot not released")
		}
	}()
	p := pinnedTestPeer()
	go func() {
		defer close(returned)
		r, err := newPinnedCDPRelay(ctx, f.endpoint, p, func(_ context.Context, c net.Conn) (externalPeerIdentity, error) {
			close(entered)
			<-release // Deliberately uncooperative TEST ONLY callback.
			return p, nil
		})
		if r != nil || err != errPinnedCDPUnavailable {
			t.Error("cancelled verifier admitted")
		}
	}()
	pinnedWait(t, entered)
	cancel()
	pinnedWait(t, returned)
	var calls atomic.Int32
	if r, err := newPinnedCDPRelay(context.Background(), f.endpoint, p, func(context.Context, net.Conn) (externalPeerIdentity, error) {
		calls.Add(1)
		return p, nil
	}); r != nil || err != errPinnedCDPUnavailable || calls.Load() != 0 {
		t.Fatal("blocked attestor accumulated another worker")
	}
}
