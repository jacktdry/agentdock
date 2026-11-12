//go:build darwin

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	processcontrol "github.com/uvwt/agentdock/internal/process"
)

func TestSharedBrowserStartedOnlyBlocksAndPreservesGuard(t *testing.T) {
	root := testPrivateNextState(t)
	g, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	started := processcontrol.BirthIdentity{
		PID: 1234, ParentPID: os.Getpid(), StartSec: 1000000, StartUsec: 123,
		EffectiveUID: 501, ProcessGroupID: 1234,
	}
	if err := g.appendSharedBrowserIdentity(started, sharedEvidenceStarted); err != nil {
		t.Fatal(err)
	}
	inspection := inspectSharedBrowserOrphanEvidence(context.Background(), root, func(context.Context, int) processcontrol.Snapshot {
		t.Fatal("pre-handshake evidence should not need to attach to or inspect a PID")
		return processcontrol.Snapshot{}
	})
	if inspection.Status != sharedOrphanPreHandshakeObserved || inspection.WorkerObservations != 1 {
		t.Fatalf("early-only observation was not distinguished: %+v", inspection)
	}
	raw, err := os.ReadFile(filepath.Join(root, sharedBrowserRestartMarker))
	if err != nil {
		t.Fatal(err)
	}
	items, valid := parseSharedBrowserEvidence(string(raw))
	if !valid || len(items) != 1 || items[0].Stage != sharedEvidenceStarted || !items[0].HasBirth {
		t.Fatalf("invalid early observation: %+v valid=%v", items, valid)
	}
	if err := g.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedBrowserStartedThenReadySameGeneration(t *testing.T) {
	root := testPrivateNextState(t)
	g, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	g.requireStartedEvidence = true
	if err := recordFakeManagedWorker(g, 1234); !errors.Is(err, errSharedBrowserEvidence) {
		t.Fatalf("ready without prior started witness admitted: %v", err)
	}
	identity, _ := fakeBirthIdentity(context.Background(), 1234)
	if err := g.appendSharedBrowserIdentity(identity, sharedEvidenceStarted); err != nil {
		t.Fatal(err)
	}
	if err := recordFakeManagedWorker(g, 1234); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, sharedBrowserRestartMarker))
	if err != nil {
		t.Fatal(err)
	}
	list, valid := parseSharedBrowserEvidence(string(raw))
	if !valid || len(list) != 2 || list[0].Stage != sharedEvidenceStarted || list[1].Stage != sharedEvidenceReady ||
		!list[0].matches(identity) || !list[1].matches(identity) {
		t.Fatalf("start/ready journal lost stage or identity")
	}
	// Same PID but different birth timestamp cannot become ready.
	changed := func(_ context.Context, pid int) (processcontrol.BirthIdentity, error) {
		e, _ := fakeBirthIdentity(context.Background(), pid)
		e.StartUsec++
		return e, nil
	}
	if err := g.recordManagedWorkerWithIdentity(readyEvidence(1234), changed); !errors.Is(err, errSharedBrowserEvidence) {
		t.Fatalf("reused birth accepted for same started PID: %v", err)
	}
	if err := g.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedBrowserStartedJournalStrictParsing(t *testing.T) {
	for _, bad := range []string{
		"worker_started_pid=1234\n",
		"worker_started_pid=0,start_sec=2,start_usec=1,uid=501,pgid=100\n",
		"worker_started_pid=1234,start_sec=2,start_usec=1,uid=501\n",
		"worker_unknown_pid=1234,start_sec=2,start_usec=1,uid=501,pgid=1234\n",
	} {
		if _, valid := parseSharedBrowserEvidence(sharedBrowserEvidencePrefix + strings.ReplaceAll(bad, "\\n", "\n")); valid {
			t.Fatalf("malformed startup record accepted: %s", bad)
		}
	}
}

func TestSharedBrowserActualEarlyStdioSpawnBeforeHandshake(t *testing.T) {
	// This test starts only a private Go test helper, NOT Chrome/npx/Edge.
	root := testPrivateNextState(t)
	g, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	g.requireStartedEvidence = true
	cfg := mcpclient.ServerConfig{Name: "early-test", Transport: mcpclient.TransportStdio,
		Command: os.Args[0], Args: []string{"-test.run=^TestSharedBrowserLaunchParentOwnedFixtureHelper$"},
		StaticEnv: map[string]string{"AGENTDOCK_TEST_BROWSER_CHILD_PROOF": "1"},
		TimeoutMS: 1000,
	}
	var witnessedPID int
	session, err := mcpclient.OpenStdioSessionWithStartedWitness(context.Background(), cfg, func(ctx context.Context, pid int) error {
		witnessedPID = pid
		return g.recordStartedWorker(ctx, pid)
	})
	if session != nil {
		t.Fatal("fixture without MCP handshake unexpectedly published session")
	}
	if err == nil || witnessedPID <= 0 {
		t.Fatalf("expected failed handshake after witnessed Start: %v %d", err, witnessedPID)
	}
	raw, err := os.ReadFile(filepath.Join(root, sharedBrowserRestartMarker))
	if err != nil {
		t.Fatal(err)
	}
	items, valid := parseSharedBrowserEvidence(string(raw))
	if !valid || len(items) != 1 || items[0].Stage != sharedEvidenceStarted || items[0].PID != witnessedPID {
		t.Fatalf("pre-handshake subprocess not durably recorded")
	}
	// The real transport failure completed its own owned-child cleanup.
	if err := g.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
}
