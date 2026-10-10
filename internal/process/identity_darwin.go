//go:build darwin

package process

import "golang.org/x/sys/unix"

// kern.proc.pid returns the kernel's process creation timestamp at subsecond
// resolution. This is a non-atomic observation, not an ownership handle.
func observeBirthIdentity(pid int) (BirthIdentity, error) {
	k, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || k == nil || k.Proc.P_pid != int32(pid) {
		return BirthIdentity{}, ErrIdentityUnavailable
	}
	return BirthIdentity{
		PID: pid, ParentPID: int(k.Eproc.Ppid), StartSec: k.Proc.P_starttime.Sec,
		StartUsec:    int64(k.Proc.P_starttime.Usec),
		EffectiveUID: k.Eproc.Ucred.Uid, ProcessGroupID: int(k.Eproc.Pgid),
	}, nil
}
