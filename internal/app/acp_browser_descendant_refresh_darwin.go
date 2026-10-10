//go:build darwin

package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	processcontrol "github.com/uvwt/agentdock/internal/process"
	"golang.org/x/sys/unix"
)

// refreshReadyDescendants records only NEW descendants of the original,
// birth-attested managed wrapper. It never signals or resumes a PID. Existing
// birth evidence is immutable; PID reuse/identity drift fails closed.
func (g *sharedBrowserRestartGuard) refreshReadyDescendants(ctx context.Context, root processcontrol.BirthIdentity) error {
	if ctx == nil || ctx.Err() != nil || g == nil {
		return errSharedBrowserEvidence
	}
	descendants, err := processcontrol.ObserveDescendantBirths(ctx, root)
	if err != nil {
		return errSharedBrowserEvidence
	}
	return g.appendNewReadyDescendants(root, descendants)
}

func (g *sharedBrowserRestartGuard) appendNewReadyDescendants(root processcontrol.BirthIdentity, sample []processcontrol.BirthIdentity) error {
	if g == nil || !validSharedBrowserIdentity(root) || !validateSharedDescendantLineage(root, sample) {
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
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || uint64(st.Dev) != g.device || st.Ino != g.inode ||
		st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) ||
		st.Mode&0077 != 0 || st.Size < int64(len(sharedBrowserEvidencePrefix)) ||
		st.Size > sharedBrowserEvidenceMaxBytes {
		return errSharedBrowserEvidence
	}
	buf := make([]byte, st.Size)
	n, readErr := unix.Pread(fd, buf, 0)
	if readErr != nil || n != len(buf) {
		return errSharedBrowserEvidence
	}
	existing, valid := parseSharedBrowserEvidence(string(buf))
	if !valid {
		return errSharedBrowserEvidence
	}
	ready := false
	seen := make(map[int]sharedBrowserWorkerEvidence)
	for _, entry := range existing {
		switch entry.Stage {
		case sharedEvidenceReady:
			if entry.PID == root.PID && entry.HasBirth &&
				entry.matches(root) {
				ready = true
			}
		case sharedEvidenceDescendant:
			if entry.RootPID != root.PID || entry.RootStartSec != root.StartSec ||
				entry.RootStartUsec != root.StartUsec {
				continue
			}
			if _, duplicate := seen[entry.PID]; duplicate {
				return errSharedBrowserEvidence
			}
			seen[entry.PID] = entry
		}
	}
	if !ready {
		return errSharedBrowserEvidence
	}
	var lines strings.Builder
	additions := 0
	for _, d := range sample {
		if prior, ok := seen[d.PID]; ok {
			if prior.StartSec != d.StartSec || prior.StartUsec != d.StartUsec ||
				prior.UID != d.EffectiveUID {
				return errSharedBrowserEvidence
			}
			continue
		}
		fmt.Fprintf(&lines, "worker_descendant_pid=%d,start_sec=%d,start_usec=%d,uid=%d,pgid=%d,parent_pid=%d,root_pid=%d,root_start_sec=%d,root_start_usec=%d\n",
			d.PID, d.StartSec, d.StartUsec, d.EffectiveUID, d.ProcessGroupID, d.ParentPID,
			root.PID, root.StartSec, root.StartUsec)
		additions++
	}
	if additions == 0 {
		return nil
	} // unchanged snapshot: zero journal I/O
	line := lines.String()
	if len(existing)+additions > sharedBrowserEvidenceMaxPIDs ||
		st.Size+int64(len(line)) > sharedBrowserEvidenceMaxBytes {
		return errSharedBrowserEvidence
	}
	if _, ok := parseSharedBrowserEvidence(string(buf) + line); !ok {
		return errSharedBrowserEvidence
	}
	written, writeErr := unix.Write(fd, []byte(line))
	if writeErr != nil || written != len(line) || unix.Fsync(fd) != nil || unix.Fsync(g.dirfd) != nil {
		// Any incomplete write keeps the restart fence at next launch.
		return errSharedBrowserEvidence
	}
	return nil
}
