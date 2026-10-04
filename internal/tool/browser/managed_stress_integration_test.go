//go:build browser_integration

package browser

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestManagedFourLiveACPLeasesIsolationStress(t *testing.T) {
	if os.Getenv("AGENTDOCK_RUN_STRESS") != "1" {
		t.Skip("set AGENTDOCK_RUN_STRESS=1 for live managed concurrency stress")
	}
	bridge := newTestACPBridge(t)
	workspace := t.TempDir()
	const owners = 4
	tokens := make([]string, owners)
	for i := range owners {
		token, err := bridge.RegisterSession(fmt.Sprintf("live-managed-%d", i), "stress", workspace)
		if err != nil {
			t.Fatal(err)
		}
		tokens[i] = token
	}
	type acquired struct {
		index int
		meta  LeaseMetadata
		err   error
	}
	results := make(chan acquired, owners)
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := range owners {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			meta, err := bridge.Acquire(context.Background(), tokens[i], "about:blank")
			if err != nil {
				results <- acquired{index: i, err: err}
				return
			}
			if _, err := bridge.Call(context.Background(), tokens[i], meta.BrowserLeaseID, "take_snapshot", nil); err != nil {
				results <- acquired{index: i, meta: meta, err: err}
				return
			}
			results <- acquired{index: i, meta: meta}
			<-release
			if _, err := bridge.Release(context.Background(), tokens[i], meta.BrowserLeaseID); err != nil {
				t.Errorf("release owner %d: %v", i, err)
			}
			if err := bridge.ReleaseSession(context.Background(), fmt.Sprintf("live-managed-%d", i)); err != nil {
				t.Errorf("release session %d: %v", i, err)
			}
		}()
	}
	seenWorkers := map[string]bool{}
	seenContexts := map[string]bool{}
	for range owners {
		select {
		case result := <-results:
			if result.err != nil {
				close(release)
				wg.Wait()
				t.Fatalf("owner %d: %v", result.index, result.err)
			}
			if result.meta.WorkerID == "" || result.meta.IsolationContextName == "" || result.meta.PageID == "" {
				t.Fatalf("owner %d incomplete lease: %+v", result.index, result.meta)
			}
			if seenWorkers[result.meta.WorkerID] {
				t.Fatalf("worker reused across live leases: %s", result.meta.WorkerID)
			}
			if seenContexts[result.meta.IsolationContextName] {
				t.Fatalf("context reused across live leases: %s", result.meta.IsolationContextName)
			}
			seenWorkers[result.meta.WorkerID] = true
			seenContexts[result.meta.IsolationContextName] = true
		case <-time.After(60 * time.Second):
			close(release)
			wg.Wait()
			t.Fatal("live managed leases did not all acquire")
		}
	}
	d := bridge.Diagnostics()
	if d.Queue.Active != owners || d.Queue.Queued != 0 || len(d.Leases) < owners {
		close(release)
		wg.Wait()
		t.Fatalf("active diagnostics=%+v leases=%d", d.Queue, len(d.Leases))
	}
	close(release)
	wg.Wait()
	deadline := time.Now().Add(10 * time.Second)
	for {
		d = bridge.Diagnostics()
		if d.Queue.Active == 0 && d.Queue.Queued == 0 && len(d.Owners) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("live managed stress did not return to baseline: queue=%+v owners=%d", d.Queue, len(d.Owners))
		}
		time.Sleep(25 * time.Millisecond)
	}
}
