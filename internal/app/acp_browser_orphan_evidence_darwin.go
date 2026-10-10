//go:build darwin

package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	processcontrol "github.com/uvwt/agentdock/internal/process"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
	"golang.org/x/sys/unix"
)

const (
	sharedBrowserEvidencePrefix   = "agentdock-next-shared-browser-pending-v1\n"
	sharedBrowserEvidenceMaxBytes = 16384
	sharedBrowserEvidenceMaxPIDs  = 64
)

var errSharedBrowserEvidence = errors.New("Next shared browser worker evidence unavailable")

// recordManagedWorker is invoked by the ONLY Next-shared WorkerRegistry after
// the handshake/schema checks, before WorkerInfo can reach the lease layer.
// PID is recorded for future human/forensic observation, NOT to authorize kill,
// signal, unlink, profile control or automatic crash recovery.
func (g *sharedBrowserRestartGuard) recordManagedWorker(info toolbrowser.WorkerInfo) error {
	if g == nil || info.State != toolbrowser.WorkerReady || info.Session.PID <= 0 || info.Session.PID > math.MaxInt32 {
		return errSharedBrowserEvidence
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.released {
		return errSharedBrowserEvidence
	}
	fd, err := unix.Openat(g.dirfd, sharedBrowserRestartMarker, unix.O_RDWR|unix.O_APPEND|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return errSharedBrowserEvidence
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil ||
		uint64(stat.Dev) != g.device || stat.Ino != g.inode ||
		stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Uid != uint32(os.Geteuid()) || stat.Mode&0077 != 0 {
		return errSharedBrowserEvidence
	}
	// Refuse appending evidence to a changed or malformed existing journal.
	// Any partial previous write must keep the startup fence, not become ready.
	buf := make([]byte, stat.Size)
	readCount, readErr := unix.Pread(fd, buf, 0)
	if readErr != nil || readCount != len(buf) {
		return errSharedBrowserEvidence
	}
	oldPIDs, valid := parseSharedBrowserEvidence(string(buf))
	if !valid || len(oldPIDs) >= sharedBrowserEvidenceMaxPIDs {
		return errSharedBrowserEvidence
	}
	line := fmt.Sprintf("worker_ready_pid=%d\n", info.Session.PID)
	// A bounded append-only journal preserves earlier generation observations.
	// It does not overwrite an orphan PID with the current generation's PID.
	if stat.Size < int64(len(sharedBrowserEvidencePrefix)) ||
		stat.Size+int64(len(line)) > sharedBrowserEvidenceMaxBytes {
		return errSharedBrowserEvidence
	}
	written, writeErr := unix.Write(fd, []byte(line))
	if writeErr != nil || written != len(line) || unix.Fsync(fd) != nil || unix.Fsync(g.dirfd) != nil {
		// Startup may already have created descendants: keep the restart fence.
		return errSharedBrowserEvidence
	}
	return nil
}

// A deliberately small, privacy-safe classification: no PID, path, workspace,
// profile name, bearer token, argv or device details escape this result.
// Every status is advisory. None authorizes cleanup, auto-restart or unlocking.
type sharedBrowserOrphanInspection struct {
	Status             string
	WorkerObservations int
}

const (
	sharedOrphanNoMarker               = "no_pending_marker"
	sharedOrphanUntrusted              = "untrusted_marker"
	sharedOrphanStartupUnknown         = "startup_unknown"
	sharedOrphanPIDVisible             = "pid_present_identity_unverified"
	sharedOrphanPIDAbsent              = "worker_pid_absent_descendants_unverified"
	sharedOrphanObservationUnavailable = "observation_unavailable"
)

func inspectSharedBrowserOrphanEvidence(ctx context.Context, root string, observe func(context.Context, int) processcontrol.Snapshot) sharedBrowserOrphanInspection {
	untrusted := sharedBrowserOrphanInspection{Status: sharedOrphanUntrusted}
	if ctx == nil || ctx.Err() != nil || observe == nil {
		return sharedBrowserOrphanInspection{Status: sharedOrphanObservationUnavailable}
	}
	state, err := os.Lstat(root)
	if err != nil || !state.IsDir() || state.Mode()&os.ModeSymlink != 0 || state.Mode().Perm()&0077 != 0 {
		return untrusted
	}
	dir, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return untrusted
	}
	defer unix.Close(dir)
	var dirStat unix.Stat_t
	if unix.Fstat(dir, &dirStat) != nil || dirStat.Uid != uint32(os.Geteuid()) ||
		dirStat.Mode&unix.S_IFMT != unix.S_IFDIR || dirStat.Mode&0077 != 0 {
		return untrusted
	}
	fd, err := unix.Openat(dir, sharedBrowserRestartMarker, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return sharedBrowserOrphanInspection{Status: sharedOrphanNoMarker}
	}
	if err != nil {
		return untrusted
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG ||
		st.Uid != uint32(os.Geteuid()) || st.Mode&0077 != 0 ||
		st.Size < int64(len(sharedBrowserEvidencePrefix)) || st.Size > sharedBrowserEvidenceMaxBytes {
		return untrusted
	}
	buf := make([]byte, st.Size)
	n, err := unix.Pread(fd, buf, 0)
	if err != nil || n != len(buf) {
		return sharedBrowserOrphanInspection{Status: sharedOrphanObservationUnavailable}
	}
	pids, valid := parseSharedBrowserEvidence(string(buf))
	if !valid {
		return untrusted
	}
	if len(pids) == 0 {
		return sharedBrowserOrphanInspection{Status: sharedOrphanStartupUnknown}
	}
	result := sharedBrowserOrphanInspection{WorkerObservations: len(pids)}
	if len(pids) > sharedBrowserEvidenceMaxPIDs {
		result.Status = sharedOrphanObservationUnavailable
		return result
	}
	visible := false
	unavailable := false
	seen := make(map[int]bool, len(pids))
	for _, pid := range pids {
		if seen[pid] {
			continue
		}
		seen[pid] = true
		snap := observe(ctx, pid)
		// PID reuse cannot be excluded with this observation. Even an alive PID
		// is NOT a claim that the process is the originally recorded worker.
		switch snap.State {
		case "alive":
			visible = true
		case "zombie", "not_found":
			// Absent wrapper does NOT prove Chrome descendants are absent.
		default:
			unavailable = true
		}
		if ctx.Err() != nil {
			unavailable = true
		}
	}
	switch {
	case unavailable:
		result.Status = sharedOrphanObservationUnavailable
	case visible:
		result.Status = sharedOrphanPIDVisible
	default:
		result.Status = sharedOrphanPIDAbsent
	}
	return result
}

func parseSharedBrowserEvidence(raw string) ([]int, bool) {
	if !strings.HasPrefix(raw, sharedBrowserEvidencePrefix) {
		return nil, false
	}
	rest := strings.TrimPrefix(raw, sharedBrowserEvidencePrefix)
	if rest == "" {
		return nil, true
	}
	if !strings.HasSuffix(rest, "\n") {
		return nil, false
	}
	lines := strings.Split(strings.TrimSuffix(rest, "\n"), "\n")
	if len(lines) > sharedBrowserEvidenceMaxPIDs {
		return nil, false
	}
	pids := make([]int, 0, len(lines))
	for _, line := range lines {
		value, ok := strings.CutPrefix(line, "worker_ready_pid=")
		if !ok || len(value) == 0 || strings.Trim(value, "0123456789") != "" {
			return nil, false
		}
		pid, err := strconv.Atoi(value)
		if err != nil || pid <= 0 || pid > math.MaxInt32 {
			return nil, false
		}
		pids = append(pids, pid)
	}
	return pids, true
}
