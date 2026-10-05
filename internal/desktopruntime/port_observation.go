package desktopruntime

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

type PortState string

const (
	PortAvailable   PortState = "available"
	PortOwnedByNext PortState = "owned_by_next"
	PortReserved    PortState = "reserved"
	PortConflict    PortState = "conflict"
	PortUnknown     PortState = "unknown"
)

// NextPortRuntime is trusted backend selection, never renderer-supplied evidence.
// PID and ProcessInstance must come from the selected Core's lifecycle, not from
// discovering whichever process currently owns the candidate port. A stopped
// Core may leave them empty; occupied ports then fail closed.
type NextPortRuntime struct {
	Variant         string
	Root            string
	Binary          string
	PID             int
	ProcessInstance string
}

type PortObservationRequest struct {
	Runtime        NextPortRuntime
	Host           string // Literal Core bind address; no DNS lookup or default scope.
	ConfiguredPort int
	CandidatePort  int
	ConfigRevision string
}

// PortObservation contains only safe metadata, never OS errors or process paths.
type PortObservation struct {
	ConfiguredPort int       `json:"configuredPort"`
	ObservedPort   int       `json:"observedPort"`
	State          PortState `json:"state"`
	ReasonCode     string    `json:"reasonCode"`
	ObservedAt     time.Time `json:"observedAt"`
	ConfigRevision string    `json:"configRevision"`
}

var ErrPortPreflight = errors.New("port preflight rejected")

// ObservePort is UI feedback, not a reusable authorization for a later mutation.
// It temporarily binds the candidate scope to establish availability, but does
// not change configuration, contact Core, or acquire the desktop mutation lock.
func ObservePort(ctx context.Context, request PortObservationRequest) PortObservation {
	return observePort(ctx, request, platformPortOwner)
}

// PreflightPortMutation must be called under the caller's Desktop mutation lock,
// immediately before the first side effect, with freshly selected configuration
// and process identity. It always observes again. It neither acquires nor proves
// lock ownership. The subsequent Core bind/restart can still race and must fail
// or roll back on bind failure, regardless of this observation.
func PreflightPortMutation(ctx context.Context, request PortObservationRequest) (PortObservation, error) {
	observation := ObservePort(ctx, request)
	if observation.State != PortAvailable && observation.State != PortOwnedByNext {
		return observation, ErrPortPreflight
	}
	return observation, nil
}

// PortProcessInstance captures a PID-reuse-resistant creation identity. Call it
// when selecting the Next Core process, not to adopt an arbitrary port owner.
func PortProcessInstance(pid int) (string, error) {
	_, instance, err := platformPortProcess(pid)
	if err != nil {
		return "", ErrPortPreflight
	}
	return instance, nil
}

type portOwnerProbe func(context.Context, PortObservationRequest) (PortState, string)

func observePort(ctx context.Context, request PortObservationRequest, owner portOwnerProbe) (result PortObservation) {
	result = PortObservation{ConfiguredPort: request.ConfiguredPort, ObservedPort: request.CandidatePort,
		ConfigRevision: request.ConfigRevision, State: PortUnknown, ReasonCode: "invalid_request"}
	defer func() { result.ObservedAt = time.Now().UTC() }()
	switch request.CandidatePort {
	case 8765:
		result.State, result.ReasonCode = PortReserved, "stable_reserved"
		return
	case 8766:
		result.State, result.ReasonCode = PortReserved, "memory_reserved"
		return
	}
	if request.CandidatePort < 1 || request.CandidatePort > 65535 || net.ParseIP(request.Host) == nil {
		return
	}
	if request.Runtime.Variant != "next" || !filepath.IsAbs(request.Runtime.Root) || !filepath.IsAbs(request.Runtime.Binary) {
		result.ReasonCode = "next_identity_unavailable"
		return
	}
	if ctx.Err() != nil {
		result.ReasonCode = "observation_cancelled"
		return
	}
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", net.JoinHostPort(request.Host, strconv.Itoa(request.CandidatePort)))
	if err == nil {
		if listener.Close() != nil {
			result.ReasonCode = "probe_failed"
			return
		}
		result.State, result.ReasonCode = PortAvailable, "bind_available"
		return
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		result.ReasonCode = "probe_failed"
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result.State, result.ReasonCode = owner(probeCtx, request)
	return
}

// All listeners on the port are checked conservatively, including other local
// addresses. Multiple PIDs or unreadable process evidence are never adopted.
func portBinaryMatches(left, right string) bool {
	if runtime.GOOS == "windows" {
		return samePath(left, right)
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func classifyPortOwner(request PortObservationRequest, pids []int, binary, instance string, err error) (PortState, string) {
	if err != nil || len(pids) != 1 || pids[0] <= 0 || binary == "" || instance == "" {
		return PortUnknown, "ownership_unavailable"
	}
	if !portBinaryMatches(binary, request.Runtime.Binary) {
		return PortConflict, "foreign_listener"
	}
	if request.Runtime.PID != pids[0] || request.Runtime.ProcessInstance == "" || request.Runtime.ProcessInstance != instance {
		return PortUnknown, "process_instance_mismatch"
	}
	return PortOwnedByNext, "selected_next_process"
}
