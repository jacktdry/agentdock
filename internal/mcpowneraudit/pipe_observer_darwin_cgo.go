//go:build darwin && cgo

package mcpowneraudit

/*
#include <libproc.h>
#include <sys/proc_info.h>
#include <stdint.h>

// Return only opaque kernel pipe handles and FD numbers. No paths, commands,
// credentials, tool arguments, or user-provided strings enter this collector.
typedef struct {
    int32_t fd;
    uint64_t handle;
    uint64_t peer;
} audit_pipe_fd;

// The limit+1 entry detects truncation. Any disappearing FD or inaccessible
// process rejects the whole snapshot; returning a partial list is unsafe.
static int audit_pipe_fds(int pid, audit_pipe_fd *out, int limit) {
    struct proc_fdinfo fds[4097];
    if (limit < 1 || limit > 4096 || pid <= 0) return -1;
    int bytes = proc_pidinfo(pid, PROC_PIDLISTFDS, 0, fds, (limit + 1) * sizeof(fds[0]));
    if (bytes <= 0 || bytes % sizeof(fds[0]) != 0) return -1;
    int n = bytes / sizeof(fds[0]);
    if (n > limit) return -1;
    int used = 0;
    for (int i = 0; i < n; i++) {
        if (fds[i].proc_fdtype != PROX_FDTYPE_PIPE) continue;
        struct pipe_fdinfo info;
        int got = proc_pidfdinfo(pid, fds[i].proc_fd, PROC_PIDFDPIPEINFO, &info, sizeof(info));
        if (got != sizeof(info) || !info.pipeinfo.pipe_handle || !info.pipeinfo.pipe_peerhandle) return -1;
        out[used].fd = fds[i].proc_fd;
        out[used].handle = info.pipeinfo.pipe_handle;
        out[used].peer = info.pipeinfo.pipe_peerhandle;
        used++;
    }
    return used;
}
*/
import "C"

import (
	"context"
	"time"

	"github.com/uvwt/agentdock/internal/process"
)

// This is deliberately NOT registered with an ACP/HTTP/CLI runtime. The peer
// proof is diagnostic-only; it cannot produce Next ownership or cleanup rights.
// A future trusted adapter diagnostic can pass a *complete* native snapshot.
type darwinKernelPipeObserver struct{}

// Do not silently treat a truncated loaded-thread snapshot as complete. This
// conservative per-observation cap is separate from the correlation row limit.
const maxKernelPipeChildren = maxCorrelationRows
const maxKernelPipeFDs = 4096

type observedPipeFD struct {
	fd           int
	handle, peer uint64
}

func snapshotPipeFDs(pid int) ([]observedPipeFD, error) {
	var out [maxKernelPipeFDs]C.audit_pipe_fd
	n := int(C.audit_pipe_fds(C.int(pid), &out[0], C.int(maxKernelPipeFDs)))
	if n < 0 || n > maxKernelPipeFDs {
		return nil, errUnavailable
	}
	result := make([]observedPipeFD, 0, n)
	for _, v := range out[:n] {
		result = append(result, observedPipeFD{int(v.fd), uint64(v.handle), uint64(v.peer)})
	}
	return result, nil
}

func (darwinKernelPipeObserver) pipeSnapshot(ctx context.Context, req diagnosticRequest, children []transportChild) (pipeEvidence, error) {
	if ctx == nil || ctx.Err() != nil || req.binding.incarnation == 0 ||
		!validIdentity(req.binding.native) || !validIdentity(req.binding.adapter) ||
		len(children) == 0 || len(children) > maxKernelPipeChildren || req.issued.IsZero() ||
		time.Since(req.issued) > correlationWindow || time.Until(req.issued) > 0 {
		return pipeEvidence{}, errUnavailable
	}
	pinned := func(id process.BirthIdentity) bool {
		if ctx.Err() != nil {
			return false
		}
		got, err := process.ObserveBirthIdentity(ctx, id.PID)
		return err == nil && got == id
	}
	if !pinned(req.binding.adapter) || !pinned(req.binding.native) || req.binding.native.ParentPID != req.binding.adapter.PID {
		return pipeEvidence{}, errUnavailable
	}
	parentFDs, err := snapshotPipeFDs(req.binding.native.PID)
	if err != nil || ctx.Err() != nil {
		return pipeEvidence{}, errUnavailable
	}
	seenPID, seenChildEnd, usedNativeFD := map[int]bool{}, map[uint64]bool{}, map[int]bool{}
	childPipeLists := map[int][]observedPipeFD{}
	result := pipeEvidence{stamp: evidenceStamp{binding: req.binding, provenance: kernelStdioPipeObservation, challenge: req.challenge, scope: correlationScope, complete: true}, children: make([]kernelTransport, 0, len(children))}
	for _, child := range children {
		id := child.child
		if !validIdentity(id) || id.ParentPID != req.binding.native.PID || id.EffectiveUID != req.binding.native.EffectiveUID || seenPID[id.PID] || !pinned(id) {
			return pipeEvidence{}, errUnavailable
		}
		seenPID[id.PID] = true
		fds, err := snapshotPipeFDs(id.PID)
		if err != nil || ctx.Err() != nil {
			return pipeEvidence{}, errUnavailable
		}
		childPipeLists[id.PID] = fds
		var stdin, stdout observedPipeFD
		got0, got1 := false, false
		for _, fd := range fds {
			if fd.fd == 0 {
				stdin, got0 = fd, true
			}
			if fd.fd == 1 {
				stdout, got1 = fd, true
			}
		}
		if !got0 || !got1 || stdin.handle == stdout.handle || stdin.peer == stdout.peer || stdin.handle == stdout.peer || stdout.handle == stdin.peer {
			return pipeEvidence{}, errUnavailable
		}
		for _, end := range []observedPipeFD{stdin, stdout} {
			if seenChildEnd[end.handle] || seenChildEnd[end.peer] {
				return pipeEvidence{}, errUnavailable
			}
			seenChildEnd[end.handle], seenChildEnd[end.peer] = true, true
			// Exactly one native FD must own the opposite end; neither
			// process may alias these endpoints on another descriptor.
			matches := 0
			for _, fd := range parentFDs {
				if fd.handle == end.handle {
					return pipeEvidence{}, errUnavailable
				}
				if fd.handle == end.peer && fd.peer == end.handle {
					if usedNativeFD[fd.fd] {
						return pipeEvidence{}, errUnavailable
					}
					usedNativeFD[fd.fd] = true
					matches++
				}
			}
			if matches != 1 {
				return pipeEvidence{}, errUnavailable
			}
			for _, fd := range fds {
				if fd.fd != end.fd && (fd.handle == end.handle || fd.handle == end.peer) {
					return pipeEvidence{}, errUnavailable
				}
			}
		}
		result.children = append(result.children, kernelTransport{child: id, pipes: stdioPipes{stdin: stdin.handle, stdout: stdout.handle}})
	}
	// Observations are non-atomic: recheck all identities and the pipe table
	// before publishing, then preserve the five-second evidence freshness gate.
	if !pinned(req.binding.native) || !pinned(req.binding.adapter) {
		return pipeEvidence{}, errUnavailable
	}
	for _, child := range result.children {
		if !pinned(child.child) {
			return pipeEvidence{}, errUnavailable
		}
		fds, err := snapshotPipeFDs(child.child.PID)
		if err != nil || !sameObservedPipes(childPipeLists[child.child.PID], fds) {
			return pipeEvidence{}, errUnavailable
		}
	}
	latestFDs, err := snapshotPipeFDs(req.binding.native.PID)
	if err != nil || !sameObservedPipes(parentFDs, latestFDs) || ctx.Err() != nil {
		return pipeEvidence{}, errUnavailable
	}
	now := time.Now()
	if now.Before(req.issued) || now.Sub(req.issued) > correlationWindow {
		return pipeEvidence{}, errUnavailable
	}
	result.stamp.observed = now
	return result, nil
}

func sameObservedPipes(a, b []observedPipeFD) bool {
	if len(a) != len(b) {
		return false
	}
	byFD := make(map[int]observedPipeFD, len(a))
	for _, v := range a {
		if _, exists := byFD[v.fd]; exists {
			return false
		}
		byFD[v.fd] = v
	}
	seen := make(map[int]bool, len(b))
	for _, v := range b {
		old, ok := byFD[v.fd]
		if !ok || old != v || seen[v.fd] {
			return false
		}
		seen[v.fd] = true
	}
	return true
}
