package process

import (
	"context"
	"errors"
	"testing"
)

func TestDescendantLineageBoundedAndDifferentGroups(t *testing.T) {
	root := BirthIdentity{PID: 100, ParentPID: 9, StartSec: 100, StartUsec: 20, EffectiveUID: 501, ProcessGroupID: 100}
	child := BirthIdentity{PID: 200, ParentPID: 100, StartSec: 101, StartUsec: 30, EffectiveUID: 501, ProcessGroupID: 200}
	grandchild := BirthIdentity{PID: 300, ParentPID: 200, StartSec: 102, StartUsec: 40, EffectiveUID: 501, ProcessGroupID: 300}
	unrelated := BirthIdentity{PID: 400, ParentPID: 9, StartSec: 105, StartUsec: 0, EffectiveUID: 501, ProcessGroupID: 400}
	rows := []BirthIdentity{unrelated, grandchild, root, child}
	found, err := boundedDescendantLineage(root, rows)
	if err != nil || len(found) != 2 || found[0] != child || found[1] != grandchild {
		t.Fatalf("did not trace group-diverged grandchild: %+v err=%v", found, err)
	}
	for _, tc := range []struct {
		name    string
		mutate  func([]BirthIdentity) []BirthIdentity
		wantErr bool
		wantNum int
	}{
		{"reparented-not-claimed", func(s []BirthIdentity) []BirthIdentity { s[1].ParentPID = 9; return s }, false, 1},
		{"ancestor-birth-changed", func(s []BirthIdentity) []BirthIdentity { s[3].StartUsec = 31; return s }, false, 2},
		{"grandchild-older-than-parent", func(s []BirthIdentity) []BirthIdentity { s[1].StartSec = 99; return s }, false, 1},
		{"missing-root", func(s []BirthIdentity) []BirthIdentity { return append(s[:2], s[3:]...) }, true, 0},
		{"duplicate-pid", func(s []BirthIdentity) []BirthIdentity { return append(s, s[0]) }, true, 0},
		{"uid-mismatch", func(s []BirthIdentity) []BirthIdentity { s[1].EffectiveUID = 503; return s }, true, 0},
		{"root-reused", func(s []BirthIdentity) []BirthIdentity { s[2].StartUsec++; return s }, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cp := append([]BirthIdentity(nil), rows...)
			got, err := boundedDescendantLineage(root, tc.mutate(cp))
			if tc.wantErr {
				if !errors.Is(err, ErrDescendantObservationUnavailable) {
					t.Fatalf("expected unavailable: %+v %v", got, err)
				}
			} else if err != nil || len(got) != tc.wantNum {
				t.Fatalf("unexpected lineage %+v %v", got, err)
			}
		})
	}
	if _, err := ObserveDescendantBirths(nil, root); !errors.Is(err, ErrDescendantObservationUnavailable) {
		t.Fatal("nil context accepted")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ObserveDescendantBirths(cancelled, root); err == nil {
		t.Fatal("cancelled context accepted")
	}
	sample := []BirthIdentity{root}
	for i := 0; i < MaxObservedDescendants+1; i++ {
		sample = append(sample, BirthIdentity{PID: 1000 + i, ParentPID: 100, StartSec: 101, EffectiveUID: 501, ProcessGroupID: 1000 + i})
	}
	if _, err := boundedDescendantLineage(root, sample); err == nil {
		t.Fatal("oversized lineage accepted")
	}
}
