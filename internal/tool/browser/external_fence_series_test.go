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

// TEST ONLY: this fixture acknowledges the expected peer; production must
// independently revalidate against the live Edge process/socket evidence.
func seriesTestRevalidate(context.Context, externalPeerIdentity, WorkerInfo) error { return nil }

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
	s, err := newExternalFenceSeries(w, p, factory, seriesTestRevalidate)
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
			s, err := newExternalFenceSeries(w, p, factory, seriesTestRevalidate)
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
	s, err := newExternalFenceSeries(w, p, factory, seriesTestRevalidate)
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
		if s, err := newExternalFenceSeries(tc.w, tc.p, tc.f, seriesTestRevalidate); s != nil || err == nil {
			t.Fatal("invalid identity granted series")
		}
	}
	if series, err := newExternalFenceSeries(w, p, factory, nil); series != nil || err == nil {
		t.Fatal("unverified peer granted transport series")
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
	s, err := newExternalFenceSeries(w, p, factory, seriesTestRevalidate)
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

func TestExternalFenceSeriesRequiresFreshPeerRevalidation(t *testing.T) {
	for _, mode := range []string{"pre-error", "pre-panic", "pre-cancel", "post-error", "post-panic", "post-cancel", "identity-mismatch"} {
		t.Run(mode, func(t *testing.T) {
			w, p := seriesTestIdentity()
			calls, opens, actions, closes := 0, 0, 0, 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			verify := func(_ context.Context, expected externalPeerIdentity, worker WorkerInfo) error {
				calls++
				if expected != p || worker != w {
					return errors.New("foreign worker or peer")
				}
				if (mode == "pre-error" && calls == 1) || (mode == "post-error" && calls == 2) {
					return errors.New("raw token secret from failed verifier")
				}
				if (mode == "pre-panic" && calls == 1) || (mode == "post-panic" && calls == 2) {
					panic("raw token panic")
				}
				if (mode == "pre-cancel" && calls == 1) || (mode == "post-cancel" && calls == 2) {
					cancel()
				}
				if mode == "identity-mismatch" && calls == 2 {
					return errors.New("observed different incarnation")
				}
				return nil
			}
			factory := func(context.Context, WorkerInfo, externalPeerIdentity) (externalFencedOperation, error) {
				opens++
				return &seriesTestOp{call: func(context.Context, WorkerInfo, externalPeerIdentity, string, map[string]any) (map[string]any, error) {
					actions++
					return map[string]any{"possibly-sensitive": "must-not-leak"}, nil
				}, close: func() error { closes++; return nil }}, nil
			}
			s, err := newExternalFenceSeries(w, p, factory, verify)
			if err != nil {
				t.Fatal(err)
			}
			result, err := s.CallExternalFenced(ctx, w, p, "list_pages", nil)
			if result != nil || err == nil || err.Error() != externalTransportFenceError().Error() {
				t.Fatalf("peer drift leaked response: result=%v err=%v", result, err)
			}
			if calls != 1 && calls != 2 {
				t.Fatalf("bad peer checks: %d", calls)
			}
			if calls == 1 && (opens != 0 || actions != 0 || closes != 0) {
				t.Fatal("precheck did not block transport")
			}
			if calls == 2 && (opens != 1 || actions != 1 || closes != 1) {
				t.Fatal("postcheck did not close owned transport")
			}
			if another, err := s.CallExternalFenced(context.Background(), w, p, "list_pages", nil); another != nil || err == nil {
				t.Fatal("compromised series replayed")
			}
		})
	}
}

func TestExternalFenceSeriesPeerRevalidatedBeforeAndAfterEveryAction(t *testing.T) {
	w, p := seriesTestIdentity()
	var sequence []string
	verify := func(context.Context, externalPeerIdentity, WorkerInfo) error {
		sequence = append(sequence, "verify")
		return nil
	}
	factory := func(context.Context, WorkerInfo, externalPeerIdentity) (externalFencedOperation, error) {
		sequence = append(sequence, "open")
		return &seriesTestOp{call: func(context.Context, WorkerInfo, externalPeerIdentity, string, map[string]any) (map[string]any, error) {
			sequence = append(sequence, "call")
			return map[string]any{"ok": true}, nil
		}, close: func() error { sequence = append(sequence, "close"); return nil }}, nil
	}
	s, err := newExternalFenceSeries(w, p, factory, verify)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		v, e := s.CallExternalFenced(context.Background(), w, p, "list_pages", nil)
		if e != nil || v["ok"] != true {
			t.Fatal(e)
		}
	}
	want := []string{"verify", "open", "call", "close", "verify"}
	if len(sequence) != len(want)*3 {
		t.Fatalf("missing verifications: %v", sequence)
	}
	for i := 0; i < 3; i++ {
		for j, x := range want {
			if sequence[i*len(want)+j] != x {
				t.Fatalf("step %d: want %v got %v", i, want, sequence)
			}
		}
	}
}

// Exercise the same bounded verification method used by real external leases.
// Source-only fixture witness represents an independently observed identity;
// production is still unwired and cannot use this synthetic witness.
func TestExternalFenceSeriesUsesBoundedLeaseVerifier(t *testing.T) {
	w, p := seriesTestIdentity()
	var verificationCount atomic.Int32
	m := NewExternalLeaseManager(&externalFakeBackend{})
	// A nil attestor is rejected before even creating a one-shot connector.
	opens := 0
	factory := func(context.Context, WorkerInfo, externalPeerIdentity) (externalFencedOperation, error) {
		opens++
		return &seriesTestOp{call: func(context.Context, WorkerInfo, externalPeerIdentity, string, map[string]any) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		}, close: func() error { return nil }}, nil
	}
	s, err := newExternalFenceSeries(w, p, factory, m.verifyPeer)
	if err != nil {
		t.Fatal(err)
	}
	if r, e := s.CallExternalFenced(context.Background(), w, p, "list_pages", nil); r != nil || e == nil || opens != 0 {
		t.Fatal("missing verified source opened connection")
	}
	m.verifier = testExternalPeerVerifier(func(_ context.Context, expected externalPeerIdentity, worker WorkerInfo) (externalPeerIdentity, error) {
		verificationCount.Add(1)
		if worker != w || expected != p {
			return externalPeerIdentity{}, errors.New("wrong worker")
		}
		// Identity comes from an independent fixture witness, not the supplied
		// expected value. This does not constitute real Edge authentication.
		return externalPeerIdentity{source: p.source, incarnation: p.incarnation}, nil
	})
	s, err = newExternalFenceSeries(w, p, factory, m.verifyPeer)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		v, e := s.CallExternalFenced(context.Background(), w, p, "list_pages", nil)
		if e != nil || v["ok"] != true {
			t.Fatal(e)
		}
	}
	if verificationCount.Load() != 4 || opens != 2 {
		t.Fatalf("missing real manager verification before/after each call: verify=%d opens=%d", verificationCount.Load(), opens)
	}
}

func TestExternalFenceSeriesBoundedNoncooperativeVerifierFailClosed(t *testing.T) {
	w, p := seriesTestIdentity()
	entered, unblock := make(chan struct{}), make(chan struct{})
	var once sync.Once
	m := NewExternalLeaseManager(&externalFakeBackend{})
	m.verifier = testExternalPeerVerifier(func(context.Context, externalPeerIdentity, WorkerInfo) (externalPeerIdentity, error) {
		once.Do(func() { close(entered) })
		<-unblock // TEST ONLY: deliberately ignore cancellation.
		return p, nil
	})
	var opened atomic.Int32
	factory := func(context.Context, WorkerInfo, externalPeerIdentity) (externalFencedOperation, error) {
		opened.Add(1)
		return nil, nil
	}
	s, err := newExternalFenceSeries(w, p, factory, m.verifyPeer)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := s.CallExternalFenced(ctx, w, p, "list_pages", nil); done <- e }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("verifier not invoked")
	}
	select {
	case e := <-done:
		if e == nil || e.Error() != externalTransportFenceError().Error() {
			t.Fatalf("unsafe verifier deadline result %v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("uncooperative verifier blocked caller")
	}
	if opened.Load() != 0 {
		t.Fatal("timed out verification opened transport")
	}
	if r, e := s.CallExternalFenced(context.Background(), w, p, "list_pages", nil); r != nil || e == nil {
		t.Fatal("series must remain closed after verification timeout")
	}
	close(unblock) // Release fixture's in-flight slot before test exit.
}
