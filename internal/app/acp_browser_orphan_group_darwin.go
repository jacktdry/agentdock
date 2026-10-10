//go:build darwin

package app

import (
	"context"
	processcontrol "github.com/uvwt/agentdock/internal/process"
)

// These are advisory kernel snapshots only. Group IDs can be recycled;
// detached descendants may not belong to the original group.
func classifyMissingWorkerGroups(ctx context.Context, workers []sharedBrowserWorkerEvidence,
	observe func(context.Context, int) (processcontrol.GroupObservation, error),
) string {
	if ctx == nil || ctx.Err() != nil || observe == nil {
		return sharedOrphanObservationUnavailable
	}
	seen := make(map[int]bool, len(workers))
	present := false
	for _, w := range workers {
		if !w.HasBirth {
			return sharedOrphanPIDAbsent
		}
		if w.PGID != w.PID {
			return sharedOrphanObservationUnavailable
		}
		if seen[w.PGID] {
			continue
		}
		seen[w.PGID] = true
		sample, err := observe(ctx, w.PGID)
		if err != nil || ctx.Err() != nil || sample.GroupID != w.PGID ||
			len(sample.Members) > processcontrol.MaxObservedGroupMembers {
			return sharedOrphanObservationUnavailable
		}
		members, ok := validateBrowserGroupMembers(w, sample.Members)
		if !ok {
			return sharedOrphanGroupConflict
		}
		if members > 0 {
			present = true
		}
	}
	if present {
		return sharedOrphanGroupMembers
	}
	return sharedOrphanGroupEmpty
}

func validateBrowserGroupMembers(w sharedBrowserWorkerEvidence,
	members []processcontrol.BirthIdentity,
) (int, bool) {
	seen := make(map[int]bool, len(members))
	for _, m := range members {
		if !validSharedBrowserIdentity(m) || m.ProcessGroupID != w.PGID ||
			seen[m.PID] || m.EffectiveUID != w.UID ||
			m.StartSec < w.StartSec || (m.StartSec == w.StartSec && m.StartUsec < w.StartUsec) ||
			m.PID == w.PID {
			return 0, false
		}
		seen[m.PID] = true
	}
	return len(members), true
}
