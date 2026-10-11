//go:build darwin

package app

import (
	"errors"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// One native Codex generation per Next root, regardless of server/config
// revision. Never unlink the lock: contenders must use the same inode.
const nextCodexStartupLockName = "next-codex-native-generation.lock"

type nextCodexStartupLock struct {
	mu              sync.Mutex
	root            string
	dirfd, fd       int
	directory, file unix.Stat_t
	closed          bool
}

func acquireNextCodexStartupLock(root string) (*nextCodexStartupLock, error) {
	dirfd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errNextCodexRegistryUnavailable
	}
	var directory unix.Stat_t
	if unix.Fstat(dirfd, &directory) != nil || directory.Mode&unix.S_IFMT != unix.S_IFDIR || directory.Mode&07777 != 0700 || directory.Uid != uint32(os.Geteuid()) {
		_ = unix.Close(dirfd)
		return nil, errNextCodexRegistryUnavailable
	}
	fd, err := unix.Openat(dirfd, nextCodexStartupLockName, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if err != nil {
		_ = unix.Close(dirfd)
		return nil, errNextCodexRegistryUnavailable
	}
	g := &nextCodexStartupLock{root: root, dirfd: dirfd, fd: fd, directory: directory}
	if unix.Fstat(fd, &g.file) != nil || g.validate() != nil || unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil || g.validate() != nil {
		_ = g.close()
		return nil, errNextCodexRegistryUnavailable
	}
	// A previous Core may have crashed after starting its native app-server.
	// flock alone is not evidence that any surviving child MCPs have exited.
	// Nonempty journal is an unresolved generation, including a truncated or
	// corrupt record. Never auto-delete it or start a replacement native Core.
	var state unix.Stat_t
	if unix.Fstat(fd, &state) != nil || state.Size != 0 {
		_ = g.close()
		return nil, errNextCodexRegistryUnavailable
	}
	return g, nil
}

// markBootstrapAttempt is durable BEFORE any native Adapter process starts.
// The marker intentionally contains no PID, credential, path or MCP args.
// Even a partial write after a crash leaves a nonempty fail-closed sentinel.
// The source-only Core deliberately has NO method to clear this marker:
// reconciling possible orphan native children needs separate operator proof.
func (g *nextCodexStartupLock) markBootstrapAttempt() error {
	if g == nil || g.validate() != nil {
		return errNextCodexRegistryUnavailable
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return errNextCodexRegistryUnavailable
	}
	const marker = "agentdock-next-codex-native-bootstrap-unresolved-v1\n"
	if err := unix.Ftruncate(g.fd, 0); err != nil {
		return errNextCodexRegistryUnavailable
	}
	if _, err := unix.Seek(g.fd, 0, io.SeekStart); err != nil {
		return errNextCodexRegistryUnavailable
	}
	if n, err := unix.Write(g.fd, []byte(marker)); err != nil || n != len(marker) {
		return errNextCodexRegistryUnavailable
	}
	if err := unix.Fsync(g.fd); err != nil {
		return errNextCodexRegistryUnavailable
	}
	// Also persist the directory entry for a newly created lock file.
	// Otherwise a sudden crash can lose the pathname but not its journal.
	if err := unix.Fsync(g.dirfd); err != nil {
		return errNextCodexRegistryUnavailable
	}
	return nil
}

func (g *nextCodexStartupLock) validate() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return errNextCodexRegistryUnavailable
	}
	var directory, file, current unix.Stat_t
	if unix.Lstat(g.root, &directory) != nil || directory.Dev != g.directory.Dev || directory.Ino != g.directory.Ino || directory.Mode&unix.S_IFMT != unix.S_IFDIR || directory.Mode&07777 != 0700 || directory.Uid != uint32(os.Geteuid()) ||
		unix.Fstat(g.fd, &file) != nil || file.Mode&unix.S_IFMT != unix.S_IFREG || file.Mode&07777 != 0600 || file.Uid != uint32(os.Geteuid()) || file.Nlink != 1 || file.Dev != g.file.Dev || file.Ino != g.file.Ino ||
		unix.Fstatat(g.dirfd, nextCodexStartupLockName, &current, unix.AT_SYMLINK_NOFOLLOW) != nil || current.Dev != file.Dev || current.Ino != file.Ino {
		return errNextCodexRegistryUnavailable
	}
	return nil
}

func (g *nextCodexStartupLock) close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil
	}
	g.closed = true
	return errors.Join(unix.Close(g.fd), unix.Close(g.dirfd))
}
