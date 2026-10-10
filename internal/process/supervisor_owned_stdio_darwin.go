//go:build darwin

package process

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// SupervisorOwnedStdio is an UNWIRED source-only prototype with an OS-owned
// command handle and stdio pipes retained in the independent Supervisor.
// No browser, installed Next runtime, or user profile uses this implementation.
// Its public API deliberately provides NO termination or group kill operation.
type SupervisorOwnedStdio struct {
	mu               sync.Mutex
	cmd              *exec.Cmd
	birth            BirthIdentity
	stdin            io.WriteCloser
	stdout           io.ReadCloser
	exit             *ExitWitness
	waiting          bool
	finished         bool
	transportClaimed bool
	waitErr          error
}

// StartSupervisorOwnedStdio starts ONLY the command supplied by its local
// caller, as this Supervisor process's direct child. Rejected startup may
// terminate only its just-started, not-yet-reaped owned child, never a PID
// looked up from a durable journal or a process group shared with users.
func StartSupervisorOwnedStdio(ctx context.Context, cmd *exec.Cmd) (*SupervisorOwnedStdio, error) {
	if ctx == nil || ctx.Err() != nil || cmd == nil || cmd.Path == "" || cmd.Process != nil ||
		cmd.SysProcAttr != nil || cmd.Stdin != nil || cmd.Stdout != nil || cmd.Stderr != nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, ErrSupervisorHandoffUnavailable
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, ErrSupervisorHandoffUnavailable
	}
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	Configure(cmd)
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, ErrSupervisorHandoffUnavailable
	}
	cleanup := func() {
		// Until cmd.Wait, the child PID cannot be reused; never signal a process
		// group or any process discovered from a UID-wide snapshot.
		_ = cmd.Process.Kill()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = cmd.Wait()
	}
	birth, err := ObserveDirectChildBirth(ctx, os.Getpid(), cmd.Process.Pid)
	if err != nil {
		cleanup()
		return nil, ErrSupervisorHandoffUnavailable
	}
	exit, err := WatchVerifiedExit(ctx, birth)
	if err != nil {
		cleanup()
		return nil, ErrSupervisorHandoffUnavailable
	}
	return &SupervisorOwnedStdio{cmd: cmd, birth: birth, stdin: stdin, stdout: stdout, exit: exit}, nil
}

func (s *SupervisorOwnedStdio) Birth() BirthIdentity {
	if s == nil {
		return BirthIdentity{}
	}
	return s.birth
}

func (s *SupervisorOwnedStdio) Stdin() io.WriteCloser {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transportClaimed || s.waiting || s.finished {
		return nil
	}
	return s.stdin
}
func (s *SupervisorOwnedStdio) Stdout() io.ReadCloser {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transportClaimed || s.waiting || s.finished {
		return nil
	}
	return s.stdout
}

// ClaimMCPTransport returns the Supervisor's OS pipe handles exactly once.
// It performs no spawning or PID lookup. The caller MUST own this wrapper in
// the Supervisor process; Core never receives these pipes or this object.
// After a claim, SDK owns the pipe usage; wrapper Wait still owns reaping.
func (s *SupervisorOwnedStdio) ClaimMCPTransport() (io.ReadCloser, io.WriteCloser, error) {
	if s == nil {
		return nil, nil, ErrSupervisorHandoffUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transportClaimed || s.waiting || s.finished ||
		s.stdin == nil || s.stdout == nil || s.cmd == nil || s.cmd.Process == nil {
		return nil, nil, ErrSupervisorHandoffUnavailable
	}
	s.transportClaimed = true
	return s.stdout, s.stdin, nil
}

// Wait witnesses this exact owned wrapper's kernel NOTE_EXIT then calls
// cmd.Wait to reap it. The caller must close its stdin to request graceful
// shutdown. No PIDs or process groups are signaled here.
func (s *SupervisorOwnedStdio) Wait(ctx context.Context) error {
	if s == nil || ctx == nil {
		return ErrSupervisorHandoffUnavailable
	}
	s.mu.Lock()
	if s.finished {
		err := s.waitErr
		s.mu.Unlock()
		return err
	}
	if s.waiting {
		s.mu.Unlock()
		return ErrSupervisorHandoffUnavailable
	}
	s.waiting = true
	s.mu.Unlock()
	if err := s.exit.Wait(ctx); err != nil {
		s.mu.Lock()
		s.waiting = false
		s.mu.Unlock()
		return errors.Join(ErrSupervisorHandoffUnavailable, err)
	}
	waitErr := s.cmd.Wait()
	closeErr := s.exit.Close()
	s.mu.Lock()
	s.waitErr = errors.Join(waitErr, closeErr)
	s.finished = true
	s.waiting = false
	s.mu.Unlock()
	return s.waitErr
}
