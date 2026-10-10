//go:build darwin

package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	processcontrol "github.com/uvwt/agentdock/internal/process"
)

func TestSharedBrowserRefreshDedupLateChildAndFailClosed(t *testing.T) {
	root := testPrivateNextState(t)
	g, err := acquireSharedBrowserRestartGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	g.requireStartedEvidence = true
	wrapper := processcontrol.BirthIdentity{PID: 1234, ParentPID: 100, StartSec: 1000000, StartUsec: 20, EffectiveUID: 501, ProcessGroupID: 1234}
	first := processcontrol.BirthIdentity{PID: 2001, ParentPID: 1234, StartSec: 1000001, StartUsec: 20, EffectiveUID: 501, ProcessGroupID: 2001}
	late := processcontrol.BirthIdentity{PID: 3001, ParentPID: 2001, StartSec: 1000002, StartUsec: 20, EffectiveUID: 501, ProcessGroupID: 3001}
	if err := g.appendSharedBrowserIdentity(wrapper, sharedEvidenceStarted); err != nil {
		t.Fatal(err)
	}
	if err := g.appendSharedBrowserEvidence(wrapper, sharedEvidenceReady, []processcontrol.BirthIdentity{first}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, sharedBrowserRestartMarker)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.appendNewReadyDescendants(wrapper, []processcontrol.BirthIdentity{first}); err != nil {
		t.Fatal(err)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || string(original) != string(unchanged) {
		t.Fatal("duplicate descendant changed immutable journal")
	}
	if err := g.appendNewReadyDescendants(wrapper, []processcontrol.BirthIdentity{first, late}); err != nil {
		t.Fatal(err)
	}
	rowsRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := parseSharedBrowserEvidence(string(rowsRaw))
	if !ok || len(rows) != 4 || rows[2].Stage != sharedEvidenceReady ||
		rows[3].PID != late.PID || rows[3].RootPID != wrapper.PID || rows[3].ParentPID != first.PID {
		t.Fatalf("late witness not anchored to earlier ready & ancestor: valid=%v entries=%+v", ok, rows)
	}
	reused := late
	reused.StartUsec++
	if err := g.appendNewReadyDescendants(wrapper, []processcontrol.BirthIdentity{first, reused}); !errors.Is(err, errSharedBrowserEvidence) {
		t.Fatalf("PID reuse failed to block durable witness: %v", err)
	}
	wrongParent := late
	wrongParent.ParentPID = 9090
	if err := g.appendNewReadyDescendants(wrapper, []processcontrol.BirthIdentity{first, wrongParent}); err == nil {
		t.Fatal("unanchored late child admitted")
	}
	otherRoot := wrapper
	otherRoot.StartUsec++
	if err := g.appendNewReadyDescendants(otherRoot, []processcontrol.BirthIdentity{first}); err == nil {
		t.Fatal("changed root birth allowed refresh")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(rowsRaw) {
		t.Fatal("failed refresh changed journal")
	}
	if err := g.releaseAfterVerifiedClose(); err != nil {
		t.Fatal(err)
	}
}
