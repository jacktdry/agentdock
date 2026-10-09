package browser

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uvwt/agentdock/internal/browserpolicy"
)

const pinnedCDPLimit = 8 << 20
const pinnedCDPTimeout = 2 * time.Second
const pinnedCDPSessionLimit = 5 * time.Minute

var errPinnedCDPUnavailable = errors.New("external pinned transport unavailable")

// TEST-ONLY injection until an independently reviewed OS socket-owner attestor
// exists. The input is the established TCP socket, before any WS handshake/data.
// It must independently observe identity, never echo a planner assertion.
type pinnedCDPAttestor func(context.Context, net.Conn) (externalPeerIdentity, error)

// A noncooperative injected attestor retains this single slot until it exits.
// Cancellation closes its socket; repeated attempts cannot accumulate workers.
var pinnedCDPAttestationSlot = make(chan struct{}, 1)

type pinnedCDPRelay struct {
	mu             sync.Mutex
	peer           externalPeerIdentity
	up             *websocket.Conn
	down           *websocket.Conn
	listener       *pinnedCDPListener
	server         *http.Server
	endpoint       string // Core-only bearer capability; never serialize or log.
	sourceEndpoint string // Immutable admitted upstream; Core-only.
	bound          bool   // One connector constructor reservation, never reset.
	path           string
	lost           bool
	claimed        bool
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	done           chan struct{}
}

func pinnedCDPEndpointValid(endpoint string) bool {
	if browserpolicy.ValidateCanonicalBrowserEndpoint(endpoint) != nil {
		return false
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "ws" || u.ForceQuery || u.RawQuery != "" || u.RawPath != "" ||
		!strings.HasPrefix(u.Path, "/devtools/browser/") || len(u.Path) == len("/devtools/browser/") {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.IsLoopback() && (ip.Equal(net.IPv4(127, 0, 0, 1)) || ip.Equal(net.IPv6loopback))
}

func attestPinnedCDP(ctx context.Context, c net.Conn, attest pinnedCDPAttestor, expected externalPeerIdentity) (ok bool) {
	select {
	case pinnedCDPAttestationSlot <- struct{}{}:
	default:
		return false
	}
	result := make(chan bool, 1)
	go func() {
		accepted := false
		defer func() {
			_ = recover()
			<-pinnedCDPAttestationSlot
			result <- accepted
		}()
		observed, err := attest(ctx, c)
		accepted = err == nil && observed == expected && ctx.Err() == nil
	}()
	select {
	case ok = <-result:
		return ok && ctx.Err() == nil
	case <-ctx.Done():
		return false
	}
}

// No production caller, discovery, reconnect, or status mint.
func newPinnedCDPRelay(ctx context.Context, endpoint string, expected externalPeerIdentity, attest pinnedCDPAttestor) (*pinnedCDPRelay, error) {
	if ctx == nil || ctx.Err() != nil || attest == nil || expected.source == nil || expected.incarnation == nil ||
		expected.source == expected.incarnation || !pinnedCDPEndpointValid(endpoint) {
		return nil, errPinnedCDPUnavailable
	}
	dialCtx, stopDial := context.WithTimeout(ctx, pinnedCDPTimeout)
	defer stopDial()
	var tcp net.Conn
	var stopSocket func() bool
	dialer := websocket.Dialer{Proxy: nil, HandshakeTimeout: pinnedCDPTimeout,
		NetDialContext: func(dc context.Context, network, addr string) (net.Conn, error) {
			if tcp != nil { // Even an unexpected dialer retry must fail closed.
				return nil, errPinnedCDPUnavailable
			}
			c, err := (&net.Dialer{}).DialContext(dc, network, addr)
			if err != nil {
				return nil, errPinnedCDPUnavailable
			}
			tcp = c
			stopSocket = context.AfterFunc(dialCtx, func() { _ = c.Close() })
			if !attestPinnedCDP(dialCtx, c, attest, expected) {
				_ = c.Close()
				return nil, errPinnedCDPUnavailable
			}
			return c, nil
		}}
	up, response, err := dialer.DialContext(dialCtx, endpoint, nil)
	if stopSocket != nil {
		stopSocket()
	}
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil || dialCtx.Err() != nil {
		if tcp != nil {
			_ = tcp.Close()
		}
		return nil, errPinnedCDPUnavailable // Includes redirects; no raw cause/URL.
	}
	var token [32]byte
	if _, err = rand.Read(token[:]); err != nil {
		_ = up.Close()
		return nil, errPinnedCDPUnavailable
	}
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		_ = up.Close()
		return nil, errPinnedCDPUnavailable
	}
	lifetime, cancel := context.WithTimeout(ctx, pinnedCDPSessionLimit)
	r := &pinnedCDPRelay{peer: expected, up: up, sourceEndpoint: endpoint, listener: &pinnedCDPListener{Listener: l, conns: make(map[*pinnedCDPConn]bool)},
		path: "/devtools/browser/" + hex.EncodeToString(token[:]), cancel: cancel, done: make(chan struct{})}
	r.endpoint = "ws://" + l.Addr().String() + r.path
	r.server = &http.Server{Handler: http.HandlerFunc(r.attach), ReadHeaderTimeout: pinnedCDPTimeout,
		ReadTimeout: pinnedCDPTimeout, WriteTimeout: pinnedCDPTimeout, IdleTimeout: pinnedCDPTimeout, MaxHeaderBytes: 4096,
		ErrorLog: log.New(io.Discard, "", 0)}
	r.configure(up)
	// Do not read upstream events before the single downstream connector joins.
	// A browser may emit unsolicited CDP events immediately after handshaking;
	// treating them as peer loss before attachment would reject a valid socket.
	// The kernel's TCP receive backpressure bounds this pre-attach phase.
	r.wg.Add(2)
	go func() { defer r.wg.Done(); _ = r.server.Serve(r.listener); r.Close() }()
	go func() { defer r.wg.Done(); <-lifetime.Done(); r.Close() }()
	go func() { r.wg.Wait(); close(r.done) }()
	return r, nil
}

func (r *pinnedCDPRelay) configure(c *websocket.Conn) {
	c.SetReadLimit(pinnedCDPLimit)
	_ = c.SetReadDeadline(time.Now().Add(pinnedCDPSessionLimit))
	// Control frames are forwarded with bounded gorilla control writes. Close
	// frames trigger permanent loss through ReadMessage, then close both sockets.
	c.SetPingHandler(func(data string) error { return r.control(c, websocket.PingMessage, data) })
	c.SetPongHandler(func(data string) error { return r.control(c, websocket.PongMessage, data) })
	c.SetCloseHandler(func(code int, text string) error {
		_ = r.control(c, websocket.CloseMessage, string(websocket.FormatCloseMessage(code, text)))
		return &websocket.CloseError{Code: code, Text: text}
	})
}

func (r *pinnedCDPRelay) control(from *websocket.Conn, kind int, data string) error {
	r.mu.Lock()
	to := r.up
	if from == r.up {
		to = r.down
	}
	lost := r.lost
	r.mu.Unlock()
	if lost || to == nil {
		return errPinnedCDPUnavailable
	}
	return to.WriteControl(kind, []byte(data), time.Now().Add(pinnedCDPTimeout))
}

func (r *pinnedCDPRelay) attach(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	if r.lost {
		r.mu.Unlock()
		return
	}
	r.wg.Add(1)
	r.mu.Unlock()
	defer r.wg.Done()
	if req.Method != http.MethodGet || req.Host != r.listener.Addr().String() || req.Header.Get("Origin") != "" ||
		subtle.ConstantTimeCompare([]byte(req.RequestURI), []byte(r.path)) != 1 || !websocket.IsWebSocketUpgrade(req) {
		http.Error(w, "unavailable", http.StatusNotFound)
		return
	}
	r.mu.Lock()
	if r.lost || r.claimed {
		r.mu.Unlock()
		http.Error(w, "unavailable", http.StatusNotFound)
		return
	}
	r.claimed = true // One authorized attempt; a failed handshake is terminal.
	r.mu.Unlock()
	upgrader := websocket.Upgrader{HandshakeTimeout: pinnedCDPTimeout, Error: func(w http.ResponseWriter, _ *http.Request, status int, _ error) {
		http.Error(w, "unavailable", status)
	}}
	down, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		r.Close()
		return
	}
	r.configure(down)
	r.mu.Lock()
	if r.lost {
		r.mu.Unlock()
		_ = down.Close()
		return
	}
	r.down = down
	// Both pumps start only after a successful one-shot downstream upgrade.
	// Serve and this handler still hold WaitGroup slots during Add.
	r.wg.Add(2)
	r.mu.Unlock()
	go func() { defer r.wg.Done(); r.forward(r.up, true) }()
	go func() { defer r.wg.Done(); r.forward(down, false) }()
}

func (r *pinnedCDPRelay) forward(from *websocket.Conn, upstream bool) {
	defer r.Close()
	for {
		kind, data, err := from.ReadMessage()
		if err != nil {
			return
		}
		r.mu.Lock()
		to := r.up
		if upstream {
			to = r.down
		}
		lost := r.lost
		r.mu.Unlock()
		// Unsolicited upstream data before attachment is terminal, never buffered.
		if lost || to == nil {
			return
		}
		if to.SetWriteDeadline(time.Now().Add(pinnedCDPTimeout)) != nil || to.WriteMessage(kind, data) != nil {
			return
		}
	}
}

// Admission check on this immutable socket only. Callback gets no socket or
// reconnect capability. Remote loss may remain undetected until I/O; Close can
// race callback execution. This is NOT atomic CDP command/worker fencing.
func (r *pinnedCDPRelay) withPinnedPeer(expected externalPeerIdentity, callback func() error) error {
	if r == nil || callback == nil {
		return errPinnedCDPUnavailable
	}
	r.mu.Lock()
	ok := !r.lost && r.peer == expected
	r.mu.Unlock()
	if !ok {
		return errPinnedCDPUnavailable
	}
	err := callback()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lost || err != nil {
		return errPinnedCDPUnavailable
	}
	return nil
}

func (r *pinnedCDPRelay) Close() {
	r.mu.Lock()
	if r.lost {
		r.mu.Unlock()
		return
	}
	r.lost = true
	down := r.down
	r.mu.Unlock()
	r.cancel()
	_ = r.up.Close()
	if down != nil {
		_ = down.Close()
	}
	_ = r.listener.Close()
	_ = r.server.Close()
}

// Bound simultaneous HTTP probes, including connections hijacked during an
// upgrade racing Close. No request logs or token-bearing error responses.
type pinnedCDPListener struct {
	net.Listener
	mu     sync.Mutex
	closed bool
	conns  map[*pinnedCDPConn]bool
}

type pinnedCDPConn struct {
	net.Conn
	owner *pinnedCDPListener
}

func (l *pinnedCDPListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		l.mu.Lock()
		if l.closed || len(l.conns) >= 4 {
			l.mu.Unlock()
			_ = c.Close()
			continue
		}
		wrapped := &pinnedCDPConn{Conn: c, owner: l}
		l.conns[wrapped] = true
		l.mu.Unlock()
		return wrapped, nil
	}
}

func (l *pinnedCDPListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	err := l.Listener.Close()
	for c := range l.conns {
		_ = c.Conn.Close()
	}
	return err
}

func (c *pinnedCDPConn) Close() error {
	err := c.Conn.Close()
	c.owner.mu.Lock()
	delete(c.owner.conns, c)
	c.owner.mu.Unlock()
	return err
}
