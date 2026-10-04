package browser

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestBrowserLifecycleRunnerSweepsAndStops(t *testing.T) {
	var calls atomic.Int32
	wantErr := errors.New("sweep failed")
	runner := newBrowserLifecycleRunner(5*time.Millisecond, func(time.Time) error {
		calls.Add(1)
		return wantErr
	})
	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatal("lifecycle runner never swept")
	}
	if !errors.Is(runner.LastError(), wantErr) {
		t.Fatalf("last error=%v", runner.LastError())
	}
	runner.Close()
	stoppedAt := calls.Load()
	time.Sleep(15 * time.Millisecond)
	if calls.Load() != stoppedAt {
		t.Fatalf("runner continued after close: before=%d after=%d", stoppedAt, calls.Load())
	}
}
