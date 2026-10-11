package launchregistry

import (
	"context"
	"github.com/uvwt/agentdock/internal/process"
	"testing"
)

func TestB5ReconcileNeverKillsUnknownNativeBorrowerOrReusedPID(t *testing.T) {
	child := process.BirthIdentity{PID: 99999999, StartSec: 1}
	ob := Observation{Server: "dedicated", Child: child, Wrapper: process.BirthIdentity{PID: 1, StartSec: 1}, AppServer: process.BirthIdentity{PID: 2, StartSec: 1}}
	for _, count := range []int{-1, 0, 1, 2, 8} {
		result := Reconcile(context.Background(), ob, count)
		if result.CleanupAllowed || result.ReleaseAction != "none" || !result.UnknownNativeBorrowers {
			t.Fatalf("unknown native owner authorized kill: %+v", result)
		}
	}
	if result := Reconcile(nil, ob, 0); result.CleanupAllowed || result.Status != "unknown_or_deferred" {
		t.Fatalf("nil context admitted %v", result)
	}
}
