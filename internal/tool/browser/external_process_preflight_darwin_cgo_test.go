//go:build darwin && cgo

package browser

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestEdgeLiveSignatureNativeRejectsTestProcess(t *testing.T) {
	// Only this test process is queried; never discover or inspect user's Edge.
	for _, pid := range []int{os.Getpid(), 0, -1, int(1 << 31)} {
		if err := (macEdgeProcessProbe{}).liveSignature(context.Background(), pid); err != errEdgeProcessUnqualified {
			t.Fatal("non-Microsoft test process or invalid PID qualified")
		}
	}
}

func TestEdgeLiveSignatureBoundsOutstandingNativeWork(t *testing.T) {
	for _, mode := range []string{"timeout", "canceled", "hard caller timeout"} {
		t.Run(mode, func(t *testing.T) {
			started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			v := edgeLiveSignatureVerifier{slot: make(chan struct{}, 1), verify: func(int) bool {
				close(started)
				<-release
				return true
			}}
			defer func() {
				close(release)
				// Wait for worker completion, including slot release.
				v.slot <- struct{}{}
				<-v.slot
			}()
			callerLimit := 100 * time.Millisecond
			if mode == "hard caller timeout" {
				callerLimit = time.Minute // verifier must impose its own two-second cap
			}
			ctx, cancel := context.WithTimeout(context.Background(), callerLimit)
			defer cancel()
			go func() {
				defer close(finished)
				if v.check(ctx, 123) != errEdgeProcessUnqualified {
					t.Error("late native success qualified timed-out caller")
				}
			}()
			<-started
			if mode == "canceled" {
				cancel()
			}
			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				t.Fatal("native work blocked caller deadline")
			}
			for i := 0; i < 100; i++ {
				if v.check(context.Background(), 123) != errEdgeProcessUnqualified {
					t.Fatal("occupied native slot allowed another verification")
				}
			}
		})
	}
}

func TestEdgeLiveSignatureUnavailableAndCancellation(t *testing.T) {
	v := edgeLiveSignatureVerifier{slot: make(chan struct{}, 1)}
	if v.check(context.Background(), 123) != errEdgeProcessUnqualified {
		t.Fatal("unavailable native verifier qualified")
	}
	v.verify = func(int) bool { t.Error("canceled context invoked native verifier"); return true }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, ctx} {
		if v.check(ctx, 123) != errEdgeProcessUnqualified {
			t.Fatal("invalid context qualified")
		}
	}
}

func TestEdgeLiveSignatureVerifierPanicFailsClosed(t *testing.T) {
	v := edgeLiveSignatureVerifier{slot: make(chan struct{}, 1), verify: func(int) bool {
		panic("private native verifier panic detail")
	}}
	if err := v.check(context.Background(), 123); err != errEdgeProcessUnqualified {
		t.Fatal("unexpected native verifier panic must fail closed")
	}
	// The first worker releases its only slot on panic.
	v.verify = func(int) bool { return false }
	for i := 0; i < 3; i++ {
		if err := v.check(context.Background(), 123); err != errEdgeProcessUnqualified {
			t.Fatal("unexpected native verifier recovery qualified process")
		}
	}
}
