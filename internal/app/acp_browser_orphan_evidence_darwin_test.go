//go:build darwin

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	processcontrol "github.com/uvwt/agentdock/internal/process"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

func readyEvidence(pid int) toolbrowser.WorkerInfo {
	return toolbrowser.WorkerInfo{State: toolbrowser.WorkerReady, Session: mcpclient.SessionInfo{PID: pid}}
}

func TestSharedBrowserOrphanEvidenceConservativeStateMatrix(t *testing.T) {
	root := testPrivateNextState(t)
	ctx := context.Background()
	visited := make([]int, 0)
	makeObserver := func(states map[int]string) func(context.Context, int) processcontrol.Snapshot {
		return func(_ context.Context, pid int) processcontrol.Snapshot {
			visited = append(visited, pid)
			return processcontrol.Snapshot{RootPID: pid, State: states[pid]}
		}
	}
	inspect := func(obs func(context.Context, int) processcontrol.Snapshot) sharedBrowserOrphanInspection {
		return inspectSharedBrowserOrphanEvidence(ctx, root, obs)
	}
	if got := inspect(makeObserver(nil)); got.Status != sharedOrphanNoMarker {
		t.Fatalf("no marker: %+v", got)
	}
	g, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := inspect(makeObserver(nil)); got.Status != sharedOrphanStartupUnknown {
		t.Fatalf("pre-launch evidence not unknown: %+v", got)
	}
	if err := g.recordManagedWorker(readyEvidence(0)); !errors.Is(err, errSharedBrowserEvidence) {
		t.Fatalf("missing PID produced proof: %v", err)
	}
	if err := g.recordManagedWorker(readyEvidence(1234)); err != nil {
		t.Fatal(err)
	}
	if err := g.recordManagedWorker(readyEvidence(5678)); err != nil {
		t.Fatal(err)
	}
	visited = nil
	if got := inspect(makeObserver(map[int]string{1234: "alive", 5678: "not_found"})); got.Status != sharedOrphanPIDVisible || got.WorkerObservations != 2 {
		t.Fatalf("PID present was treated as owned: %+v", got)
	}
	if len(visited) != 2 {
		t.Fatalf("missing old generation observations: %v", visited)
	}
	if got := inspect(makeObserver(map[int]string{1234: "not_found", 5678: "zombie"})); got.Status != sharedOrphanPIDAbsent {
		t.Fatalf("absent wrapper proved descendants gone: %+v", got)
	}
	if _, err := os.Lstat(filepath.Join(root, sharedBrowserRestartMarker)); err != nil {
		t.Fatal("read-only classification cleared crash fence")
	}
	if got := inspect(makeObserver(map[int]string{1234: "unknown", 5678: "not_found"})); got.Status != sharedOrphanObservationUnavailable {
		t.Fatalf("unknown ps result trusted: %+v", got)
	}
	if err := g.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
	if got := inspect(makeObserver(nil)); got.Status != sharedOrphanNoMarker {
		t.Fatalf("clean Close did not remove own marker: %+v", got)
	}
}

func TestSharedBrowserOrphanEvidenceCorruptionKeepsMarker(t *testing.T) {
	root := testPrivateNextState(t)
	g, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.recordManagedWorker(readyEvidence(111)); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, sharedBrowserRestartMarker)
	if err := os.WriteFile(marker, []byte("forged/invalid marker"), 0600); err != nil {
		t.Fatal(err)
	}
	inspect := inspectSharedBrowserOrphanEvidence(context.Background(), root, func(context.Context, int) processcontrol.Snapshot {
		t.Fatal("corrupted journal should not probe a PID")
		return processcontrol.Snapshot{}
	})
	if inspect.Status != sharedOrphanUntrusted {
		t.Fatalf("corruption classification: %+v", inspect)
	}
	if err := g.recordManagedWorker(readyEvidence(222)); !errors.Is(err, errSharedBrowserEvidence) {
		t.Fatalf("corrupt marker admitted Worker: %v", err)
	}
	if err := g.releaseAfterVerifiedClose(); !errors.Is(err, errSharedBrowserRestartGuard) {
		t.Fatalf("corruption unlinked evidence: %v", err)
	}
	if raw, err := os.ReadFile(marker); err != nil || !strings.Contains(string(raw), "forged") {
		t.Fatalf("lost untrusted marker: %q %v", raw, err)
	}
}

func TestSharedBrowserOrphanEvidenceSymlinkAndPartialFileNeverProbe(t *testing.T) {
	root := testPrivateNextState(t)
	marker := filepath.Join(root, sharedBrowserRestartMarker)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(marker, alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(alias, marker); err != nil {
		t.Fatal(err)
	}
	call := false
	probe := func(context.Context, int) processcontrol.Snapshot { call = true; return processcontrol.Snapshot{} }
	if state := inspectSharedBrowserOrphanEvidence(context.Background(), root, probe); state.Status != sharedOrphanUntrusted {
		t.Fatalf("symlink marker accepted: %+v", state)
	}
	if call {
		t.Fatal("symlink marker caused process lookup")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte(sharedBrowserEvidencePrefix+"worker_ready_pid=123"), 0600); err != nil {
		t.Fatal(err)
	}
	if state := inspectSharedBrowserOrphanEvidence(context.Background(), root, probe); state.Status != sharedOrphanUntrusted {
		t.Fatalf("partial evidence accepted: %+v", state)
	}
	if call {
		t.Fatal("partial evidence caused process lookup")
	}
}

func TestSharedBrowserOrphanEvidenceConcurrentAppendRead(t *testing.T) {
	root := testPrivateNextState(t)
	g, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(pid int) {
			defer wg.Done()
			if err := g.recordManagedWorker(readyEvidence(pid + 1000)); err != nil {
				t.Errorf("append witness: %v", err)
			}
		}(i)
	}
	wg.Wait()
	got := inspectSharedBrowserOrphanEvidence(context.Background(), root, func(context.Context, int) processcontrol.Snapshot {
		return processcontrol.Snapshot{State: "not_found"}
	})
	if got.Status != sharedOrphanPIDAbsent || got.WorkerObservations != 24 {
		t.Fatalf("concurrent evidence lost: %+v", got)
	}
	if err := g.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
}
