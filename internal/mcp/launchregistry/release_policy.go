package launchregistry

import (
	"context"
	"github.com/uvwt/agentdock/internal/process"
)

// Reconciliation is a truthful operator report, never a signal/kill permit.
// Thread-to-MCP transport ownership is unavailable even when native app-server
// and directly spawned MCP child identities were verified at launch time.
type Reconciliation struct {
	Server                 string `json:"server"`
	Evidence               string `json:"evidence"`
	Status                 string `json:"status"`
	LocalBorrowers         int    `json:"local_borrowers"`
	UnknownNativeBorrowers bool   `json:"unknown_native_borrowers"`
	CleanupAllowed         bool   `json:"cleanup_allowed"`
	ReleaseAction          string `json:"release_action"`
}

// Reconcile checks that the original kernel generation is STILL the one
// observed; stale birth/recycled PID cannot accidentally authorize cleanup.
// A wrapper-owned MCP that exits naturally may be called "observed_exited".
// Never infer its exit reason, specific session, or gracefulness.
func Reconcile(ctx context.Context, ob Observation, localBorrowers int) Reconciliation {
	answer := Reconciliation{Server: ob.Server, Evidence: "kernel_direct_child_at_launch", Status: "unknown_or_deferred", LocalBorrowers: localBorrowers, UnknownNativeBorrowers: true, CleanupAllowed: false, ReleaseAction: "none"}
	if ctx == nil || ctx.Err() != nil || ob.Child.PID <= 0 || ob.Wrapper.PID <= 0 || ob.AppServer.PID <= 0 || localBorrowers < 0 {
		return answer
	}
	actual, err := process.ObserveBirthIdentity(ctx, ob.Child.PID)
	if err != nil || actual != ob.Child {
		answer.Status = "observed_exited_or_replaced"
		return answer
	}
	wrapper, err := process.ObserveBirthIdentity(ctx, ob.Wrapper.PID)
	if err != nil || wrapper != ob.Wrapper {
		return answer
	}
	parent, err := process.ObserveBirthIdentity(ctx, ob.AppServer.PID)
	if err != nil || parent != ob.AppServer {
		return answer
	}
	if actual.ParentPID != wrapper.PID || wrapper.ParentPID != parent.PID {
		return answer
	}
	if localBorrowers > 0 {
		answer.Status = "still_in_use"
	} else {
		answer.Status = "unknown_or_deferred"
	}
	return answer
}
