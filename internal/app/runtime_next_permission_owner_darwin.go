//go:build darwin

package app

import (
	"golang.org/x/sys/unix"
	"os"
	"sync"
)

// One permission writer per owner-private Next home. This lifetime fence is
// installed BEFORE permission.NewStore can recover or mutate state.json.
// It does not claim compatibility with older Next builds that do not honor it.
const nextPermissionOwnerLockName = "next-permission-authority.lock"

type nextPermissionOwner struct {
	mu        sync.Mutex
	dirfd, fd int
	closed    bool
}

func acquireNextPermissionOwner(root string) (*nextPermissionOwner, error) {
	if err := os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return nil, errNextCodexRegistryUnavailable
	}
	dirfd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errNextCodexRegistryUnavailable
	}
	var d unix.Stat_t
	if unix.Fstat(dirfd, &d) != nil || d.Mode&unix.S_IFMT != unix.S_IFDIR || d.Mode&07777 != 0700 || d.Uid != uint32(os.Geteuid()) {
		_ = unix.Close(dirfd)
		return nil, errNextCodexRegistryUnavailable
	}
	fd, err := unix.Openat(dirfd, nextPermissionOwnerLockName, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if err != nil {
		_ = unix.Close(dirfd)
		return nil, errNextCodexRegistryUnavailable
	}
	l := &nextPermissionOwner{dirfd: dirfd, fd: fd}
	var file, current unix.Stat_t
	if unix.Fstat(fd, &file) != nil || file.Mode&unix.S_IFMT != unix.S_IFREG || file.Mode&07777 != 0600 || file.Uid != uint32(os.Geteuid()) || file.Nlink != 1 || unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil || unix.Fstatat(dirfd, nextPermissionOwnerLockName, &current, unix.AT_SYMLINK_NOFOLLOW) != nil || current.Dev != file.Dev || current.Ino != file.Ino {
		_ = l.close()
		return nil, errNextCodexRegistryUnavailable
	}
	return l, nil
}
func (l *nextPermissionOwner) close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	first := unix.Close(l.fd)
	second := unix.Close(l.dirfd)
	if first != nil {
		return first
	}
	return second
}
