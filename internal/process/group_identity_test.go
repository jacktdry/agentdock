package process

import (
	"context"
	"errors"
	"testing"
)

func TestValidatedGroupBirthsRejectInconsistentSnapshot(t *testing.T) {
	valid := BirthIdentity{PID: 22, StartSec: 100, StartUsec: 300, EffectiveUID: 501, ProcessGroupID: 22}
	for _, tc := range []struct {
		name    string
		members []BirthIdentity
	}{
		{"valid", []BirthIdentity{valid}},
		{"empty", nil},
		{"wrong-pgid", []BirthIdentity{{PID: 22, StartSec: 100, StartUsec: 300, ProcessGroupID: 23}}},
		{"zero-birth", []BirthIdentity{{PID: 22, StartUsec: 300, ProcessGroupID: 22}}},
		{"negative-usec", []BirthIdentity{{PID: 22, StartSec: 100, StartUsec: -1, ProcessGroupID: 22}}},
		{"duplicate", []BirthIdentity{valid, valid}},
		{"too-many", make([]BirthIdentity, MaxObservedGroupMembers+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validatedGroupBirths(22, tc.members)
			bad := tc.name != "valid" && tc.name != "empty"
			if bad && !errors.Is(err, ErrGroupObservationUnavailable) {
				t.Fatalf("invalid group accepted: %+v %v", got, err)
			}
			if !bad && (err != nil || len(got.Members) != len(tc.members)) {
				t.Fatalf("valid snapshot rejected: %+v %v", got, err)
			}
		})
	}
	if _, err := ObserveGroupBirths(nil, 12); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ObserveGroupBirths(ctx, 12); err == nil {
		t.Fatal("cancelled context accepted")
	}
	if _, err := ObserveGroupBirths(context.Background(), -1); err == nil {
		t.Fatal("invalid group accepted")
	}
}
