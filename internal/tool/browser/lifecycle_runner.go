package browser

import (
	"context"
	"sync"
	"time"
)

type browserLifecycleRunner struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu      sync.Mutex
	lastErr error
}

func newBrowserLifecycleRunner(interval time.Duration, sweep func(time.Time) error) *browserLifecycleRunner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &browserLifecycleRunner{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(r.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				err := sweep(now.UTC())
				r.mu.Lock()
				r.lastErr = err
				r.mu.Unlock()
			}
		}
	}()
	return r
}

func (r *browserLifecycleRunner) Close() {
	if r == nil {
		return
	}
	r.cancel()
	<-r.done
}

func (r *browserLifecycleRunner) LastError() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErr
}
