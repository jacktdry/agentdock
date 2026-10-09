package browser

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

type pinnedConnectorSession struct {
	*fakeWorkerSession
	conn      *websocket.Conn
	toolsHook func()
	closeErr  error
}

func (s *pinnedConnectorSession) Tools() map[string]mcpclient.Tool {
	if s.toolsHook != nil {
		s.toolsHook()
	}
	return s.fakeWorkerSession.Tools()
}
func (s *pinnedConnectorSession) Close() error {
	if s.conn != nil {
		_ = s.conn.Close()
	}
	_ = s.fakeWorkerSession.Close()
	return s.closeErr
}

func pinnedConnectorOptions(f *pinnedFixture) ExternalWorkerOptions {
	d := externalRoute(externalStart())
	d.Start.Endpoint = f.endpoint
	d.grant = &externalRouteGrant{}
	return ExternalWorkerOptions{Cwd: "/synthetic", Route: d}
}

func pinnedConnectorRegistry(t *testing.T, relay *pinnedCDPRelay, options ExternalWorkerOptions, s *pinnedConnectorSession, hook func()) *WorkerRegistry {
	t.Helper()
	want := externalWorkerConfig(options)
	want.Args[2] = "--wsEndpoint=" + relay.endpoint
	check := func(cfg mcpclient.ServerConfig) {
		t.Helper()
		count := 0
		for _, arg := range cfg.Args {
			if strings.HasPrefix(arg, "--wsEndpoint=") {
				count++
			}
			if strings.Contains(arg, options.Route.Start.Endpoint) {
				t.Fatal("source endpoint escaped into connector args")
			}
		}
		if count != 1 || !reflect.DeepEqual(cfg, want) {
			t.Fatal("connector config changed beyond pinned endpoint")
		}
	}
	return NewWorkerRegistry(WorkerDependencies{
		Probe: func(_ context.Context, cfg mcpclient.ServerConfig) (string, error) {
			check(cfg)
			return ManagedEngineVersion, nil
		},
		Open: func(_ context.Context, cfg mcpclient.ServerConfig) (WorkerSession, error) {
			check(cfg)
			s.conn = pinnedAttach(t, relay)
			// Synthetic data traverses the actual relay and established upstream.
			if s.conn.WriteMessage(websocket.TextMessage, []byte("synthetic handshake")) != nil {
				t.Fatal("handshake write")
			}
			_, got, err := s.conn.ReadMessage()
			if err != nil || string(got) != "synthetic handshake" {
				t.Fatal("handshake echo")
			}
			if hook != nil {
				hook()
			}
			return s, nil
		},
	})
}

func TestPinnedConnectorBindingAndStop(t *testing.T) {
	for _, mode := range []string{"concurrent", "registry stop", "shutdown", "stop error"} {
		t.Run(mode, func(t *testing.T) {
			f := newPinnedFixture(t)
			relay, peer := pinnedStart(t, context.Background(), f)
			options := pinnedConnectorOptions(f)
			original := options
			s := &pinnedConnectorSession{fakeWorkerSession: fakeManagedSession(t, "connector")}
			if mode == "stop error" {
				s.closeErr = errors.New("private endpoint/token/PID cleanup canary")
			}
			r := pinnedConnectorRegistry(t, relay, options, s, nil)
			c, err := newPinnedExternalConnector(context.Background(), relay, peer, options, r)
			if err != nil || c == nil || c.checkAlive(context.Background()) != nil {
				t.Fatal("binding failed")
			}
			if options != original || options.Route.grant.used.Load() {
				t.Fatal("route/grant mutated")
			}
			before := c.worker
			if f.connections.Load() != 1 || f.requests.Load() != 1 {
				t.Fatal("upstream was redialed")
			}
			// Reuse never launches another connector, including before Stop.
			if other, err := newPinnedExternalConnector(context.Background(), relay, peer, options, r); other != nil || err != errPinnedConnectorUnavailable {
				t.Fatal("binding reused")
			}
			switch mode {
			case "registry stop":
				if r.Stop(c.worker.WorkerID) != nil {
					t.Fatal("registry stop")
				}
			case "shutdown":
				if r.Shutdown(context.Background()) != nil {
					t.Fatal("shutdown")
				}
			default:
				var wg sync.WaitGroup
				for i := 0; i < 8; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						err := c.Stop()
						if (mode == "stop error") != (err == errPinnedConnectorUnavailable) {
							t.Error("stop result")
						}
					}()
				}
				wg.Wait()
			}
			pinnedWait(t, relay.done)
			if c.worker != before || c.checkAlive(context.Background()) != errPinnedConnectorUnavailable || s.closes != 1 {
				t.Fatal("generation mutated or connector leaked")
			}
			if other, err := newPinnedExternalConnector(context.Background(), relay, peer, options, r); other != nil || err != errPinnedConnectorUnavailable {
				t.Fatal("stopped binding reused")
			}
			// Only the connector and relay sockets close; the fixture listener lives.
			<-f.accepted
			up, _, err := (&websocket.Dialer{HandshakeTimeout: time.Second}).Dial(f.endpoint, nil)
			if err != nil {
				t.Fatal("user browser fixture was closed")
			}
			_ = up.Close()
		})
	}
}

func TestPinnedConnectorRejectBeforeWorker(t *testing.T) {
	for _, mode := range []string{"nil relay", "nil ctx", "canceled", "nil registry", "zero registry", "closed registry", "invalid route", "invalid endpoint", "other source path", "nil peer", "aliased peer", "other peer", "lost", "claimed", "relay path", "relay endpoint"} {
		t.Run(mode, func(t *testing.T) {
			f := newPinnedFixture(t)
			relay, peer := pinnedStart(t, context.Background(), f)
			options := pinnedConnectorOptions(f)
			calls := 0
			r := NewWorkerRegistry(WorkerDependencies{Probe: func(context.Context, mcpclient.ServerConfig) (string, error) {
				calls++
				return "", errors.New("unexpected")
			}, Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) {
				calls++
				return nil, errors.New("unexpected")
			}})
			ctx := context.Background()
			target := relay
			switch mode {
			case "nil relay":
				target = nil
			case "nil ctx":
				ctx = nil
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "nil registry":
				r = nil
			case "zero registry":
				r = &WorkerRegistry{}
			case "closed registry":
				_ = r.Shutdown(ctx)
			case "invalid route":
				options.Route = RouteDecision{}
			case "invalid endpoint":
				options.Route.Start.Endpoint = "ws://localhost:9222/devtools/browser/private"
			case "other source path":
				options.Route.Start.Endpoint += "other"
			case "nil peer":
				peer = externalPeerIdentity{}
			case "aliased peer":
				peer.incarnation = peer.source
			case "other peer":
				peer = pinnedTestPeer()
			case "lost":
				relay.Close()
			case "claimed":
				_ = pinnedAttach(t, relay)
			case "relay path":
				relay.mu.Lock()
				relay.path += "/"
				relay.mu.Unlock()
			case "relay endpoint":
				relay.mu.Lock()
				relay.endpoint += "?private"
				relay.mu.Unlock()
			}
			if c, err := newPinnedExternalConnector(ctx, target, peer, options, r); c != nil || err != errPinnedConnectorUnavailable || calls != 0 {
				t.Fatal("rejection launched connector or exposed raw error")
			}
		})
	}
	var c *pinnedExternalConnector
	if c.Stop() != nil || c.checkAlive(nil) != errPinnedConnectorUnavailable {
		t.Fatal("nil binding")
	}
}

func TestPinnedConnectorHandshakeFailures(t *testing.T) {
	for _, mode := range []string{"open error", "nil session", "not attached", "wrong name", "wrong version", "schema", "changed session", "lost during open", "lost during validation", "cancel during open", "probe error", "lost during probe", "cancel during probe"} {
		t.Run(mode, func(t *testing.T) {
			f := newPinnedFixture(t)
			relay, peer := pinnedStart(t, context.Background(), f)
			options := pinnedConnectorOptions(f)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := &pinnedConnectorSession{fakeWorkerSession: fakeManagedSession(t, "bad")}
			switch mode {
			case "wrong name":
				s.info.ServerName = "private-canary"
			case "wrong version":
				s.info.ServerVersion = "1.8.0"
			case "schema":
				delete(s.tools, "click")
			case "changed session":
				s.toolsHook = func() { s.info.PID++ }
			case "lost during validation":
				s.toolsHook = relay.Close
			}
			r := pinnedConnectorRegistry(t, relay, options, s, func() {
				if mode == "lost during open" {
					relay.Close()
				}
				if mode == "cancel during open" {
					cancel()
				}
			})
			if mode == "open error" {
				r.deps.Open = func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) {
					return s, errors.New("private endpoint/token/PID canary")
				}
			}
			if mode == "nil session" {
				r.deps.Open = func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) { return nil, nil }
			}
			if mode == "not attached" {
				r.deps.Open = func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) { return s, nil }
			}
			if mode == "probe error" {
				r.deps.Probe = func(context.Context, mcpclient.ServerConfig) (string, error) {
					return "", errors.New("private probe canary")
				}
			}
			if mode == "lost during probe" || mode == "cancel during probe" {
				r.deps.Probe = func(context.Context, mcpclient.ServerConfig) (string, error) {
					if mode == "lost during probe" {
						relay.Close()
					} else {
						cancel()
					}
					return ManagedEngineVersion, nil
				}
				r.deps.Open = func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) {
					t.Error("open after loss/cancel during probe")
					return s, nil
				}
			}
			if c, err := newPinnedExternalConnector(ctx, relay, peer, options, r); c != nil || err != errPinnedConnectorUnavailable {
				t.Fatal("failed handshake published or raw error escaped")
			}
			pinnedWait(t, relay.done)
			opened := mode != "nil session" && mode != "probe error" && mode != "lost during probe" && mode != "cancel during probe"
			if opened && s.closes != 1 || !opened && s.closes != 0 {
				t.Fatal("owned connector cleanup wrong")
			}
			if f.connections.Load() != 1 {
				t.Fatal("failure reattached")
			}
		})
	}
}

func TestPinnedConnectorAliveInvalidation(t *testing.T) {
	for _, mode := range []string{"worker time", "worker session", "worker state", "live session", "peer", "relay loss", "nil ctx", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			f := newPinnedFixture(t)
			relay, peer := pinnedStart(t, context.Background(), f)
			options := pinnedConnectorOptions(f)
			s := &pinnedConnectorSession{fakeWorkerSession: fakeManagedSession(t, "alive")}
			r := pinnedConnectorRegistry(t, relay, options, s, nil)
			c, err := newPinnedExternalConnector(context.Background(), relay, peer, options, r)
			if err != nil {
				t.Fatal("start")
			}
			before := c.worker
			ctx := context.Background()
			w := c.instance
			w.mu.Lock()
			switch mode {
			case "worker time":
				w.info.CreatedAt = w.info.CreatedAt.Add(time.Second)
			case "worker session":
				w.info.Session.PID++
			case "worker state":
				w.info.State = WorkerFailed
			case "live session":
				s.info.PID++
			case "nil ctx":
				ctx = nil
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			w.mu.Unlock()
			if mode == "peer" {
				relay.mu.Lock()
				relay.peer = pinnedTestPeer()
				relay.mu.Unlock()
			}
			if mode == "relay loss" {
				relay.Close()
			}
			if c.checkAlive(ctx) != errPinnedConnectorUnavailable {
				t.Fatal("changed binding remained alive")
			}
			pinnedWait(t, relay.done)
			if c.worker != before || s.closes != 1 || c.checkAlive(context.Background()) != errPinnedConnectorUnavailable {
				t.Fatal("loss not latched or immutable state changed")
			}
		})
	}
}

func TestPinnedConnectorReservationDuringStart(t *testing.T) {
	f := newPinnedFixture(t)
	relay, peer := pinnedStart(t, context.Background(), f)
	options := pinnedConnectorOptions(f)
	s := &pinnedConnectorSession{fakeWorkerSession: fakeManagedSession(t, "reserved")}
	r := pinnedConnectorRegistry(t, relay, options, s, nil)
	entered, gate := make(chan struct{}), make(chan struct{})
	probe := r.deps.Probe
	r.deps.Probe = func(ctx context.Context, cfg mcpclient.ServerConfig) (string, error) {
		close(entered)
		<-gate
		return probe(ctx, cfg)
	}
	done := make(chan *pinnedExternalConnector, 1)
	go func() {
		c, err := newPinnedExternalConnector(context.Background(), relay, peer, options, r)
		if err != nil {
			t.Error("reserved start failed")
		}
		done <- c
	}()
	<-entered
	if c, err := newPinnedExternalConnector(context.Background(), relay, peer, options, r); c != nil || err != errPinnedConnectorUnavailable {
		t.Fatal("concurrent constructor reused relay")
	}
	close(gate)
	c := <-done
	if c == nil {
		t.Fatal("missing binding")
	}
	_ = c.Stop()
	pinnedWait(t, relay.done)
}
