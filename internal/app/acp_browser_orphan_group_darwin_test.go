//go:build darwin

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	processcontrol "github.com/uvwt/agentdock/internal/process"
)

func TestSharedBrowserOrphanGroupAdvisoryOnly(t *testing.T) {
	root := testPrivateNextState(t)
	guard, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := recordFakeManagedWorker(guard, 1234); err != nil {
		t.Fatal(err)
	}
	absent := func(_ context.Context, pid int) processcontrol.Snapshot {
		return processcontrol.Snapshot{RootPID: pid, State: "not_found"}
	}
	check := func(try func(context.Context, int) (processcontrol.GroupObservation, error), want string) {
		t.Helper()
		result := inspectSharedBrowserOrphanEvidenceWithGroup(context.Background(), root, absent, fakeBirthIdentity, try)
		if result.Status != want || result.WorkerObservations != 1 {
			t.Fatalf("group inspection expected %s, got %+v", want, result)
		}
		if _, err := os.Lstat(filepath.Join(root, sharedBrowserRestartMarker)); err != nil {
			t.Fatal("inspection unexpectedly cleared pending restart marker")
		}
	}
	empty := func(_ context.Context, group int) (processcontrol.GroupObservation, error) {
		return processcontrol.GroupObservation{GroupID: group}, nil
	}
	check(empty, sharedOrphanGroupEmpty)
	survivor := func(_ context.Context, group int) (processcontrol.GroupObservation, error) {
		return processcontrol.GroupObservation{GroupID: group, Members: []processcontrol.BirthIdentity{
			{PID: 4321, StartSec: 1000001, StartUsec: 300, EffectiveUID: 501, ProcessGroupID: group},
		}}, nil
	}
	check(survivor, sharedOrphanGroupMembers)
	for _, member := range []processcontrol.BirthIdentity{
		{PID: 4321, StartSec: 999999, StartUsec: 300, EffectiveUID: 501, ProcessGroupID: 1234},
		{PID: 4321, StartSec: 1000001, StartUsec: 300, EffectiveUID: 999, ProcessGroupID: 1234},
		{PID: 4321, StartSec: 1000001, StartUsec: 300, EffectiveUID: 501, ProcessGroupID: 999},
		{PID: 1234, StartSec: 1000001, StartUsec: 300, EffectiveUID: 501, ProcessGroupID: 1234},
	} {
		bad := member
		check(func(_ context.Context, group int) (processcontrol.GroupObservation, error) {
			return processcontrol.GroupObservation{GroupID: group, Members: []processcontrol.BirthIdentity{bad}}, nil
		}, sharedOrphanGroupConflict)
	}
	check(func(_ context.Context, group int) (processcontrol.GroupObservation, error) {
		return processcontrol.GroupObservation{}, errors.New("kernel snapshot unavailable")
	}, sharedOrphanObservationUnavailable)
	check(func(_ context.Context, group int) (processcontrol.GroupObservation, error) {
		return processcontrol.GroupObservation{GroupID: group + 1}, nil
	}, sharedOrphanObservationUnavailable)
	if err := guard.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedBrowserOrphanGroupLegacyOrNonleaderCannotBeAsserted(t *testing.T) {
	group := func(context.Context, int) (processcontrol.GroupObservation, error) {
		t.Fatal("invalid group provenance must never trigger kernel enumeration")
		return processcontrol.GroupObservation{}, nil
	}
	legacy := []sharedBrowserWorkerEvidence{{PID: 1234}}
	if got := classifyMissingWorkerGroups(context.Background(), legacy, group); got != sharedOrphanPIDAbsent {
		t.Fatalf("legacy journal elevated to group evidence: %s", got)
	}
	nonleader := []sharedBrowserWorkerEvidence{
		{PID: 1234, HasBirth: true, StartSec: 1000000, StartUsec: 10, UID: 501, PGID: 22},
	}
	if got := classifyMissingWorkerGroups(context.Background(), nonleader, group); got != sharedOrphanObservationUnavailable {
		t.Fatalf("nonleader worker inferred group: %s", got)
	}
}
