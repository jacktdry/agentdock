//go:build darwin

package app

import (
	"errors"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// A sticky record, not a PID and not authority to kill any process. It is
// created BEFORE a shared browser may launch and remains after Core crash,
// startup uncertainty, or unsuccessful cleanup. Removal needs clean Bridge Close.
type sharedBrowserRestartGuard struct {
	mu         sync.Mutex
	dirfd      int
	device     uint64
	inode      uint64
	released   bool
	releaseErr error
}

var errSharedBrowserRestartGuard = errors.New("Next shared browser restart reconciliation required")

func acquireSharedBrowserRestartGuard(root string) (*sharedBrowserRestartGuard, error) {
	st, err := os.Lstat(root)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0077 != 0 {
		return nil, errSharedBrowserRestartGuard
	}
	dirfd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errSharedBrowserRestartGuard
	}
	ok := false
	defer func() {
		if !ok {
			_ = unix.Close(dirfd)
		}
	}()
	var dir unix.Stat_t
	if unix.Fstat(dirfd, &dir) != nil || dir.Uid != uint32(os.Geteuid()) || dir.Mode&0077 != 0 || dir.Mode&unix.S_IFMT != unix.S_IFDIR {
		return nil, errSharedBrowserRestartGuard
	}
	fd, err := unix.Openat(dirfd, sharedBrowserRestartMarker, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, errSharedBrowserRestartGuard
	}
	// Never remove a marker after a failed durability step: a Core crash may
	// have begun startup and no evidence proves it was side-effect free.
	var marker unix.Stat_t
	statErr := unix.Fstat(fd, &marker)
	writeErr := error(nil)
	if statErr == nil {
		var n int
		n, writeErr = unix.Write(fd, []byte("agentdock-next-shared-browser-pending-v1\n"))
		if writeErr == nil && n != len("agentdock-next-shared-browser-pending-v1\n") {
			writeErr = unix.EIO
		}
	}
	syncErr := unix.Fsync(fd)
	closeErr := unix.Close(fd)
	if statErr != nil || writeErr != nil || syncErr != nil || closeErr != nil || unix.Fsync(dirfd) != nil {
		return nil, errSharedBrowserRestartGuard
	}
	if marker.Mode&unix.S_IFMT != unix.S_IFREG || marker.Uid != uint32(os.Geteuid()) || marker.Mode&0077 != 0 {
		return nil, errSharedBrowserRestartGuard
	}
	ok = true
	return &sharedBrowserRestartGuard{dirfd: dirfd, device: uint64(marker.Dev), inode: marker.Ino}, nil
}

func (g *sharedBrowserRestartGuard) releaseAfterVerifiedClose() (err error) {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.released {
		return g.releaseErr
	}
	defer func() {
		_ = unix.Close(g.dirfd)
		g.released = true
		g.releaseErr = err
	}()
	var marker unix.Stat_t
	if unix.Fstatat(g.dirfd, sharedBrowserRestartMarker, &marker, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		uint64(marker.Dev) != g.device || marker.Ino != g.inode ||
		marker.Mode&unix.S_IFMT != unix.S_IFREG ||
		marker.Uid != uint32(os.Geteuid()) || marker.Mode&0077 != 0 {
		return errSharedBrowserRestartGuard
	}
	if unix.Unlinkat(g.dirfd, sharedBrowserRestartMarker, 0) != nil {
		return errSharedBrowserRestartGuard
	}
	if unix.Fsync(g.dirfd) != nil {
		return errSharedBrowserRestartGuard
	}
	return nil
}
