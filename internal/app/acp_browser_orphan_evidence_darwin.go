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

// recordStartedWorker is invoked at the owned stdio process Start/Attach
// boundary before MCP negotiation. The sticky guard already exists on disk.
// Failure closes only the just-launched owned child via the stdio transport.
func (g *sharedBrowserRestartGuard) recordStartedWorker(ctx context.Context, pid int) error {
	if g == nil || ctx == nil || ctx.Err() != nil {
		return errSharedBrowserEvidence
	}
	identity, err := processcontrol.ObserveDirectChildBirth(ctx, os.Getpid(), pid)
	if err != nil {
		return errSharedBrowserEvidence
	}
	return g.appendSharedBrowserIdentity(identity, sharedEvidenceStarted)
}

// recordManagedWorker is invoked by the ONLY Next-shared WorkerRegistry after
// the handshake/schema checks, before WorkerInfo can reach the lease layer.
// PID is recorded for future human/forensic observation, NOT to authorize kill,
// signal, unlink, profile control or automatic crash recovery.
func (g *sharedBrowserRestartGuard) recordManagedWorker(info toolbrowser.WorkerInfo) error {
	if g == nil || info.Session.PID <= 0 {
		return errSharedBrowserEvidence
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	identity, err := processcontrol.ObserveDirectChildBirth(ctx, os.Getpid(), info.Session.PID)
	if err != nil {
		return errSharedBrowserEvidence
	}
	watch, err := processcontrol.WatchVerifiedExit(ctx, identity)
	if err != nil {
		return errSharedBrowserEvidence
	}
	// The proof has been checked twice and kernel NOTE_EXIT armed.
	// A persistence or close race must refuse the ready Worker and retain
	// the sticky restart marker, never publish a lease.
	if err := g.recordManagedWorkerWithIdentity(info, func(context.Context, int) (processcontrol.BirthIdentity, error) {
		return identity, nil
	}); err != nil {
		_ = watch.Close()
		return err
	}
	if err := g.watchReadyExit(watch); err != nil {
		_ = watch.Close()
		return err
	}
	return nil
}

func (g *sharedBrowserRestartGuard) recordManagedWorkerWithIdentity(info toolbrowser.WorkerInfo, observe func(context.Context, int) (processcontrol.BirthIdentity, error)) error {
	if g == nil || observe == nil || info.State != toolbrowser.WorkerReady || info.Session.PID <= 0 || info.Session.PID > math.MaxInt32 {
		return errSharedBrowserEvidence
	}
	// Kernel birth timestamp and credentials must be observed while the
	// validated worker is still alive, BEFORE its lease becomes usable.
	// No inference from PID alone and never an authority for SIGKILL.
	identity, identityErr := observe(context.Background(), info.Session.PID)
	if identityErr != nil || !validSharedBrowserIdentity(identity) || identity.PID != info.Session.PID {
		return errSharedBrowserEvidence
	}
	return g.appendSharedBrowserIdentity(identity, sharedEvidenceReady)
}

type sharedBrowserEvidenceStage string

const (
	sharedEvidenceStarted sharedBrowserEvidenceStage = "started"
	sharedEvidenceReady   sharedBrowserEvidenceStage = "ready"
)

func (g *sharedBrowserRestartGuard) appendSharedBrowserIdentity(identity processcontrol.BirthIdentity, stage sharedBrowserEvidenceStage) error {
	if g == nil || !validSharedBrowserIdentity(identity) || (stage != sharedEvidenceStarted && stage != sharedEvidenceReady) {
		return errSharedBrowserEvidence
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.released || g.closing {
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
	if stage == sharedEvidenceReady && g.requireStartedEvidence {
		found := false
		for _, prior := range oldPIDs {
			if prior.PID != identity.PID || prior.Stage != sharedEvidenceStarted {
				continue
			}
			// PID reuse or a changed worker identity is not an attested ready.
			if !prior.matches(identity) {
				return errSharedBrowserEvidence
			}
			found = true
		}
		if !found {
			return errSharedBrowserEvidence
		}
	}
	line := fmt.Sprintf("worker_%s_pid=%d,start_sec=%d,start_usec=%d,uid=%d,pgid=%d\n",
		stage, identity.PID, identity.StartSec, identity.StartUsec, identity.EffectiveUID, identity.ProcessGroupID)
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
	sharedOrphanPreHandshakeObserved   = "worker_started_handshake_unverified"
	sharedOrphanPIDVisible             = "pid_present_identity_unverified"
	sharedOrphanBirthMatched           = "worker_birth_matches_descendants_unverified"
	sharedOrphanBirthChanged           = "worker_pid_reused_or_changed_descendants_unverified"
	sharedOrphanGroupMembers           = "worker_group_members_descendants_unverified"
	sharedOrphanGroupEmpty             = "worker_group_empty_detached_descendants_unverified"
	sharedOrphanGroupConflict          = "worker_group_identity_conflict"
	sharedOrphanPIDAbsent              = "worker_pid_absent_descendants_unverified"
	sharedOrphanObservationUnavailable = "observation_unavailable"
)

func inspectSharedBrowserOrphanEvidence(ctx context.Context, root string, observe func(context.Context, int) processcontrol.Snapshot) sharedBrowserOrphanInspection {
	return inspectSharedBrowserOrphanEvidenceWithGroup(ctx, root, observe, processcontrol.ObserveBirthIdentity, processcontrol.ObserveGroupBirths)
}

func inspectSharedBrowserOrphanEvidenceWithIdentity(ctx context.Context, root string,
	observe func(context.Context, int) processcontrol.Snapshot,
	birth func(context.Context, int) (processcontrol.BirthIdentity, error),
) sharedBrowserOrphanInspection {
	return inspectSharedBrowserOrphanEvidenceWithGroup(ctx, root, observe, birth, nil)
}

func inspectSharedBrowserOrphanEvidenceWithGroup(ctx context.Context, root string,
	observe func(context.Context, int) processcontrol.Snapshot,
	birth func(context.Context, int) (processcontrol.BirthIdentity, error),
	group func(context.Context, int) (processcontrol.GroupObservation, error),
) sharedBrowserOrphanInspection {
	untrusted := sharedBrowserOrphanInspection{Status: sharedOrphanUntrusted}
	if ctx == nil || ctx.Err() != nil || observe == nil || birth == nil {
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
	// Startup-only evidence is useful after Core crash before MCP handshake;
	// do not imply a ready session or attempt PID/Chrome cleanup.
	onlyStarted := true
	for _, p := range pids {
		if p.Stage == sharedEvidenceReady {
			onlyStarted = false
			break
		}
	}
	if onlyStarted {
		return sharedBrowserOrphanInspection{Status: sharedOrphanPreHandshakeObserved, WorkerObservations: len(pids)}
	}
	result := sharedBrowserOrphanInspection{WorkerObservations: len(pids)}
	if len(pids) > sharedBrowserEvidenceMaxPIDs {
		result.Status = sharedOrphanObservationUnavailable
		return result
	}
	visible, matched, changed := false, false, false
	unavailable := false
	seen := make(map[string]bool, len(pids))
	for _, worker := range pids {
		// A PID can reappear with another start timestamp (or another group).
		// Deduplicate only identical evidence, not separate PID generations.
		key := fmt.Sprintf("%d/%d/%d/%d/%d", worker.PID, worker.StartSec, worker.StartUsec, worker.UID, worker.PGID)
		if seen[key] {
			continue
		}
		seen[key] = true
		snap := observe(ctx, worker.PID)
		switch snap.State {
		case "alive":
			if !worker.HasBirth {
				visible = true // Legacy PID-only observation cannot be attested.
				continue
			}
			current, err := birth(ctx, worker.PID)
			if err != nil || !validSharedBrowserIdentity(current) {
				unavailable = true
				continue
			}
			if worker.matches(current) {
				matched = true
			} else {
				// Identity mismatch can indicate PID reuse, not ownership.
				changed = true
			}
		case "zombie", "not_found":
			// Neither wrapper absence nor zombie proves descendant exit.
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
	case matched:
		result.Status = sharedOrphanBirthMatched
	case changed:
		result.Status = sharedOrphanBirthChanged
	case visible:
		result.Status = sharedOrphanPIDVisible
	default:
		result.Status = sharedOrphanPIDAbsent
	}
	if result.Status == sharedOrphanPIDAbsent && group != nil {
		result.Status = classifyMissingWorkerGroups(ctx, pids, group)
	}
	return result
}

type sharedBrowserWorkerEvidence struct {
	Stage     sharedBrowserEvidenceStage
	PID       int
	HasBirth  bool
	StartSec  int64
	StartUsec int64
	UID       uint32
	PGID      int
}

func (w sharedBrowserWorkerEvidence) matches(id processcontrol.BirthIdentity) bool {
	return w.HasBirth && w.PID == id.PID && w.StartSec == id.StartSec &&
		w.StartUsec == id.StartUsec && w.UID == id.EffectiveUID && w.PGID == id.ProcessGroupID
}

func validSharedBrowserIdentity(id processcontrol.BirthIdentity) bool {
	return id.PID > 0 && id.PID <= math.MaxInt32 && id.StartSec > 0 &&
		id.StartUsec >= 0 && id.StartUsec < 1000000 &&
		id.ProcessGroupID > 0 && id.ProcessGroupID <= math.MaxInt32
}

// Historic v1 PID-only entries remain parseable but cannot prove identity.
// New entries require the full birth fingerprint, no permissive extra fields.
func parseSharedBrowserEvidence(raw string) ([]sharedBrowserWorkerEvidence, bool) {
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
	entries := make([]sharedBrowserWorkerEvidence, 0, len(lines))
	parseDigits := func(raw string) (uint64, bool) {
		if raw == "" || strings.Trim(raw, "0123456789") != "" {
			return 0, false
		}
		value, err := strconv.ParseUint(raw, 10, 64)
		return value, err == nil
	}
	for _, line := range lines {
		fields := strings.Split(line, ",")
		if len(fields) != 1 && len(fields) != 5 {
			return nil, false
		}
		rawPID, ok := strings.CutPrefix(fields[0], "worker_ready_pid=")
		stage := sharedEvidenceReady
		if !ok {
			rawPID, ok = strings.CutPrefix(fields[0], "worker_started_pid=")
			stage = sharedEvidenceStarted
			// Historical journal always used worker_ready_pid. A started
			// witness MUST include complete birth metadata, never PID-only.
			if !ok || len(fields) != 5 {
				return nil, false
			}
		}
		pid, ok := parseDigits(rawPID)
		if !ok || pid == 0 || pid > math.MaxInt32 {
			return nil, false
		}
		e := sharedBrowserWorkerEvidence{Stage: stage, PID: int(pid)}
		if len(fields) == 5 {
			prefixes := [4]string{"start_sec=", "start_usec=", "uid=", "pgid="}
			values := [4]uint64{}
			for i, prefix := range prefixes {
				raw, ok := strings.CutPrefix(fields[i+1], prefix)
				if !ok {
					return nil, false
				}
				values[i], ok = parseDigits(raw)
				if !ok {
					return nil, false
				}
			}
			if values[0] > math.MaxInt64 || values[1] >= 1000000 || values[2] > math.MaxUint32 || values[3] > math.MaxInt32 {
				return nil, false
			}
			e.HasBirth = true
			e.StartSec = int64(values[0])
			e.StartUsec = int64(values[1])
			e.UID = uint32(values[2])
			e.PGID = int(values[3])
			if !validSharedBrowserIdentity(processcontrol.BirthIdentity{
				PID: e.PID, StartSec: e.StartSec, StartUsec: e.StartUsec,
				EffectiveUID: e.UID, ProcessGroupID: e.PGID,
			}) {
				return nil, false
			}
		}
		entries = append(entries, e)
	}
	return entries, true
}
