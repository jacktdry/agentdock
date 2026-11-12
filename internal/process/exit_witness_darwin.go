//go:build darwin

package process

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/sys/unix"
)

const exitWitnessPollingNS = int64(100 * 1000 * 1000) // only to honor cancellation

type darwinExitWitness struct {
	mu     sync.Mutex
	fd     int
	pid    int
	closed bool
}

func openExitWitness(ctx context.Context, expected BirthIdentity) (exitWitnessPlatform, error) {
	// Look before AND after registration, to reject PID reuse during setup.
	before, err := ObserveBirthIdentity(ctx, expected.PID)
	if err != nil || before != expected {
		return nil, ErrExitWitnessUnavailable
	}
	fd, err := unix.Kqueue()
	if err != nil {
		return nil, ErrExitWitnessUnavailable
	}
	ok := false
	defer func() {
		if !ok {
			_ = unix.Close(fd)
		}
	}()
	unix.CloseOnExec(fd)
	change := unix.Kevent_t{}
	unix.SetKevent(&change, expected.PID, unix.EVFILT_PROC, unix.EV_ADD|unix.EV_ENABLE|unix.EV_ONESHOT)
	change.Fflags = unix.NOTE_EXIT
	if _, err = unix.Kevent(fd, []unix.Kevent_t{change}, nil, nil); err != nil {
		return nil, ErrExitWitnessUnavailable
	}
	after, err := ObserveBirthIdentity(ctx, expected.PID)
	if err != nil || after != expected {
		return nil, ErrExitWitnessUnavailable
	}
	ok = true
	return &darwinExitWitness{fd: fd, pid: expected.PID}, nil
}

func (w *darwinExitWitness) wait(ctx context.Context) error {
	if ctx == nil {
		return ErrExitWitnessUnavailable
	}
	// One Wait owner. A caller must cancel & wait for it before closing FD.
	events := make([]unix.Kevent_t, 1)
	timeout := unix.Timespec{Sec: 0, Nsec: exitWitnessPollingNS}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		count, err := unix.Kevent(w.fd, nil, events, &timeout)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return ErrExitWitnessUnavailable
		}
		if count == 0 {
			continue
		}
		ev := events[0]
		if ev.Filter != unix.EVFILT_PROC || ev.Ident != uint64(w.pid) ||
			ev.Flags&unix.EV_ERROR != 0 || ev.Fflags&unix.NOTE_EXIT == 0 {
			return ErrExitWitnessUnavailable
		}
		return nil
	}
}

func (w *darwinExitWitness) close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if err := unix.Close(w.fd); err != nil {
		return err
	}
	return nil
}
