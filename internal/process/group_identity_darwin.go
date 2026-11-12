//go:build darwin

package process

import "golang.org/x/sys/unix"

// A kernel process-group enumeration, not ps command output and never killpg.
// A disassociated Chrome descendant will NOT necessarily appear here.
func observeGroupBirths(groupID int) ([]BirthIdentity, error) {
	rows, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", groupID)
	if err != nil {
		return nil, ErrGroupObservationUnavailable
	}
	if len(rows) > MaxObservedGroupMembers {
		return nil, ErrGroupObservationUnavailable
	}
	ids := make([]BirthIdentity, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, BirthIdentity{
			PID: int(r.Proc.P_pid), ParentPID: int(r.Eproc.Ppid), StartSec: r.Proc.P_starttime.Sec,
			StartUsec:    int64(r.Proc.P_starttime.Usec),
			EffectiveUID: r.Eproc.Ucred.Uid, ProcessGroupID: int(r.Eproc.Pgid),
		})
	}
	return ids, nil
}
