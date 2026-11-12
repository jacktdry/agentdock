//go:build darwin

package launchregistry

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/uvwt/agentdock/internal/process"
)

const maxCoreRegistrationClients = 4
const maxCoreRegistrationRows = 64

// TrustedScope is supplied by a Next Core callback that has already pinned
// the exact managed ACP Adapter and its dedicated Codex app-server. The Core
// listener re-observes the entire chain; a claimed PID is not authority.
// These identities are neither a Codex native Thread mapping nor a kill lease.
type TrustedScope struct {
	Adapter   process.BirthIdentity
	AppServer process.BirthIdentity
}

type ScopeProvider func(context.Context) (TrustedScope, error)

// PrivateCoreListener is an opt-in *source-only* Next Core registrar candidate.
// It owns only its temporary Unix socket and in-memory bounded diagnostic rows.
// There is intentionally no Runtime constructor/HTTP/Desktop hookup.
type PrivateCoreListener struct {
	listener       *net.UnixListener
	path, dir      string
	socketFile     os.FileInfo
	provider       ScopeProvider
	server, digest string
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	slots          chan struct{}
	mu             sync.Mutex
	closed         bool
	records        []Observation
}

func StartPrivateCoreListener(parentDir string, provider ScopeProvider, serverID, configDigest string) (*PrivateCoreListener, error) {
	if provider == nil || !validServer(serverID) || !validDigest(configDigest) || parentDir == "" {
		return nil, ErrUnavailable
	}
	// Accept only an existing private, non-symlink, same-user directory. Never
	// unlink/replace any preexisting socket or a foreign runtime directory.
	info, err := os.Lstat(parentDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, ErrUnavailable
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) {
		return nil, ErrUnavailable
	}
	dir, err := os.MkdirTemp(parentDir, "next-mcp-")
	if err != nil {
		return nil, ErrUnavailable
	}
	path := filepath.Join(dir, "register.sock")
	addr := &net.UnixAddr{Name: path, Net: "unix"}
	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		_ = os.Remove(dir)
		return nil, ErrUnavailable
	}
	l.SetUnlinkOnClose(false)
	if os.Chmod(path, 0600) != nil {
		_ = l.Close()
		_ = os.Remove(path)
		_ = os.Remove(dir)
		return nil, ErrUnavailable
	}
	sock, err := os.Lstat(path)
	if err != nil || sock.Mode()&os.ModeSocket == 0 {
		_ = l.Close()
		_ = os.Remove(path)
		_ = os.Remove(dir)
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithCancel(context.Background())
	svc := &PrivateCoreListener{listener: l, path: path, dir: dir, socketFile: sock, provider: provider, server: serverID, digest: configDigest, ctx: ctx, cancel: cancel, slots: make(chan struct{}, maxCoreRegistrationClients)}
	svc.wg.Add(1)
	go svc.acceptLoop()
	return svc, nil
}

func (s *PrivateCoreListener) SocketPath() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *PrivateCoreListener) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.AcceptUnix()
		if err != nil {
			return
		}
		select {
		case s.slots <- struct{}{}:
			s.wg.Add(1)
			go func() { defer s.wg.Done(); defer func() { <-s.slots }(); defer conn.Close(); s.register(conn) }()
		default:
			_ = conn.Close()
		}
	}
}

func (s *PrivateCoreListener) register(conn *net.UnixConn) {
	ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
	defer cancel()
	before, err := s.verifiedScope(ctx)
	if err != nil {
		return
	}
	_, _ = (Registrar{AppServer: before.AppServer, Server: s.server, ConfigDigest: s.digest}).AcceptWithCommit(ctx, conn, func(record Observation) error {
		after, err := s.verifiedScope(ctx)
		if err != nil || after != before || record.AppServer != before.AppServer {
			return ErrUnavailable
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed || ctx.Err() != nil {
			return ErrUnavailable
		}
		if len(s.records) >= maxCoreRegistrationRows {
			copy(s.records, s.records[1:])
			s.records[len(s.records)-1] = record
		} else {
			s.records = append(s.records, record)
		}
		return nil
	})
}

func (s *PrivateCoreListener) verifiedScope(ctx context.Context) (TrustedScope, error) {
	scope, err := s.provider(ctx)
	if err != nil || ctx.Err() != nil || scope.Adapter.PID <= 0 || scope.AppServer.PID <= 0 || scope.Adapter.PID == scope.AppServer.PID || scope.Adapter.PID == os.Getpid() {
		return TrustedScope{}, ErrUnavailable
	}
	// Repeated kernel checks prove Core→Adapter→app-server at this instant.
	adapter, err := process.ObserveBirthIdentity(ctx, scope.Adapter.PID)
	if err != nil || adapter != scope.Adapter || adapter.ParentPID != os.Getpid() || adapter.EffectiveUID != uint32(os.Geteuid()) {
		return TrustedScope{}, ErrUnavailable
	}
	native, err := process.ObserveBirthIdentity(ctx, scope.AppServer.PID)
	if err != nil || native != scope.AppServer || native.ParentPID != adapter.PID || native.EffectiveUID != adapter.EffectiveUID {
		return TrustedScope{}, ErrUnavailable
	}
	recheck, err := process.ObserveBirthIdentity(ctx, adapter.PID)
	if err != nil || recheck != adapter {
		return TrustedScope{}, ErrUnavailable
	}
	return scope, nil
}

// Snapshot returns only read-only diagnostic rows. Rows may refer to child
// processes that have since exited. No cleanup operation or PID controls exist.
func (s *PrivateCoreListener) Snapshot() []Observation {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Observation(nil), s.records...)
}

func (s *PrivateCoreListener) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	err := s.listener.Close()
	s.wg.Wait()
	// Only unlink the exact socket file created by this listener. Replacements
	// remain untouched even when they prevent removing our private directory.
	actual, statErr := os.Lstat(s.path)
	if statErr == nil && os.SameFile(actual, s.socketFile) {
		err = errors.Join(err, os.Remove(s.path))
	}
	err = errors.Join(err, os.Remove(s.dir))
	return err
}
