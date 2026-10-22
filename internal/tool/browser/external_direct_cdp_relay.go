package browser

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uvwt/agentdock/internal/browserpolicy"
)

// PRIVATE transport identity foundation for a future trusted Core direct
// dispatcher. Semantic intent belongs to that dispatcher, not CDP frames.
// This does not prove browser login, consent, or no-focus; Edge is UNQUALIFIED.
const (
	directCDPAttachTimeout    = 5 * time.Second
	directCDPHandshakeTimeout = 15 * time.Second
	directCDPMaxLifetime      = 30 * time.Minute
)

type directCDPRelay struct {
	mu             sync.Mutex
	writeMu        sync.Mutex
	peer           externalPeerIdentity
	up, down       *websocket.Conn
	listener       *pinnedCDPListener
	server         *http.Server
	endpoint, path string // Core-only bearer capability; never serialize/log.
	lost, claimed  bool
	cancel         context.CancelFunc
	lifetime       context.Context
	attached       chan struct{}
	done           chan struct{}
	wg             sync.WaitGroup
	closeOnce      sync.Once
}

// No production caller, discovery, reconnect, or status mint.
func newDirectCDPRelay(ctx context.Context, endpoint string, expected externalPeerIdentity, attest pinnedCDPAttestor) (*directCDPRelay, error) {
	if ctx == nil || ctx.Err() != nil || attest == nil || expected.source == nil || expected.incarnation == nil ||
		expected.source == expected.incarnation || !directCDPEndpointValid(endpoint) {
		return nil, errPinnedCDPUnavailable
	}
	dialCtx, stopDial := context.WithTimeout(ctx, directCDPHandshakeTimeout)
	defer stopDial()
	var tcp net.Conn
	var attempted bool
	var stopSocket func() bool
	dialer := websocket.Dialer{Proxy: nil, HandshakeTimeout: directCDPHandshakeTimeout,
		NetDialContext: func(dc context.Context, network, addr string) (net.Conn, error) {
			if attempted { // Count attempts, including failed TCP dials.
				return nil, errPinnedCDPUnavailable
			}
			attempted = true
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
	lifetime, cancel := context.WithTimeout(ctx, directCDPMaxLifetime)
	r := &directCDPRelay{peer: expected, up: up, listener: &pinnedCDPListener{Listener: l, conns: make(map[*pinnedCDPConn]bool)},
		path: "/devtools/browser/" + hex.EncodeToString(token[:]), cancel: cancel, lifetime: lifetime, done: make(chan struct{}), attached: make(chan struct{})}
	r.endpoint = "ws://" + l.Addr().String() + r.path
	r.server = &http.Server{Handler: http.HandlerFunc(r.attach), ReadHeaderTimeout: pinnedCDPTimeout,
		ReadTimeout: pinnedCDPTimeout, WriteTimeout: pinnedCDPTimeout, IdleTimeout: pinnedCDPTimeout, MaxHeaderBytes: 4096,
		ErrorLog: log.New(io.Discard, "", 0)}
	r.configure(up)
	// Read immediately to detect pre-attach loss. At most one bounded frame
	// waits for the downstream; TCP backpressure bounds further events.
	r.wg.Add(3)
	go func() { defer r.wg.Done(); _ = r.server.Serve(r.listener); r.Close() }()
	go func() {
		defer r.wg.Done()
		timer := time.NewTimer(directCDPAttachTimeout)
		defer timer.Stop()
		select {
		case <-r.attached:
			<-lifetime.Done()
			r.Close()
		case <-timer.C:
			r.Close()
		case <-lifetime.Done():
			r.Close()
		}
	}()
	go func() { defer r.wg.Done(); r.forward(up, true) }()
	go func() { r.wg.Wait(); close(r.done) }()
	return r, nil
}

// Edge's existing supported socket can use /devtools/browser without a
// browser-ID suffix. This canonical form is allowed only by the direct
// dispatcher; the legacy single-use MCP relay retains its stricter grammar.
// Either form still requires pre-handshake attestation of the concrete socket.
func directCDPEndpointValid(endpoint string) bool {
	if browserpolicy.ValidateCanonicalBrowserEndpoint(endpoint) != nil {
		return false
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "ws" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && (ip.Equal(net.IPv4(127, 0, 0, 1)) || ip.Equal(net.IPv6loopback))
}

func (r *directCDPRelay) configure(c *websocket.Conn) {
	c.SetReadLimit(pinnedCDPLimit)
	deadline, _ := r.lifetime.Deadline()
	_ = c.SetReadDeadline(deadline)
	// Control frames are forwarded with bounded gorilla control writes. Close
	// frames trigger permanent loss through ReadMessage, then close both sockets.
	c.SetPingHandler(func(data string) error { return r.control(c, websocket.PingMessage, data) })
	c.SetPongHandler(func(data string) error { return r.control(c, websocket.PongMessage, data) })
	c.SetCloseHandler(func(code int, text string) error {
		select {
		case <-r.attached:
			_ = r.control(c, websocket.CloseMessage, string(websocket.FormatCloseMessage(code, text)))
		default: // A pre-attach close must not wait for a downstream claim.
		}
		return &websocket.CloseError{Code: code, Text: text}
	})
}

func (r *directCDPRelay) control(from *websocket.Conn, kind int, data string) error {
	return r.writeFrame(from == r.up, kind, []byte(data), true)
}

// Wait for the sole downstream without buffering unbounded browser events.
func (r *directCDPRelay) writeFrame(upstream bool, kind int, data []byte, control bool) error {
	if upstream {
		select {
		case <-r.attached:
		case <-r.lifetime.Done():
			r.Close()
			return errPinnedCDPUnavailable
		}
	}
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.Lock()
	to := r.up
	if upstream {
		to = r.down
	}
	allowed := !r.lost && r.lifetime.Err() == nil && to != nil
	r.mu.Unlock()
	deadline := time.Now().Add(pinnedCDPTimeout)
	if end, _ := r.lifetime.Deadline(); end.Before(deadline) {
		deadline = end
	}
	var err error
	if !allowed {
		err = errPinnedCDPUnavailable
	} else if control {
		err = to.WriteControl(kind, data, deadline)
	} else if err = to.SetWriteDeadline(deadline); err == nil {
		err = to.WriteMessage(kind, data)
	}
	if err != nil {
		r.Close()
		return errPinnedCDPUnavailable
	}
	return nil
}

func (r *directCDPRelay) attach(w http.ResponseWriter, req *http.Request) {
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
	// This handler holds a WaitGroup slot during Add.
	r.wg.Add(1)
	close(r.attached)
	r.mu.Unlock()
	go func() { defer r.wg.Done(); r.forward(down, false) }()
}

func (r *directCDPRelay) forward(from *websocket.Conn, upstream bool) {
	defer r.Close()
	for {
		kind, data, err := from.ReadMessage()
		if err != nil {
			return
		}
		if r.writeFrame(upstream, kind, data, false) != nil {
			return
		}
	}
}

func (r *directCDPRelay) Close() {
	r.mu.Lock()
	r.lost = true
	down := r.down
	r.mu.Unlock()
	r.closeOnce.Do(func() {
		r.cancel()
		_ = r.up.Close()
		if down != nil {
			_ = down.Close()
		}
		_ = r.listener.Close()
		_ = r.server.Close()
	})
}

func (r *directCDPRelay) Endpoint() string      { return r.endpoint }
func (r *directCDPRelay) Done() <-chan struct{} { return r.done }
func (r *directCDPRelay) Lost() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lost || r.lifetime.Err() != nil
}
func (r *directCDPRelay) MatchesPeer(peer externalPeerIdentity) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.lost && r.lifetime.Err() == nil && r.peer == peer
}

// B2k evidence and an independent attestor observe the same established TCP
// socket before handshake. Evidence remains non-atomic and grants no route,
// authenticated status or live consent. No production caller exists.
func newEdgeEvidenceDirectRelay(ctx context.Context, endpoint string, source edgeProcessPreflight, probe edgeSocketProbe, expected externalPeerIdentity, independentAttestor pinnedCDPAttestor) (*directCDPRelay, error) {
	if probe == nil || independentAttestor == nil || endpoint != source.endpoint {
		return nil, errPinnedCDPUnavailable
	}
	return newDirectCDPRelay(ctx, endpoint, expected, edgeEvidencePinnedAttestor(source, probe, independentAttestor))
}
