package browser

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

type pinnedGateSession struct {
	*pinnedConnectorSession
	call func(context.Context) (map[string]any, error)
}

func (s *pinnedGateSession) Call(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return s.call(ctx)
}

// No browser/OS probes: count frames actually read on the one upstream socket.
func pinnedGateBinding(t *testing.T) (*pinnedExternalConnector, *pinnedGateSession, *atomic.Int32, func(int, []byte) error) {
	t.Helper()
	var frames atomic.Int32
	accepted := make(chan func(int, []byte) error, 1)
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		up, err := (&websocket.Upgrader{}).Upgrade(w, req, nil)
		if err != nil {
			return
		}
		defer close(done)
		defer up.Close()
		var writeMu sync.Mutex
		send := func(kind int, frame []byte) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			return up.WriteMessage(kind, frame)
		}
		accepted <- send
		for {
			kind, frame, err := up.ReadMessage()
			if err != nil {
				return
			}
			frames.Add(1)
			if send(kind, frame) != nil {
				return
			}
		}
	}))
	f := &pinnedFixture{endpoint: "ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/browser/gate"}
	// Register fixture cleanup before relay cleanup (Cleanup runs in reverse).
	t.Cleanup(func() { server.Close(); pinnedWait(t, done) })
	relay, peer := pinnedStart(t, context.Background(), f)
	base := &pinnedConnectorSession{fakeWorkerSession: fakeManagedSession(t, "gate")}
	session := &pinnedGateSession{pinnedConnectorSession: base}
	registry := pinnedConnectorRegistry(t, relay, pinnedConnectorOptions(f), base, nil)
	open := registry.deps.Open
	registry.deps.Open = func(ctx context.Context, cfg mcpclient.ServerConfig) (WorkerSession, error) {
		_, err := open(ctx, cfg)
		return session, err
	}
	c, err := newPinnedExternalConnector(context.Background(), relay, peer, pinnedConnectorOptions(f), registry)
	if err != nil {
		t.Fatal("synthetic binding failed", err)
	}
	t.Cleanup(func() { _ = c.Stop() })
	if frames.Load() != 1 {
		t.Fatal("bootstrap frame did not traverse upstream")
	}
	return c, session, &frames, <-accepted
}

func pinnedGateEcho(s *pinnedGateSession) error {
	if err := s.conn.WriteMessage(websocket.TextMessage, []byte(`{"id":1,"method":"Browser.getVersion"}`)); err != nil {
		return err
	}
	_, data, err := s.conn.ReadMessage()
	if err == nil && string(data) != `{"id":1,"method":"Browser.getVersion"}` {
		return errors.New("synthetic response mismatch")
	}
	return err
}

func TestPinnedCDPGateBootstrapPermitAndIdleEvent(t *testing.T) {
	c, s, frames, send := pinnedGateBinding(t)
	calls := 0
	s.call = func(context.Context) (map[string]any, error) {
		calls++
		return map[string]any{"synthetic": true}, pinnedGateEcho(s)
	}
	result, err := c.callBound(context.Background(), c.worker, c.peer, "list_pages", nil)
	if err != nil || result["synthetic"] != true || calls != 1 || frames.Load() != 2 {
		t.Fatal("authorized frame/response did not use pinned socket", err)
	}
	c.relay.mu.Lock()
	closed := c.relay.gate == pinnedCDPClosed && c.relay.permit == nil && !c.relay.lost
	c.relay.mu.Unlock()
	if !closed {
		t.Fatal("permit not deterministically closed")
	}
	// Echo completion on the client does not synchronize the server writer.
	// The fixture send method serializes this event with all echo writes.
	if send(websocket.TextMessage, []byte(`{"method":"Target.targetCreated"}`)) != nil {
		t.Fatal("synthetic event write")
	}
	_, event, err := s.conn.ReadMessage()
	if err != nil || string(event) != `{"method":"Target.targetCreated"}` || frames.Load() != 2 {
		t.Fatal("idle upstream event was blocked")
	}
	if result, err := c.callBound(context.Background(), c.worker, c.peer, "list_pages", nil); result != nil || err != errPinnedConnectorUnavailable {
		t.Fatal("spent connector accepted second operation")
	}
	pinnedWait(t, c.relay.done)
	if calls != 1 || frames.Load() != 2 || s.closes != 1 {
		t.Fatal("second operation dispatched or did not quarantine owned connector")
	}
}

func TestPinnedCDPGateSingleUseQueuedOperationA(t *testing.T) {
	c, s, frames, _ := pinnedGateBinding(t)
	var queued io.WriteCloser
	calls := 0
	s.call = func(context.Context) (map[string]any, error) {
		calls++
		if calls == 1 {
			if err := pinnedGateEcho(s); err != nil {
				return nil, err
			}
			var err error
			queued, err = s.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return nil, err
			}
			// Exceed Gorilla's write buffer: a non-final fragment reaches the
			// relay reader during A, but ReadMessage waits for the final fragment.
			_, err = queued.Write([]byte(`{"id":2,"method":"Browser.getVersion"}` + strings.Repeat(" ", 8192)))
			return map[string]any{"A": true}, err
		}
		// Under B2n-A this final fragment would complete A's queued message
		// inside B's permit. B2n-B must never invoke this second Session.Call.
		if err := queued.Close(); err != nil {
			return nil, err
		}
		_, _, err := s.conn.ReadMessage()
		return map[string]any{"B": true}, err
	}
	result, err := c.callBound(context.Background(), c.worker, c.peer, "list_pages", nil)
	if err != nil || result["A"] != true || frames.Load() != 2 {
		t.Fatal("A frame/result not delivered", err)
	}
	for i := 0; i < 4; i++ {
		if result, err := c.callBound(context.Background(), c.worker, c.peer, "list_pages", nil); result != nil || err != errPinnedConnectorUnavailable {
			t.Fatal("B/replay accepted")
		}
	}
	_ = queued.Close()
	pinnedWait(t, c.relay.done)
	if calls != 1 || frames.Load() != 2 || s.closes != 1 {
		t.Fatal("queued A frame passed in B or connector not quarantined")
	}
	c.relay.mu.Lock()
	spent := c.relay.operationSpent && c.relay.lost && c.relay.gate == pinnedCDPClosed
	c.relay.mu.Unlock()
	if !spent {
		t.Fatal("Close reset single-use reservation")
	}
}

func TestPinnedCDPGateUnsolicitedAndLateFrames(t *testing.T) {
	for _, mode := range []string{"idle", "late", "control"} {
		t.Run(mode, func(t *testing.T) {
			c, s, frames, _ := pinnedGateBinding(t)
			if mode == "late" {
				s.call = func(context.Context) (map[string]any, error) { return map[string]any{"done": true}, nil }
				if _, err := c.callBound(context.Background(), c.worker, c.peer, "list_pages", nil); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "control" {
				_ = s.conn.WriteControl(websocket.PingMessage, []byte("idle"), time.Now().Add(time.Second))
			} else {
				_ = s.conn.WriteMessage(websocket.TextMessage, []byte("unsolicited"))
			}
			pinnedWait(t, c.relay.done)
			_ = c.Stop() // Join automatic owned-connector cleanup.
			if frames.Load() != 1 || c.checkAlive(context.Background()) != errPinnedConnectorUnavailable || s.closes != 1 {
				t.Fatal("unauthorized outbound frame reached upstream or loss not permanent")
			}
		})
	}
}

func TestPinnedCDPGateMismatchZeroFrames(t *testing.T) {
	for _, mode := range []string{"worker", "peer", "live session", "worker generation", "tool"} {
		t.Run(mode, func(t *testing.T) {
			c, s, frames, _ := pinnedGateBinding(t)
			calls := 0
			s.call = func(context.Context) (map[string]any, error) { calls++; return nil, pinnedGateEcho(s) }
			worker, peer, tool := c.worker, c.peer, "list_pages"
			switch mode {
			case "worker":
				worker.CreatedAt = worker.CreatedAt.Add(time.Second)
			case "peer":
				peer = pinnedTestPeer()
			case "live session":
				s.info.PID++
			case "worker generation":
				c.instance.mu.Lock()
				c.instance.info.CreatedAt = c.instance.info.CreatedAt.Add(time.Second)
				c.instance.mu.Unlock()
			case "tool":
				tool = "select_page"
			}
			result, err := c.callBound(context.Background(), worker, peer, tool, map[string]any{"private": "canary"})
			if result != nil || err != errPinnedConnectorUnavailable || calls != 0 || frames.Load() != 1 {
				t.Fatal("mismatch dispatched or exposed data")
			}
		})
	}
}

func TestPinnedCDPGateOverlap(t *testing.T) {
	c, s, frames, _ := pinnedGateBinding(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	s.call = func(context.Context) (map[string]any, error) {
		calls.Add(1)
		close(entered)
		<-release
		return map[string]any{"ok": true}, pinnedGateEcho(s)
	}
	done := make(chan error, 1)
	go func() { _, err := c.callBound(context.Background(), c.worker, c.peer, "list_pages", nil); done <- err }()
	pinnedWait(t, entered)
	result, err := c.callBound(context.Background(), c.worker, c.peer, "list_pages", nil)
	close(release)
	if result != nil || err != errPinnedConnectorUnavailable || <-done != errPinnedConnectorUnavailable || calls.Load() != 1 {
		t.Fatal("overlapping permit dispatched or did not quarantine first call")
	}
	pinnedWait(t, c.relay.done)
	_ = c.Stop()
	if frames.Load() != 1 || s.closes != 1 {
		t.Fatal("overlap forwarded frames or leaked connector")
	}
}

func TestPinnedCDPGateReentrantAndCanceledFirst(t *testing.T) {
	for _, mode := range []string{"reentrant", "canceled", "nil context"} {
		t.Run(mode, func(t *testing.T) {
			c, s, frames, _ := pinnedGateBinding(t)
			calls := 0
			s.call = func(context.Context) (map[string]any, error) {
				calls++
				result, err := c.callBound(context.Background(), c.worker, c.peer, "list_pages", nil)
				if result != nil || err != errPinnedConnectorUnavailable {
					t.Error("reentry accepted")
				}
				return map[string]any{"private": true}, nil
			}
			ctx := context.Background()
			if mode == "canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			} else if mode == "nil context" {
				ctx = nil
			}
			if result, err := c.callBound(ctx, c.worker, c.peer, "list_pages", nil); result != nil || err != errPinnedConnectorUnavailable {
				t.Fatal("invalid first call accepted")
			}
			if result, err := c.callBound(context.Background(), c.worker, c.peer, "list_pages", nil); result != nil || err != errPinnedConnectorUnavailable {
				t.Fatal("invalid first call reopened")
			}
			pinnedWait(t, c.relay.done)
			wantCalls := 0
			if mode == "reentrant" {
				wantCalls = 1
			}
			if calls != wantCalls || frames.Load() != 1 || s.closes != 1 {
				t.Fatal("reentry/cancellation dispatched or leaked")
			}
		})
	}
}

func TestPinnedCDPGateCancellationStopAndGenerationQuarantine(t *testing.T) {
	for _, mode := range []string{"cancel before frame", "cancel queued frame", "cancel after frame", "stop", "registry stop", "relay loss", "session", "generation", "reconnect", "error", "panic", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			c, s, frames, _ := pinnedGateBinding(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.call = func(op context.Context) (map[string]any, error) {
				if mode != "cancel before frame" && mode != "cancel queued frame" && mode != "deadline" {
					if err := pinnedGateEcho(s); err != nil {
						return nil, err
					}
				}
				result := map[string]any{"private": "output canary"}
				switch mode {
				case "cancel before frame", "cancel after frame":
					cancel()
					_ = s.conn.WriteMessage(websocket.TextMessage, []byte("after cancel"))
				case "cancel queued frame":
					// Block the pump at the same mutex used by Close and the
					// per-write check; queued data cannot pass after cancel.
					c.relay.mu.Lock()
					_ = s.conn.WriteMessage(websocket.TextMessage, []byte("queued"))
					cancel()
					c.relay.mu.Unlock()
				case "stop":
					go func() { _ = c.Stop() }()
					<-op.Done()
				case "registry stop":
					go func() { _ = c.registry.Stop(c.worker.WorkerID) }()
					<-op.Done()
				case "relay loss":
					c.relay.Close()
				case "session":
					s.info.PID++
				case "generation":
					c.instance.info.CreatedAt = c.instance.info.CreatedAt.Add(time.Second) // Call holds worker.mu.
				case "reconnect":
					result["structuredContent"] = map[string]any{"reconnected": true}
				case "error":
					return result, errors.New("private args/token/PID canary")
				case "panic":
					panic("private canary")
				case "deadline":
					<-op.Done()
				}
				return result, nil // Even apparently successful late output is quarantined.
			}
			result, err := c.callBound(ctx, c.worker, c.peer, "list_pages", nil)
			pinnedWait(t, c.relay.done)
			want := int32(2)
			if mode == "cancel before frame" || mode == "cancel queued frame" || mode == "deadline" {
				want = 1
			}
			if result != nil || err != errPinnedConnectorUnavailable || frames.Load() != want || s.closes != 1 {
				t.Fatal("invalid output escaped or frame/connector not quarantined", mode)
			}
			if result, err := c.callBound(context.Background(), c.worker, c.peer, "list_pages", nil); result != nil || err != errPinnedConnectorUnavailable || frames.Load() != want {
				t.Fatal("failed/canceled/stopped epoch reopened")
			}
		})
	}
}
