//go:build darwin

package app

import (
	"context"
	"fmt"

	processcontrol "github.com/uvwt/agentdock/internal/process"
)

// An observed child may have reparented or switched process groups.
// Its matching birth fingerprint is only a forensic clue, not a kill handle.
func classifyRecordedDescendantSurvival(ctx context.Context,
	observed []sharedBrowserWorkerEvidence,
	observe func(context.Context, int) processcontrol.Snapshot,
	birth func(context.Context, int) (processcontrol.BirthIdentity, error),
) string {
	if len(observed) == 0 {
		return ""
	}
	if ctx == nil || ctx.Err() != nil || observe == nil || birth == nil {
		return sharedOrphanObservationUnavailable
	}
	matched, changed, unknown := false, false, false
	seen := make(map[string]bool, len(observed))
	for _, e := range observed {
		key := fmt.Sprintf("%d/%d/%d/%d", e.PID, e.StartSec, e.StartUsec, e.UID)
		if seen[key] {
			continue
		}
		seen[key] = true
		snapshot := observe(ctx, e.PID)
		switch snapshot.State {
		case "alive":
			id, err := birth(ctx, e.PID)
			if err != nil || !validSharedBrowserIdentity(id) {
				unknown = true
				break
			}
			if e.PID == id.PID && e.StartSec == id.StartSec && e.StartUsec == id.StartUsec &&
				e.UID == id.EffectiveUID {
				matched = true
			} else {
				changed = true
			}
		case "zombie", "not_found":
		default:
			unknown = true
		}
		if ctx.Err() != nil {
			unknown = true
		}
	}
	switch {
	case unknown:
		return sharedOrphanObservationUnavailable
	case matched:
		return sharedOrphanDescendantVisible
	case changed:
		return sharedOrphanDescendantChanged
	}
	return ""
}
