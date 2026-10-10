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

func fakeBirthIdentity(_ context.Context, pid int) (processcontrol.BirthIdentity, error) {
	return processcontrol.BirthIdentity{
		PID: pid, StartSec: 1000000, StartUsec: 123456, EffectiveUID: 501, ProcessGroupID: pid,
	}, nil
}
func recordFakeManagedWorker(g *sharedBrowserRestartGuard, pid int) error {
	return g.recordManagedWorkerWithIdentity(readyEvidence(pid), fakeBirthIdentity)
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
		return inspectSharedBrowserOrphanEvidenceWithIdentity(ctx, root, obs, fakeBirthIdentity)
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
	if err := recordFakeManagedWorker(g, 0); !errors.Is(err, errSharedBrowserEvidence) {
		t.Fatalf("missing PID produced proof: %v", err)
	}
	if err := recordFakeManagedWorker(g, 1234); err != nil {
		t.Fatal(err)
	}
	if err := recordFakeManagedWorker(g, 5678); err != nil {
		t.Fatal(err)
	}
	visited = nil
	if got := inspect(makeObserver(map[int]string{1234: "alive", 5678: "not_found"})); got.Status != sharedOrphanBirthMatched || got.WorkerObservations != 2 {
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
	// A reused PID is visible but its different kernel birth timestamp is
	// not the originally observed worker; never release the marker.
	reused := func(_ context.Context, pid int) (processcontrol.BirthIdentity, error) {
		id, _ := fakeBirthIdentity(context.Background(), pid)
		id.StartUsec++
		return id, nil
	}
	if got := inspectSharedBrowserOrphanEvidenceWithIdentity(ctx, root,
		makeObserver(map[int]string{1234: "alive", 5678: "not_found"}), reused); got.Status != sharedOrphanBirthChanged {
		t.Fatalf("reused PID was treated as original worker: %+v", got)
	}
	if got := inspectSharedBrowserOrphanEvidenceWithIdentity(ctx, root,
		makeObserver(map[int]string{1234: "alive", 5678: "not_found"}),
		func(context.Context, int) (processcontrol.BirthIdentity, error) {
			return processcontrol.BirthIdentity{}, errors.New("denied")
		}); got.Status != sharedOrphanObservationUnavailable {
		t.Fatalf("failed OS birth query was trusted: %+v", got)
	}
	if _, err := os.Lstat(filepath.Join(root, sharedBrowserRestartMarker)); err != nil {
		t.Fatal("read-only identity mismatch cleared restart guard")
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
	if err := recordFakeManagedWorker(g, 111); err != nil {
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
	if err := recordFakeManagedWorker(g, 222); !errors.Is(err, errSharedBrowserEvidence) {
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
			if err := recordFakeManagedWorker(g, pid+1000); err != nil {
				t.Errorf("append witness: %v", err)
			}
		}(i)
	}
	wg.Wait()
	got := inspectSharedBrowserOrphanEvidenceWithIdentity(context.Background(), root, func(context.Context, int) processcontrol.Snapshot {
		return processcontrol.Snapshot{State: "not_found"}
	}, fakeBirthIdentity)
	if got.Status != sharedOrphanPIDAbsent || got.WorkerObservations != 24 {
		t.Fatalf("concurrent evidence lost: %+v", got)
	}
	if err := g.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedBrowserOrphanLegacyPIDNoBirthAuthority(t *testing.T) {
	root := testPrivateNextState(t)
	marker := filepath.Join(root, sharedBrowserRestartMarker)
	if err := os.WriteFile(marker, []byte(sharedBrowserEvidencePrefix+"worker_ready_pid=999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	inspect := inspectSharedBrowserOrphanEvidenceWithIdentity(context.Background(), root,
		func(context.Context, int) processcontrol.Snapshot { return processcontrol.Snapshot{State: "alive"} },
		func(context.Context, int) (processcontrol.BirthIdentity, error) {
			called = true
			return processcontrol.BirthIdentity{}, errors.New("must not derive birth after the fact")
		})
	if inspect.Status != sharedOrphanPIDVisible || called {
		t.Fatalf("legacy PID unexpectedly acquired birth proof: %+v call=%v", inspect, called)
	}
}

// Test only the current Go process (not any Chrome, Edge or user process).
// The same read-only kernel observation must survive persistence and re-read.
func TestSharedBrowserOrphanRealSelfBirthSnapshot(t *testing.T) {
	root := testPrivateNextState(t)
	g, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	pid := os.Getpid()
	// This test proves the kernel birth journal round-trip for our test PID.
	// It is not a direct-child launch test, which is in process tests.
	if err := g.recordManagedWorkerWithIdentity(readyEvidence(pid), processcontrol.ObserveBirthIdentity); err != nil {
		t.Fatal(err)
	}
	result := inspectSharedBrowserOrphanEvidence(context.Background(), root, processcontrol.Observe)
	if result.Status != sharedOrphanBirthMatched || result.WorkerObservations != 1 {
		t.Fatalf("macOS self birth identity did not match persisted witness: %+v", result)
	}
	if err := g.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedBrowserOrphanRejectMalformedBirthRecord(t *testing.T) {
	root := testPrivateNextState(t)
	marker := filepath.Join(root, sharedBrowserRestartMarker)
	for _, suffix := range []string{
		"worker_ready_pid=22,start_sec=0,start_usec=1,uid=501,pgid=22\\n",
		"worker_ready_pid=22,start_sec=5,start_usec=1000000,uid=501,pgid=22\\n",
		"worker_ready_pid=22,start_sec=5,start_usec=1,uid=4294967296,pgid=22\\n",
		"worker_ready_pid=22,start_sec=5,start_usec=1,uid=501,pgid=0\\n",
		"worker_ready_pid=22,start_sec=5,start_usec=1,uid=501,pgid=22,extra=1\\n",
		"worker_ready_pid=22,start_sec=5,start_usec=1,uid=501\\n",
		"worker_ready_pid=22,start_sec=5,start_usec=-1,uid=501,pgid=22\\n",
	} {
		raw := sharedBrowserEvidencePrefix + strings.ReplaceAll(suffix, "\\n", "\n")
		if err := os.WriteFile(marker, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		seen := false
		result := inspectSharedBrowserOrphanEvidenceWithIdentity(context.Background(), root,
			func(context.Context, int) processcontrol.Snapshot { seen = true; return processcontrol.Snapshot{} }, fakeBirthIdentity)
		if result.Status != sharedOrphanUntrusted || seen {
			t.Fatalf("malformed birth parsed: %+v", result)
		}
	}
}
