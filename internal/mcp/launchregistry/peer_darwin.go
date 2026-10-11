//go:build darwin

package launchregistry

import (
	"context"
	"github.com/uvwt/agentdock/internal/process"
	"golang.org/x/sys/unix"
	"net"
	"os"
)

func verifyRegistrarPeer(ctx context.Context, conn *net.UnixConn, expected process.BirthIdentity) bool {
	if expected.PID <= 0 || expected.EffectiveUID != uint32(os.Geteuid()) {
		return false
	}
	var pid int
	var uid uint32
	var sockErr error
	raw, err := conn.SyscallConn()
	if err != nil {
		return false
	}
	if err = raw.Control(func(fd uintptr) {
		pid, sockErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		if sockErr != nil {
			return
		}
		cred, e := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if e != nil {
			sockErr = e
			return
		}
		uid = cred.Uid
	}); err != nil || sockErr != nil || pid != expected.PID || uid != expected.EffectiveUID {
		return false
	}
	current, err := process.ObserveBirthIdentity(ctx, pid)
	return err == nil && current == expected
}
