package browser

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

type stressWorkerTracker struct {
	mu         sync.Mutex
	live, peak int
}

func (t *stressWorkerTracker) opened() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.live++
	if t.live > t.peak {
		t.peak = t.live
	}
}
func (t *stressWorkerTracker) closed() {
	t.mu.Lock()
	t.live--
	t.mu.Unlock()
}
func (t *stressWorkerTracker) snapshot() (live, peak int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.live, t.peak
}

type stressWorkerSession struct {
	mu       sync.Mutex
	info     mcpclient.SessionInfo
	tools    map[string]mcpclient.Tool
	identity string
	tracker  *stressWorkerTracker
	closed   bool
}

func (s *stressWorkerSession) Info() mcpclient.SessionInfo      { return s.info }
func (s *stressWorkerSession) Tools() map[string]mcpclient.Tool { return s.tools }
func (s *stressWorkerSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.tracker.closed()
	}
	return nil
}
func (s *stressWorkerSession) Call(_ context.Context, tool string, args map[string]any) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("closed")
	}
	switch tool {
	case "new_page":
		isolation, _ := args["isolatedContext"].(string)
		return map[string]any{"structuredContent": map[string]any{"pages": []any{map[string]any{"id": float64(1), "isolatedContext": isolation}}}}, nil
	case "close_page":
		return map[string]any{}, nil
	default:
		return map[string]any{"worker": s.identity, "page_id": args["pageId"], "tool": tool}, nil
	}
}

type acpStressResult struct {
	index    int
	token    string
	leaseID  string
	workerID string
	pageID   any
	err      error
}

func TestACPBridgeEightOwnerIsolationAndBoundedConcurrency(t *testing.T) {
	catalog, err := browserpolicy.NewCatalog(browserpolicy.CatalogDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := NewRoutePlanner(nil, catalog, nil)
	if err != nil {
		t.Fatal(err)
	}
	tracker := &stressWorkerTracker{}
	var seq atomic.Int64
	registry := NewWorkerRegistry(WorkerDependencies{
		Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return ManagedEngineVersion, nil },
		Open: func(_ context.Context, _ mcpclient.ServerConfig) (WorkerSession, error) {
			id := seq.Add(1)
			tracker.opened()
			return &stressWorkerSession{
				info:  mcpclient.SessionInfo{ServerName: ManagedEngineServerName, ServerVersion: ManagedEngineVersion, ProtocolVersion: "2025-06-18", PID: int(id)},
				tools: managedToolFixture(t), identity: fmt.Sprintf("worker-%d", id), tracker: tracker,
			}, nil
		},
	})
	bridge, err := NewACPBridge(planner, registry)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bridge.Close() }()
	workspace := t.TempDir()
	const owners = 8
	tokens := make([]string, owners)
	for i := range owners {
		tokens[i], err = bridge.RegisterSession(fmt.Sprintf("acp-stress-%d", i), "stress", workspace)
		if err != nil {
			t.Fatal(err)
		}
	}

	gate := make(chan struct{})
	results := make(chan acpStressResult, owners)
	var wg sync.WaitGroup
	for i := range owners {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			meta, acquireErr := bridge.Acquire(context.Background(), tokens[i], "about:blank")
			if acquireErr != nil {
				results <- acpStressResult{index: i, token: tokens[i], err: acquireErr}
				return
			}
			snapshot, callErr := bridge.Call(context.Background(), tokens[i], meta.BrowserLeaseID, "take_snapshot", nil)
			results <- acpStressResult{index: i, token: tokens[i], leaseID: meta.BrowserLeaseID, workerID: meta.WorkerID, pageID: snapshot["page_id"], err: callErr}
			<-gate
			_, releaseErr := bridge.Release(context.Background(), tokens[i], meta.BrowserLeaseID)
			if releaseErr != nil {
				t.Errorf("owner %d release: %v", i, releaseErr)
			}
			if releaseErr := bridge.ReleaseSession(context.Background(), fmt.Sprintf("acp-stress-%d", i)); releaseErr != nil {
				t.Errorf("owner %d session release: %v", i, releaseErr)
			}
		}()
	}

	first := make([]acpStressResult, 0, 4)
	for len(first) < 4 {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatal(result.err)
			}
			first = append(first, result)
		case <-time.After(5 * time.Second):
			t.Fatal("first four leases did not acquire")
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		d := bridge.Diagnostics()
		if d.Queue.Active == 4 && d.Queue.Queued == 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("queue did not settle at 4+4: %+v", d.Queue)
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := bridge.Call(context.Background(), first[0].token, first[1].leaseID, "take_snapshot", nil); err == nil {
		t.Fatal("cross-owner token accessed another lease")
	} else {
		assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
	}
	close(gate)

	all := append([]acpStressResult(nil), first...)
	for len(all) < owners {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatal(result.err)
			}
			all = append(all, result)
		case <-time.After(10 * time.Second):
			t.Fatal("remaining leases did not acquire")
		}
	}
	wg.Wait()
	workers := map[string]bool{}
	for _, result := range all {
		if result.workerID == "" || result.pageID != float64(1) {
			t.Fatalf("result=%+v", result)
		}
		if workers[result.workerID] {
			t.Fatalf("worker reused across exclusive leases: %s", result.workerID)
		}
		workers[result.workerID] = true
	}
	live, peak := tracker.snapshot()
	if len(workers) != owners || live != 0 || peak != 4 {
		t.Fatalf("workers=%d live=%d peak=%d", len(workers), live, peak)
	}
	d := bridge.Diagnostics()
	if d.Queue.Active != 0 || d.Queue.Queued != 0 || len(d.Owners) != 0 {
		t.Fatalf("stress leaked ownership/queue: %+v owners=%d", d.Queue, len(d.Owners))
	}
}
