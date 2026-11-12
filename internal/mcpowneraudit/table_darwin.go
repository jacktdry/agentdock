//go:build darwin

package mcpowneraudit

import (
	"bytes"
	"context"
	"github.com/uvwt/agentdock/internal/process"
	"golang.org/x/sys/unix"
	"unsafe"
)

const available = true

func readTable(ctx context.Context) ([]row, error) {
	if ctx.Err() != nil {
		return nil, errUnavailable
	}
	// Fixed-size, single kernel call: no unbounded allocation/retry when the
	// global table grows. ENOMEM fails closed rather than publishing a prefix.
	// Darwin sys/sysctl.h: KERN_PROC=14, KERN_PROC_ALL=0 (not exported by x/sys).
	mib := [...]int32{unix.CTL_KERN, 14, 0}
	ks := make([]unix.KinfoProc, MaxTableRows)
	n := uintptr(len(ks) * unix.SizeofKinfoProc)
	_, _, errno := unix.Syscall6(unix.SYS___SYSCTL, uintptr(unsafe.Pointer(&mib[0])), uintptr(len(mib)), uintptr(unsafe.Pointer(&ks[0])), uintptr(unsafe.Pointer(&n)), 0, 0)
	if errno != 0 || ctx.Err() != nil || n > uintptr(len(ks)*unix.SizeofKinfoProc) || n%unix.SizeofKinfoProc != 0 {
		return nil, errUnavailable
	}
	ks = ks[:n/unix.SizeofKinfoProc]
	rows := make([]row, 0, len(ks))
	for _, k := range ks {
		// PID 0 (kernel task) is not a userspace process identity.
		if k.Proc.P_pid == 0 {
			continue
		}
		comm := k.Proc.P_comm[:]
		if i := bytes.IndexByte(comm, 0); i >= 0 {
			comm = comm[:i]
		}
		rows = append(rows, row{process.BirthIdentity{
			PID: int(k.Proc.P_pid), ParentPID: int(k.Eproc.Ppid), StartSec: k.Proc.P_starttime.Sec,
			StartUsec: int64(k.Proc.P_starttime.Usec), EffectiveUID: k.Eproc.Ucred.Uid, ProcessGroupID: int(k.Eproc.Pgid),
		}, string(comm)})
	}
	return rows, nil
}
