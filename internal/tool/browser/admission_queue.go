package browser

import (
	"context"
	"slices"
	"sync"
	"time"
)

const (
	ErrQueueFull         = "BROWSER_QUEUE_FULL"
	ErrQueueTimeout      = "BROWSER_QUEUE_TIMEOUT"
	ErrLockOwnerMismatch = "BROWSER_LOCK_OWNER_MISMATCH"
)

// admissionQueue is strict FIFO, including head-of-line blocking for multi-key
// requests. A waiter owns only a channel and context timer, never a goroutine.
// limit == 0 is for independent resource locks, with no browser slot limit.
type admissionQueue struct {
	mu                      sync.Mutex
	limit, capacity, active int
	timeout                 time.Duration
	held                    map[string]bool
	waiters                 []*admissionWaiter
}

type admissionWaiter struct {
	ctx               context.Context
	keys              []string // Immutable, canonical and unique.
	ready             chan struct{}
	granted, released bool
	queue             *admissionQueue
}

func newAdmissionQueue(limit, capacity int, timeout time.Duration) *admissionQueue {
	return &admissionQueue{limit: limit, capacity: capacity, timeout: timeout, held: make(map[string]bool)}
}

func (q *admissionQueue) canGrant(w *admissionWaiter) bool {
	if q.limit > 0 && q.active >= q.limit {
		return false
	}
	for _, key := range w.keys {
		if q.held[key] {
			return false
		}
	}
	return true
}

func (q *admissionQueue) grant(w *admissionWaiter) {
	q.active++
	for _, key := range w.keys {
		q.held[key] = true
	}
	w.granted = true
	close(w.ready)
}

func (q *admissionQueue) pump() {
	// Prune canceled waiters even behind a blocked head, so they consume no
	// queue capacity and can never be granted by a later release.
	q.waiters = slices.DeleteFunc(q.waiters, func(w *admissionWaiter) bool { return w.ctx.Err() != nil })
	for len(q.waiters) > 0 {
		w := q.waiters[0]
		if !q.canGrant(w) {
			break
		}
		q.waiters = slices.Delete(q.waiters, 0, 1)
		if w.ctx.Err() == nil {
			q.grant(w)
		}
	}
}

func (q *admissionQueue) acquire(ctx context.Context, keys []string) (*admissionWaiter, error) {
	waitCtx, cancel := context.WithTimeout(ctx, q.timeout)
	defer cancel()
	w := &admissionWaiter{ctx: waitCtx, keys: keys, ready: make(chan struct{}), queue: q}
	q.mu.Lock()
	q.pump()
	if err := waitCtx.Err(); err != nil {
		q.mu.Unlock()
		return nil, admissionError(err)
	}
	if len(q.waiters) == 0 && q.canGrant(w) {
		q.grant(w)
	} else if len(q.waiters) >= q.capacity {
		q.mu.Unlock()
		return nil, leaseError(ErrQueueFull, "", "bounded admission queue full", nil)
	} else {
		q.waiters = append(q.waiters, w)
	}
	q.mu.Unlock()
	select {
	case <-w.ready:
	case <-waitCtx.Done():
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := waitCtx.Err(); err != nil {
		q.waiters = slices.DeleteFunc(q.waiters, func(v *admissionWaiter) bool { return v == w })
		q.releaseLocked(w)
		q.pump()
		return nil, admissionError(err)
	}
	return w, nil
}

func admissionError(err error) error {
	if err == context.DeadlineExceeded {
		return leaseError(ErrQueueTimeout, "", "admission wait timed out", err)
	}
	return err
}

func (q *admissionQueue) releaseLocked(w *admissionWaiter) {
	if !w.granted || w.released {
		return
	}
	w.released = true
	q.active--
	for _, key := range w.keys {
		delete(q.held, key)
	}
}

func (w *admissionWaiter) release() {
	q := w.queue
	q.mu.Lock()
	defer q.mu.Unlock()
	q.releaseLocked(w)
	q.pump()
}
