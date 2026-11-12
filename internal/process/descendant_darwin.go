//go:build darwin

package process

import "golang.org/x/sys/unix"

// kern.proc.uid reveals kernel metadata for same-UID processes, never argv,
// paths, profiles or sockets. Non-descendant rows are never returned to caller.
func uidProcessBirths(uid uint32) ([]BirthIdentity, error) {
	rows, err := unix.SysctlKinfoProcSlice("kern.proc.uid", int(uid))
	if err != nil || len(rows) > maxUIDProcessSample {
		return nil, ErrDescendantObservationUnavailable
	}
	out := make([]BirthIdentity, 0, len(rows))
	for _, r := range rows {
		out = append(out, BirthIdentity{
			PID:            int(r.Proc.P_pid),
			ParentPID:      int(r.Eproc.Ppid),
			StartSec:       r.Proc.P_starttime.Sec,
			StartUsec:      int64(r.Proc.P_starttime.Usec),
			EffectiveUID:   r.Eproc.Ucred.Uid,
			ProcessGroupID: int(r.Eproc.Pgid),
		})
	}
	return out, nil
}
