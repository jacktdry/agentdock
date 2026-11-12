package launchregistry

import (
	"context"
	"github.com/uvwt/agentdock/internal/process"
	"testing"
)

func TestB6GenerationRevokeDeniesStaleAdapterNativeAfterRecycling(t *testing.T) {
	a := process.BirthIdentity{PID: 10001, ParentPID: 10000, StartSec: 111, EffectiveUID: 500}
	native := process.BirthIdentity{PID: 10002, ParentPID: a.PID, StartSec: 112, EffectiveUID: 500}
	pin, err := NewGenerationPin(a, native)
	if err != nil {
		t.Fatal(err)
	}
	if observed, err := pin.Scope(context.Background()); err != nil || observed.Adapter != a || observed.AppServer != native {
		t.Fatalf("missing pinned native generation %+v %v", observed, err)
	}
	pin.Revoke()
	pin.Revoke()
	if _, err := pin.Scope(context.Background()); err == nil {
		t.Fatal("recycled native generation was still admissible")
	}
	recycled := native
	recycled.StartSec++
	if _, err := NewGenerationPin(a, process.BirthIdentity{PID: recycled.PID, ParentPID: 99999, StartSec: recycled.StartSec, EffectiveUID: recycled.EffectiveUID}); err == nil {
		t.Fatal("stale parent accepted")
	}
}
