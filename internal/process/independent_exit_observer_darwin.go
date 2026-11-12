//go:build darwin

package process

import (
	"context"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	independentExitReceiptPrefix = "agentdock-next-independent-exit-v1"
	maxIndependentReceiptName    = 64
)

// WatchIndependentWorkerExit can run in a separate process from Core. The
// caller is responsible for launching that process in an independent session.
// Arm acknowledgement occurs only AFTER a private durable receipt is created
// AND a birth-bound kqueue NOTE_EXIT watch has been installed. No signals.
func WatchIndependentWorkerExit(ctx context.Context, req IndependentExitRequest, armed func() error) error {
	if ctx == nil || ctx.Err() != nil || armed == nil || !validIndependentReceiptName(req.ReceiptName) ||
		req.Core.PID <= 0 || req.Worker.PID <= 0 || req.Core.PID == req.Worker.PID ||
		req.Worker.ParentPID != req.Core.PID {
		return ErrIndependentExitObserverUnavailable
	}
	core, err := ObserveBirthIdentity(ctx, req.Core.PID)
	if err != nil || core != req.Core {
		return ErrIndependentExitObserverUnavailable
	}
	child, err := ObserveDirectChildBirth(ctx, req.Core.PID, req.Worker.PID)
	if err != nil || child != req.Worker {
		return ErrIndependentExitObserverUnavailable
	}
	watch, err := WatchVerifiedExit(ctx, child)
	if err != nil {
		return ErrIndependentExitObserverUnavailable
	}
	defer watch.Close()

	dir, err := unix.Open(req.StateDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return ErrIndependentExitObserverUnavailable
	}
	defer unix.Close(dir)
	var dirStat unix.Stat_t
	if unix.Fstat(dir, &dirStat) != nil || dirStat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		dirStat.Uid != uint32(os.Geteuid()) || dirStat.Mode&0077 != 0 {
		return ErrIndependentExitObserverUnavailable
	}
	fd, err := unix.Openat(dir, req.ReceiptName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return ErrIndependentExitObserverUnavailable
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG ||
		st.Uid != uint32(os.Geteuid()) || st.Mode&0077 != 0 {
		return ErrIndependentExitObserverUnavailable
	}
	start := fmt.Sprintf("%s\ncore_pid=%d,core_start_sec=%d,core_start_usec=%d,worker_pid=%d,worker_start_sec=%d,worker_start_usec=%d,worker_uid=%d,worker_pgid=%d\nstate=armed\n",
		independentExitReceiptPrefix, core.PID, core.StartSec, core.StartUsec,
		child.PID, child.StartSec, child.StartUsec, child.EffectiveUID, child.ProcessGroupID)
	if !writeDurableIndependentEvidence(fd, dir, []byte(start)) {
		return ErrIndependentExitObserverUnavailable
	}
	if err := armed(); err != nil {
		return ErrIndependentExitObserverUnavailable
	}
	if err := watch.Wait(ctx); err != nil {
		return ErrIndependentExitObserverUnavailable
	}
	// A renamed/replaced/symlinked receipt is not the file that was armed.
	// Its OS event must never be attributed to a different inode.
	var current, pinned unix.Stat_t
	if unix.Fstatat(dir, req.ReceiptName, &current, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		unix.Fstat(fd, &pinned) != nil ||
		current.Dev != st.Dev || current.Ino != st.Ino ||
		pinned.Dev != st.Dev || pinned.Ino != st.Ino ||
		current.Mode&unix.S_IFMT != unix.S_IFREG ||
		current.Uid != uint32(os.Geteuid()) || current.Mode&0077 != 0 {
		return ErrIndependentExitObserverUnavailable
	}
	if !writeDurableIndependentEvidence(fd, dir, []byte("state=wrapper_exit_observed\n")) {
		return ErrIndependentExitObserverUnavailable
	}
	return nil
}

func validIndependentReceiptName(name string) bool {
	if len(name) < len("observer-x.v1") || len(name) > maxIndependentReceiptName ||
		!strings.HasPrefix(name, "observer-") || !strings.HasSuffix(name, ".v1") {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return !strings.Contains(name, "..")
}

func writeDurableIndependentEvidence(fd, dir int, data []byte) bool {
	count, err := unix.Write(fd, data)
	return err == nil && count == len(data) && unix.Fsync(fd) == nil && unix.Fsync(dir) == nil
}
