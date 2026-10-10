package process

import (
	"context"
	"errors"
	"testing"
)

func TestDirectChildBirthProofFailsClosed(t *testing.T) {
	parent := BirthIdentity{PID: 100, ParentPID: 1, StartSec: 100000, StartUsec: 20, EffectiveUID: 501, ProcessGroupID: 100}
	child := BirthIdentity{PID: 200, ParentPID: 100, StartSec: 100001, StartUsec: 30, EffectiveUID: 501, ProcessGroupID: 200}
	fixtures := []struct {
		name     string
		modify   func(*BirthIdentity, *BirthIdentity)
		wantGood bool
	}{
		{"valid", nil, true},
		{"not-direct-child", func(p, c *BirthIdentity) { c.ParentPID = 101 }, false},
		{"unexpected-group", func(p, c *BirthIdentity) { c.ProcessGroupID = 100 }, false},
		{"foreign-owner", func(p, c *BirthIdentity) { c.EffectiveUID = 777 }, false},
		{"older-child", func(p, c *BirthIdentity) { c.StartSec = 99999 }, false},
		{"older-usec", func(p, c *BirthIdentity) { c.StartSec = p.StartSec; c.StartUsec = p.StartUsec - 1 }, false},
		{"missing-parent-birth", func(p, c *BirthIdentity) { p.StartSec = 0 }, false},
		{"reused-parent", func(p, c *BirthIdentity) { p.PID = 300 }, false},
		{"reused-child", func(p, c *BirthIdentity) { c.PID = 300 }, false},
	}
	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			p, c := parent, child
			if tc.modify != nil {
				tc.modify(&p, &c)
			}
			n := 0
			observer := func(_ context.Context, pid int) (BirthIdentity, error) {
				n++
				if pid == 100 {
					return p, nil
				}
				if pid == 200 {
					return c, nil
				}
				return BirthIdentity{}, ErrIdentityUnavailable
			}
			got, err := verifyDirectChildBirth(context.Background(), 100, 200, observer)
			if tc.wantGood {
				if err != nil || got != child || n != 4 {
					t.Fatalf("valid proof %+v %v calls=%d", got, err, n)
				}
			} else if !errors.Is(err, ErrIdentityUnavailable) {
				t.Fatalf("invalid birth relationship admitted %+v %v", got, err)
			}
		})
	}
	t.Run("identity-changes-during-check", func(t *testing.T) {
		n := 0
		observer := func(_ context.Context, pid int) (BirthIdentity, error) {
			n++
			if pid == 100 {
				return parent, nil
			}
			if n >= 4 {
				different := child
				different.StartUsec++
				return different, nil
			}
			return child, nil
		}
		if _, err := verifyDirectChildBirth(context.Background(), 100, 200, observer); err == nil {
			t.Fatal("PID reused during observation was accepted")
		}
	})
	t.Run("denied-snapshot", func(t *testing.T) {
		observer := func(context.Context, int) (BirthIdentity, error) { return BirthIdentity{}, ErrIdentityUnavailable }
		if _, err := verifyDirectChildBirth(context.Background(), 100, 200, observer); err == nil {
			t.Fatal("denied OS read was admitted")
		}
	})
	if _, err := verifyDirectChildBirth(nil, 100, 200, func(context.Context, int) (BirthIdentity, error) { return child, nil }); err == nil {
		t.Fatal("nil context admitted")
	}
	if _, err := verifyDirectChildBirth(context.Background(), 100, 100, func(context.Context, int) (BirthIdentity, error) { return child, nil }); err == nil {
		t.Fatal("self parentage admitted")
	}
}
