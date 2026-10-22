package browser

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

type seriesTestOp struct {
	call  func(context.Context, WorkerInfo, externalPeerIdentity, string, map[string]any) (map[string]any, error)
	close func() error
}

func (o *seriesTestOp) CallExternalFenced(ctx context.Context, w WorkerInfo, p externalPeerIdentity, name string, args map[string]any) (map[string]any, error) {
	return o.call(ctx, w, p, name, args)
}
func (o *seriesTestOp) Close() error { return o.close() }
func seriesTestIdentity() (WorkerInfo, externalPeerIdentity) {
	return WorkerInfo{WorkerID: "synthetic-owner", State: WorkerReady, Session: mcpclient.SessionInfo{PID: 17, ServerName: "isolated"}}, pinnedTestPeer()
}

func TestExternalFenceSeriesFreshTransportPerOperation(t *testing.T) {
	w, p := seriesTestIdentity()
	var mu sync.Mutex
	var events []string
	active := 0
	factory := func(ctx context.Context, worker WorkerInfo, peer externalPeerIdentity) (externalFencedOperation, error) {
		mu.Lock()
		defer mu.Unlock()
		if active != 0 {
			t.Error("previous transport not joined before next opened")
		}
		active++
		events = append(events, "open")
		return &seriesTestOp{call: func(context.Context, WorkerInfo, externalPeerIdentity, string, map[string]any) (map[string]any, error) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, "call")
			return map[string]any{"synthetic": true}, nil
		}, close: func() error { mu.Lock(); defer mu.Unlock(); events = append(events, "close"); active--; return nil }}, nil
	}
	s, err := newExternalFenceSeries(w, p, factory)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		res, err := s.CallExternalFenced(context.Background(), w, p, "list_pages", nil)
		if err != nil || res["synthetic"] != true {
			t.Fatal("independent synthetic operation failed", err)
		}
	}
	mu.Lock()
	got := append([]string(nil), events...)
	mu.Unlock()
	if len(got) != 9 {
		t.Fatalf("missing operation stages %v", got)
	}
	for i := 0; i < 3; i++ {
		if got[i*3] != "open" || got[i*3+1] != "call" || got[i*3+2] != "close" {
			t.Fatalf("not fresh/joined for operation %d: %v", i, got)
		}
	}
	s.Close()
	if res, err := s.CallExternalFenced(context.Background(), w, p, "list_pages", nil); res != nil || err == nil {
		t.Fatal("closed lease series re-opened")
	}
}

func TestExternalFenceSeriesFailureLatchAndNoRawLeak(t *testing.T) {
	for _, mode := range []string{"bad-worker", "bad-peer", "nil-context", "canceled", "open-error", "open-error-with-op", "cancel-during-open", "open-nil", "open-panic", "call-error", "call-panic", "close-error", "close-panic", "canceled-after-call", "canceled-during-close"} {
		t.Run(mode, func(t *testing.T) {
			w, p := seriesTestIdentity()
			opens, called, closes := 0, 0, 0
			var cancel context.CancelFunc
			ctx := context.Background()
			factory := func(context.Context, WorkerInfo, externalPeerIdentity) (externalFencedOperation, error) {
				opens++
				if mode == "open-error" {
					return nil, errors.New("secret WS url / bearer")
				}
				if mode == "open-nil" {
					return nil, nil
				}
				if mode == "cancel-during-open" {
					cancel()
				}

				if mode == "open-panic" {
					panic("secret WS URL")
				}
				return &seriesTestOp{call: func(context.Context, WorkerInfo, externalPeerIdentity, string, map[string]any) (map[string]any, error) {
					called++
					if mode == "call-error" {
						return map[string]any{"secret": "token"}, errors.New("secret token")
					}
					if mode == "call-panic" {
						panic("secret token")
					}
					if mode == "canceled-after-call" {
						cancel()
					}
					return map[string]any{"secret": "token"}, nil
				}, close: func() error {
					closes++
					if mode == "canceled-during-close" {
						cancel()
					}
					if mode == "close-error" {
						return errors.New("secret socket")
					}
					if mode == "close-panic" {
						panic("secret socket")
					}
					return nil
				}}, func() error {
					if mode == "open-error-with-op" {
						return errors.New("secret factory")
					}
					return nil
				}()
			}
			s, err := newExternalFenceSeries(w, p, factory)
			if err != nil {
				t.Fatal(err)
			}
			mw, mp := w, p
			switch mode {
			case "bad-worker":
				mw.CreatedAt = time.Now()
			case "bad-peer":
				mp = pinnedTestPeer()
			case "nil-context":
				ctx = nil
			case "canceled", "canceled-after-call", "canceled-during-close", "cancel-during-open":
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				if mode == "canceled" {
					cancel()
				}
			}
			res, err := s.CallExternalFenced(ctx, mw, mp, "list_pages", nil)
			if err == nil || err.Error() != externalTransportFenceError().Error() {
				t.Fatalf("unqualified call not rejected with sanitized error: %v", err)
			}
			if res != nil {
				t.Fatal("unsafe result escaped")
			}
			for i := 0; i < 2; i++ {
				r, e := s.CallExternalFenced(context.Background(), w, p, "list_pages", nil)
				if r != nil || e == nil {
					t.Fatal("latched series re-opened")
				}
			}
			if opens > 1 || called > 1 || closes > 1 {
				t.Fatalf("duplicate/multi action: %d/%d/%d", opens, called, closes)
			}
			if opens == 1 && mode != "open-error" && mode != "open-nil" && mode != "open-panic" && closes != 1 {
				t.Fatal("owned operation not closed")
			}
		})
	}
}

func TestExternalFenceSeriesSerializedOverlappingOperations(t *testing.T) {
	w, p := seriesTestIdentity()
	entered := make(chan struct{})
	release := make(chan struct{})
	var opens, closes, active, maxActive atomic.Int32
	factory := func(context.Context, WorkerInfo, externalPeerIdentity) (externalFencedOperation, error) {
		opens.Add(1)
		if n := active.Add(1); n > maxActive.Load() {
			maxActive.Store(n)
		}
		return &seriesTestOp{call: func(context.Context, WorkerInfo, externalPeerIdentity, string, map[string]any) (map[string]any, error) {
			if opens.Load() == 1 {
				close(entered)
				<-release
			}
			return map[string]any{"ok": true}, nil
		}, close: func() error { active.Add(-1); closes.Add(1); return nil }}, nil
	}
	s, err := newExternalFenceSeries(w, p, factory)
	if err != nil {
		t.Fatal(err)
	}
	one := make(chan error, 1)
	two := make(chan error, 1)
	go func() { _, e := s.CallExternalFenced(context.Background(), w, p, "list_pages", nil); one <- e }()
	select {
	case <-entered:
	case <-time.After(4 * time.Second):
		t.Fatal("first operation not entered")
	}
	go func() { _, e := s.CallExternalFenced(context.Background(), w, p, "list_pages", nil); two <- e }()
	// Close must not let a pending operation bypass the first operation's join.
	close(release)
	for _, ch := range []<-chan error{one, two} {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("synthetic operation unexpectedly failed: %v", err)
			}
		case <-time.After(4 * time.Second):
			t.Fatal("serialized operation stalled")
		}
	}
	if opens.Load() != 2 || closes.Load() != 2 || active.Load() != 0 || maxActive.Load() != 1 {
		t.Fatalf("overlap: open=%d close=%d active=%d max=%d", opens.Load(), closes.Load(), active.Load(), maxActive.Load())
	}
}

func TestExternalFenceSeriesConstructorRejectsNoIdentity(t *testing.T) {
	w, p := seriesTestIdentity()
	factory := func(context.Context, WorkerInfo, externalPeerIdentity) (externalFencedOperation, error) {
		t.Fatal("invalid constructor invoked factory")
		return nil, nil
	}
	cases := []struct {
		w WorkerInfo
		p externalPeerIdentity
		f externalFencedOperationFactory
	}{
		{WorkerInfo{}, p, factory}, {w, externalPeerIdentity{}, factory}, {w, pinnedTestPeer(), nil},
	}
	for _, tc := range cases {
		if s, err := newExternalFenceSeries(tc.w, tc.p, tc.f); s != nil || err == nil {
			t.Fatal("invalid identity granted series")
		}
	}
}

// Close must wait for the preceding operation's *cleanup*, not only its
// Session.Call return. This is the fundamental one-transport-at-a-time rule.
func TestExternalFenceSeriesCloseWaitsForOwnedOperationJoin(t *testing.T) {
	w, p := seriesTestIdentity()
	enteredClose, releaseClose := make(chan struct{}), make(chan struct{})
	var opened, joined atomic.Int32
	factory := func(context.Context, WorkerInfo, externalPeerIdentity) (externalFencedOperation, error) {
		opened.Add(1)
		return &seriesTestOp{call: func(context.Context, WorkerInfo, externalPeerIdentity, string, map[string]any) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		}, close: func() error {
			close(enteredClose)
			<-releaseClose
			joined.Add(1)
			return nil
		}}, nil
	}
	s, err := newExternalFenceSeries(w, p, factory)
	if err != nil {
		t.Fatal(err)
	}
	actionDone, closeDone := make(chan error, 1), make(chan struct{})
	go func() { _, e := s.CallExternalFenced(context.Background(), w, p, "list_pages", nil); actionDone <- e }()
	select {
	case <-enteredClose:
	case <-time.After(4 * time.Second):
		t.Fatal("cleanup not entered")
	}
	go func() { s.Close(); close(closeDone) }()
	select {
	case <-closeDone:
		t.Fatal("series Close finished before owned connector joined")
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseClose)
	select {
	case e := <-actionDone:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("action not completed")
	}
	select {
	case <-closeDone:
	case <-time.After(4 * time.Second):
		t.Fatal("series Close did not finish")
	}
	if opened.Load() != 1 || joined.Load() != 1 {
		t.Fatal("connector was not joined exactly once")
	}
	if r, e := s.CallExternalFenced(context.Background(), w, p, "list_pages", nil); r != nil || e == nil {
		t.Fatal("closed series resumed")
	}
}
