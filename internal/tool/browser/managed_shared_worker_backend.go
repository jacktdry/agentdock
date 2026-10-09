package browser

import (
	"context"
	"crypto/rand"
	"errors"
	"maps"
	"path/filepath"
	"sync"
	"time"
)

const managedSharedIdleRetention = 30 * time.Second

var errManagedSharedUnavailable = errors.New("shared managed worker unavailable")

type managedSharedBorrow struct {
	page              *float64
	created, released bool
	err               error
}

// Offline opt-in adapter, never constructed by ACPBridge. WorkerID at this
// boundary is a unique borrow token, NOT the underlying worker generation.
// This preserves Manager's paired Stop/recovery semantics without refcount ABA.
// A single canonical workspace is pinned; other roots need a separate adapter.
type managedSharedWorkerBackend struct {
	backend                             ManagedLeaseBackend
	options                             ManagedWorkerOptions
	idleTTL                             time.Duration
	ctx                                 context.Context
	cancel                              context.CancelFunc
	mu                                  sync.Mutex
	worker                              WorkerInfo // Immutable after startDone closes.
	keeperPage                          *float64   // Pool-owned; never borrowed or individually closed.
	startDone                           chan struct{}
	done                                chan struct{}
	gate                                chan struct{} // Serializes all calls, page cleanup and final Stop.
	started, closed, poisoned, stopping bool
	startErr, stopErr                   error
	borrows                             map[string]*managedSharedBorrow
	active                              int
	idleAt                              time.Time
}

var _ ManagedLeaseBackend = (*managedSharedWorkerBackend)(nil)

func newManagedSharedWorkerBackend(backend ManagedLeaseBackend, options ManagedWorkerOptions) (*managedSharedWorkerBackend, error) {
	if backend == nil || !filepath.IsAbs(options.Cwd) || filepath.Clean(options.Cwd) != options.Cwd {
		return nil, errManagedSharedUnavailable
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &managedSharedWorkerBackend{backend: backend, options: options, idleTTL: managedSharedIdleRetention, ctx: ctx, cancel: cancel, startDone: make(chan struct{}), done: make(chan struct{}), gate: make(chan struct{}, 1), borrows: make(map[string]*managedSharedBorrow)}
	b.gate <- struct{}{}
	return b, nil
}

func (b *managedSharedWorkerBackend) start() {
	ctx, cancel := context.WithTimeout(b.ctx, 35*time.Second)
	defer cancel()
	worker, err := b.backend.StartManaged(ctx, b.options)
	if err == nil && (worker.WorkerID == "" || worker.State != WorkerReady) {
		err = errManagedSharedUnavailable
	}
	var keeper *float64
	if err == nil {
		// MCP cannot close its last page. Reserve one before any borrower can
		// become ready; only terminal owned worker Stop cleans this page.
		name := "agentdock-pool-keeper-" + rand.Text()
		result, callErr := b.backend.Call(ctx, worker.WorkerID, "new_page", map[string]any{"url": "about:blank", "background": true, "isolatedContext": name})
		err = callErr
		if err == nil {
			err = leaseCallResultError(result)
		}
		if err == nil {
			var page float64
			page, _, err = isolatedPageID(result, name)
			if err == nil {
				keeper = &page
			}
		}
		if err == nil {
			err = ctx.Err()
		}
	}
	b.mu.Lock()
	b.worker, b.keeperPage, b.startErr = worker, keeper, err
	b.idleAt = time.Now()
	close(b.startDone)
	if err != nil {
		b.poisoned = true
		b.closeLocked()
	}
	b.mu.Unlock()
}

func (b *managedSharedWorkerBackend) StartManaged(ctx context.Context, options ManagedWorkerOptions) (WorkerInfo, error) {
	b.mu.Lock()
	if err := ctx.Err(); err != nil {
		b.mu.Unlock()
		return WorkerInfo{}, err
	}
	if b.closed || b.poisoned || options != b.options {
		b.mu.Unlock()
		return WorkerInfo{}, errManagedSharedUnavailable
	}
	if !b.started {
		b.started = true
		go b.start()
	}
	b.mu.Unlock()
	select {
	case <-ctx.Done():
		return WorkerInfo{}, ctx.Err()
	case <-b.ctx.Done():
		return WorkerInfo{}, errManagedSharedUnavailable
	case <-b.startDone:
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return WorkerInfo{}, err
	}
	if b.startErr != nil {
		return WorkerInfo{}, b.startErr
	}
	if b.closed || b.poisoned || len(b.borrows) >= 1024 {
		return WorkerInfo{}, errManagedSharedUnavailable
	}
	info := b.worker
	info.WorkerID = rand.Text()
	b.borrows[info.WorkerID] = &managedSharedBorrow{}
	b.active++
	return info, nil
}

func (b *managedSharedWorkerBackend) lockCall(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.ctx.Done():
		return errManagedSharedUnavailable
	case <-b.gate:
		return nil
	}
}

// Caller holds gate. MCP formats close responses using its selected page, so
// keep that selection on our reserved page without foregrounding the browser.
func (b *managedSharedWorkerBackend) selectKeeper(ctx context.Context, workerID string, target float64) error {
	b.mu.Lock()
	keeper := b.keeperPage
	valid := !b.closed && workerID == b.worker.WorkerID && keeper != nil && *keeper != target
	b.mu.Unlock()
	if !valid {
		return errManagedSharedUnavailable
	}
	result, err := b.backend.Call(ctx, workerID, "select_page", map[string]any{"pageId": *keeper, "bringToFront": false})
	if err == nil {
		err = leaseCallResultError(result)
	}
	return err
}

func (b *managedSharedWorkerBackend) Call(ctx context.Context, id, tool string, args map[string]any) (map[string]any, error) {
	if err := b.lockCall(ctx); err != nil {
		return nil, err
	}
	defer func() { b.gate <- struct{}{} }()
	b.mu.Lock()
	borrow := b.borrows[id]
	if b.closed || borrow == nil || borrow.released || ctx.Err() != nil {
		b.mu.Unlock()
		return nil, errManagedSharedUnavailable
	}
	if tool == "new_page" {
		isolation, _ := args["isolatedContext"].(string)
		if borrow.created || isolation == "" || args["background"] != true {
			b.mu.Unlock()
			return nil, errManagedSharedUnavailable
		}
		borrow.created = true
	} else {
		page, ok := args["pageId"].(float64)
		if (!safeLeasePageTool(tool) && tool != "close_page") || !ok || borrow.page == nil || page != *borrow.page {
			b.mu.Unlock()
			return nil, errManagedSharedUnavailable
		}
	}
	workerID := b.worker.WorkerID
	b.mu.Unlock()
	callCtx, cancel := context.WithCancel(ctx)
	stopCancel := context.AfterFunc(b.ctx, cancel)
	defer cancel()
	defer stopCancel()
	var result map[string]any
	var err error
	if tool == "close_page" {
		err = b.selectKeeper(callCtx, workerID, args["pageId"].(float64))
		if err != nil {
			b.mu.Lock()
			borrow.err, b.poisoned = errors.Join(borrow.err, err), true
			b.mu.Unlock()
		}
	}
	if err == nil {
		result, err = b.backend.Call(callCtx, workerID, tool, maps.Clone(args))
	}
	if err == nil {
		err = leaseCallResultError(result)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if tool == "new_page" {
		var page float64
		if err == nil {
			isolation, _ := args["isolatedContext"].(string)
			page, _, err = isolatedPageID(result, isolation)
		}
		if err == nil {
			if b.keeperPage == nil || page == *b.keeperPage {
				err = errManagedSharedUnavailable
			}
			for _, other := range b.borrows {
				if other != borrow && other.page != nil && *other.page == page {
					err = errManagedSharedUnavailable
				}
			}
		}
		if err != nil {
			// Unknown side effects cannot be cleaned by guessing page IDs. Retire
			// after remaining holders leave, never kill them on this failure.
			borrow.err, b.poisoned = err, true
		} else {
			borrow.page = &page
		}
	} else if tool == "close_page" && err == nil {
		borrow.page = nil
	}
	if b.closed || ctx.Err() != nil {
		return nil, errManagedSharedUnavailable
	}
	return result, err
}

// Stop releases exactly this borrow. Unlike exclusive Stop, success means its
// owned page is gone; the shared worker may remain idle. Tombstones make retry
// idempotent. At most 1024 borrow tombstones are retained per generation;
// exhaustion rejects new borrows until terminal reap/replacement by its owner.
func (b *managedSharedWorkerBackend) Stop(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.lockCall(ctx); err != nil {
		b.mu.Lock()
		closed := b.closed
		known := b.borrows[id] != nil
		b.mu.Unlock()
		if closed && known {
			return b.Shutdown(ctx)
		}
		return err
	}
	defer func() { b.gate <- struct{}{} }()
	b.mu.Lock()
	borrow := b.borrows[id]
	if borrow == nil {
		b.mu.Unlock()
		return errManagedSharedUnavailable
	}
	if borrow.released {
		err := borrow.err
		b.mu.Unlock()
		return err
	}
	page, workerID := borrow.page, b.worker.WorkerID
	closed := b.closed
	b.mu.Unlock()
	if closed {
		// Do not report page cleanup before final worker Stop has completed.
		return errManagedSharedUnavailable
	}
	var err error
	if page != nil {
		err = b.selectKeeper(ctx, workerID, *page)
		if err == nil {
			result, callErr := b.backend.Call(ctx, workerID, "close_page", map[string]any{"pageId": *page})
			err = callErr
			if err == nil {
				err = leaseCallResultError(result)
			}
		}
	}
	b.mu.Lock()
	borrow.released = true
	borrow.err = errors.Join(borrow.err, err)
	if borrow.err != nil {
		b.poisoned = true
	}
	b.active--
	if b.active == 0 {
		b.idleAt = time.Now()
	}
	b.mu.Unlock()
	return borrow.err
}

// Caller holds mu. One finalizer waits for startup and the sole connection
// gate, so it never stops a session concurrently with startup or Call.
func (b *managedSharedWorkerBackend) closeLocked() {
	b.closed = true
	b.cancel()
	if b.stopping {
		return
	}
	b.stopping = true
	go func() {
		if b.started {
			<-b.startDone
		}
		<-b.gate
		var err error
		if b.worker.WorkerID != "" {
			err = b.backend.Stop(b.worker.WorkerID)
		}
		b.mu.Lock()
		b.stopErr = err
		b.active = 0
		for _, borrow := range b.borrows {
			borrow.released, borrow.page, borrow.err = true, nil, err
		}
		close(b.done)
		b.mu.Unlock()
		b.gate <- struct{}{}
	}()
}

// Manual, terminal reap: no timer, automatic restart, or user process cleanup.
func (b *managedSharedWorkerBackend) ReapIdle(now time.Time) error {
	b.mu.Lock()
	if !b.closed && b.started && b.active == 0 && !b.idleAt.IsZero() && !now.Before(b.idleAt.Add(b.idleTTL)) {
		b.closeLocked()
	}
	closed := b.closed
	b.mu.Unlock()
	if closed {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return b.Shutdown(ctx)
	}
	return nil
}

func (b *managedSharedWorkerBackend) Shutdown(ctx context.Context) error {
	b.mu.Lock()
	b.closeLocked()
	b.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.done:
		b.mu.Lock()
		defer b.mu.Unlock()
		return b.stopErr
	}
}
