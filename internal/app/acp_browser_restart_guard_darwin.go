//go:build darwin

package app

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	processcontrol "github.com/uvwt/agentdock/internal/process"

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

	// Next-only kernel NOTE_EXIT observers are live only while this Core lives.
	// Events cannot authorize unlocking or clearing the durable marker.
	closing       bool
	exitWatches   []sharedExitWatch
	exitObserved  int
	exitUncertain bool
	// Set only by the Next opted-in source constructor before workers run.
	// Ready may not publish unless matching durable spawn evidence exists.
	requireStartedEvidence bool
	// Private test seam; production 0 uses 15s. Only configured prior to Ready.
	descendantRefreshInterval time.Duration
}

type sharedExitWatch struct {
	cancel      context.CancelFunc
	done        <-chan struct{}
	monitorDone <-chan struct{}
}

// A fixed conservative interval avoids busy process enumeration or expensive
// polling when no Chrome descendants change.
const sharedDescendantRefreshInterval = 15 * time.Second

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
	// Stop and join all Core-lifetime watchers before final marker checks.
	// No watcher may race a closed guard dirfd or be kept alive after Close.
	g.stopExitWatches()
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
	// Missing or ambiguous OS events cannot certify verified shutdown.
	// Even a positive wrapper exit does not attest detached Chrome children.
	if g.exitUncertain {
		return errSharedBrowserRestartGuard
	}
	var marker unix.Stat_t
	if unix.Fstatat(g.dirfd, sharedBrowserRestartMarker, &marker, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		uint64(marker.Dev) != g.device || marker.Ino != g.inode ||
		marker.Mode&unix.S_IFMT != unix.S_IFREG ||
		marker.Uid != uint32(os.Geteuid()) || marker.Mode&0077 != 0 {
		return errSharedBrowserRestartGuard
	}
	// A same-inode corrupted/partially written witness is not a clean release
	// certificate. Read under the pinned directory and refuse to unlink it.
	fd, openErr := unix.Openat(g.dirfd, sharedBrowserRestartMarker, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if openErr != nil {
		return errSharedBrowserRestartGuard
	}
	var verified unix.Stat_t
	verifyErr := unix.Fstat(fd, &verified)
	valid := verifyErr == nil && uint64(verified.Dev) == g.device && verified.Ino == g.inode &&
		verified.Size >= int64(len(sharedBrowserEvidencePrefix)) && verified.Size <= sharedBrowserEvidenceMaxBytes
	if valid {
		buf := make([]byte, verified.Size)
		n, readErr := unix.Pread(fd, buf, 0)
		if readErr != nil || n != len(buf) {
			valid = false
		} else {
			_, valid = parseSharedBrowserEvidence(string(buf))
		}
	}
	_ = unix.Close(fd)
	if !valid {
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

// watchReadyExit attaches a live, identity-checked kqueue registration after
// journal persistence but before WorkerInfo publication. It never grants
// cleanup authority. Caller keeps ownership on attachment failure.
func (g *sharedBrowserRestartGuard) watchReadyExit(watch *processcontrol.ExitWitness, root processcontrol.BirthIdentity) error {
	if g == nil || watch == nil || !validSharedBrowserIdentity(root) {
		return errSharedBrowserEvidence
	}
	g.mu.Lock()
	if g.released || g.closing {
		g.mu.Unlock()
		return errSharedBrowserEvidence
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	monitorDone := make(chan struct{})
	g.exitWatches = append(g.exitWatches, sharedExitWatch{cancel: cancel, done: done, monitorDone: monitorDone})
	g.mu.Unlock()
	go func() {
		defer close(done)
		err := watch.Wait(ctx)
		closeErr := watch.Close()
		g.mu.Lock()
		switch {
		case err == nil && closeErr == nil:
			g.exitObserved++
		case ctx.Err() == nil:
			g.exitUncertain = true
		}
		g.mu.Unlock()
		// Wrapper exit also ends this worker's periodic witness sampler.
		cancel()
	}()
	go func() {
		defer close(monitorDone)
		interval := g.descendantRefreshInterval
		if interval <= 0 {
			interval = sharedDescendantRefreshInterval
		}
		tick := time.NewTicker(interval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if ctx.Err() != nil {
					return
				}
				if err := g.refreshReadyDescendants(ctx, root); err != nil && ctx.Err() == nil {
					g.mu.Lock()
					g.exitUncertain = true // A durable observation gap blocks clean auto-unlink.
					g.mu.Unlock()
					return
				}
			}
		}
	}()
	return nil
}

func (g *sharedBrowserRestartGuard) stopExitWatches() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.closing = true
	watches := append([]sharedExitWatch(nil), g.exitWatches...)
	for _, w := range watches {
		w.cancel()
	}
	g.mu.Unlock()
	for _, w := range watches {
		<-w.done
		if w.monitorDone != nil {
			<-w.monitorDone
		}
	}
}
