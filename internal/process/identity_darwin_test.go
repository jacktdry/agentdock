//go:build darwin

package process

import (
	"context"
	"os"
	"testing"
)

func TestBirthIdentityCurrentProcessAndUnavailable(t *testing.T) {
	self := os.Getpid()
	a, err := ObserveBirthIdentity(context.Background(), self)
	if err != nil || a.PID != self || a.StartSec <= 0 || a.ProcessGroupID <= 0 || a.EffectiveUID != uint32(os.Geteuid()) {
		t.Fatalf("own read-only birth identity unavailable: %+v %v", a, err)
	}
	b, err := ObserveBirthIdentity(context.Background(), self)
	if err != nil || b != a {
		t.Fatalf("stable birth identity changed: %+v -> %+v %v", a, b, err)
	}
	if _, err := ObserveBirthIdentity(context.Background(), -1); err == nil {
		t.Fatal("negative PID accepted")
	}
	if _, err := ObserveBirthIdentity(nil, self); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ObserveBirthIdentity(ctx, self); err == nil {
		t.Fatal("canceled context accepted")
	}
	// Avoid a guessed missing PID: no other process may be inspected or signaled.
}
