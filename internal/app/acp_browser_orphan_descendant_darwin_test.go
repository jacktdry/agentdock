//go:build darwin

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	processcontrol "github.com/uvwt/agentdock/internal/process"
)

func TestSharedBrowserBoundedDescendantJournalAndOrphanSurvival(t *testing.T) {
	root := testPrivateNextState(t)
	g, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	g.requireStartedEvidence = true
	wrapper := processcontrol.BirthIdentity{
		PID: 1234, ParentPID: os.Getpid(), StartSec: 1000000, StartUsec: 123,
		EffectiveUID: 501, ProcessGroupID: 1234,
	}
	child := processcontrol.BirthIdentity{
		PID: 4321, ParentPID: 1234, StartSec: 1000001, StartUsec: 10,
		EffectiveUID: 501, ProcessGroupID: 4321,
	}
	grandchild := processcontrol.BirthIdentity{
		PID: 5678, ParentPID: 4321, StartSec: 1000002, StartUsec: 20,
		EffectiveUID: 501, ProcessGroupID: 5678,
	}
	if err := g.appendSharedBrowserIdentity(wrapper, sharedEvidenceStarted); err != nil {
		t.Fatal(err)
	}
	if err := g.appendSharedBrowserEvidence(wrapper, sharedEvidenceReady, []processcontrol.BirthIdentity{child, grandchild}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, sharedBrowserRestartMarker))
	if err != nil {
		t.Fatal(err)
	}
	entries, valid := parseSharedBrowserEvidence(string(raw))
	if !valid || len(entries) != 4 || entries[0].Stage != sharedEvidenceStarted ||
		entries[1].Stage != sharedEvidenceDescendant || entries[2].Stage != sharedEvidenceDescendant ||
		entries[3].Stage != sharedEvidenceReady || entries[1].RootPID != 1234 || entries[2].ParentPID != 4321 {
		t.Fatalf("journal lost anchored descendant birth proof %+v valid=%v", entries, valid)
	}
	wrapperGone := func(_ context.Context, pid int) processcontrol.Snapshot {
		if pid == 4321 || pid == 5678 {
			return processcontrol.Snapshot{RootPID: pid, State: "alive"}
		}
		return processcontrol.Snapshot{RootPID: pid, State: "not_found"}
	}
	birth := func(_ context.Context, pid int) (processcontrol.BirthIdentity, error) {
		if pid == 4321 {
			m := child
			m.ParentPID = 1
			m.ProcessGroupID = 9999
			return m, nil
		} // reparented & regrouped
		if pid == 5678 {
			return grandchild, nil
		}
		return processcontrol.BirthIdentity{}, errors.New("wrapper gone")
	}
	noGroup := func(context.Context, int) (processcontrol.GroupObservation, error) {
		t.Fatal("live child identity should be checked before root group")
		return processcontrol.GroupObservation{}, nil
	}
	state := inspectSharedBrowserOrphanEvidenceWithGroup(context.Background(), root, wrapperGone, birth, noGroup)
	if state.Status != sharedOrphanDescendantVisible || state.WorkerObservations != 4 {
		t.Fatalf("reparented child birth was not advisory detectable: %+v", state)
	}
	reused := func(ctx context.Context, pid int) (processcontrol.BirthIdentity, error) {
		id, err := birth(ctx, pid)
		if err == nil {
			id.StartUsec++
		}
		return id, err
	}
	state = inspectSharedBrowserOrphanEvidenceWithGroup(context.Background(), root, wrapperGone, reused, noGroup)
	if state.Status != sharedOrphanDescendantChanged {
		t.Fatalf("reused child PID accepted as prior generation: %+v", state)
	}
	absent := func(_ context.Context, pid int) processcontrol.Snapshot {
		return processcontrol.Snapshot{RootPID: pid, State: "not_found"}
	}
	group := func(_ context.Context, id int) (processcontrol.GroupObservation, error) {
		if id != 1234 {
			t.Fatalf("child PGID was mistaken for managed worker root: %d", id)
		}
		return processcontrol.GroupObservation{GroupID: id}, nil
	}
	state = inspectSharedBrowserOrphanEvidenceWithGroup(context.Background(), root, absent, birth, group)
	if state.Status != sharedOrphanGroupEmpty {
		t.Fatalf("group was misclassified after all child PIDs disappeared: %+v", state)
	}
	if _, err := os.Lstat(filepath.Join(root, sharedBrowserRestartMarker)); err != nil {
		t.Fatal(err)
	}
	if err := g.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedBrowserDescendantJournalRejectsBrokenChainsAndCorruption(t *testing.T) {
	wrapper := processcontrol.BirthIdentity{PID: 1234, StartSec: 1000000, StartUsec: 100,
		EffectiveUID: 501, ProcessGroupID: 1234}
	child := processcontrol.BirthIdentity{PID: 4321, ParentPID: 1234, StartSec: 1000001, StartUsec: 100,
		EffectiveUID: 501, ProcessGroupID: 4321}
	for _, tc := range []struct {
		name  string
		alter func(*processcontrol.BirthIdentity)
	}{
		{"uid-mismatch", func(c *processcontrol.BirthIdentity) { c.EffectiveUID = 777 }},
		{"missing-parent", func(c *processcontrol.BirthIdentity) { c.ParentPID = 9988 }},
		{"self-parent", func(c *processcontrol.BirthIdentity) { c.ParentPID = c.PID }},
		{"older-than-root", func(c *processcontrol.BirthIdentity) { c.StartSec = 999999 }},
		{"duplicate-root", func(c *processcontrol.BirthIdentity) { c.PID = 1234 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := testPrivateNextState(t)
			g, err := acquireSharedBrowserRestartGuard(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := g.appendSharedBrowserIdentity(wrapper, sharedEvidenceStarted); err != nil {
				t.Fatal(err)
			}
			bad := child
			tc.alter(&bad)
			if err := g.appendSharedBrowserEvidence(wrapper, sharedEvidenceReady, []processcontrol.BirthIdentity{bad}); !errors.Is(err, errSharedBrowserEvidence) {
				t.Fatalf("broken descendant lineage saved: %v", err)
			}
			if err := g.releaseAfterVerifiedClose(); err != nil {
				t.Fatal(err)
			}
		})
	}
	rooted := "worker_started_pid=1234,start_sec=1000000,start_usec=100,uid=501,pgid=1234\n"
	validChild := "worker_descendant_pid=4321,start_sec=1000001,start_usec=100,uid=501,pgid=4321,parent_pid=1234,root_pid=1234,root_start_sec=1000000,root_start_usec=100\n"
	for _, raw := range []string{
		validChild, // child with no started root
		rooted + strings.Replace(validChild, "root_start_sec=1000000", "root_start_sec=1000009", 1),
		rooted + strings.Replace(validChild, "parent_pid=1234", "parent_pid=4567", 1),
		rooted + strings.Replace(validChild, "pgid=4321", "pgid=0", 1),
		rooted + strings.Replace(validChild, "root_pid=1234", "root_pid=0", 1),
		rooted + strings.Replace(validChild, "root_start_usec=100", "root_start_usec=-1", 1),
		rooted + strings.TrimSuffix(validChild, "\n"),
	} {
		if _, valid := parseSharedBrowserEvidence(sharedBrowserEvidencePrefix + raw); valid {
			t.Fatalf("malformed descendant journal accepted %q", raw)
		}
	}
}
