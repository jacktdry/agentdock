package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func portFixtureRequest(t *testing.T, port int) PortObservationRequest {
	t.Helper()
	root := t.TempDir()
	return PortObservationRequest{Runtime: NextPortRuntime{Variant: "next", Root: root, Binary: filepath.Join(root, "core"), PID: 123, ProcessInstance: "creation-1"},
		Host: "127.0.0.1", ConfiguredPort: 8767, CandidatePort: port, ConfigRevision: "revision-1"}
}

func tempPortListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	if port := listener.Addr().(*net.TCPAddr).Port; port == 8765 || port == 8766 || port == 8767 {
		t.Fatal("fixture selected production port")
	}
	return listener
}

func TestPortReservedNeverProbes(t *testing.T) {
	for port, reason := range map[int]string{8765: "stable_reserved", 8766: "memory_reserved"} {
		request := PortObservationRequest{CandidatePort: port, ConfigRevision: "r"}
		observation := observePort(context.Background(), request, func(context.Context, PortObservationRequest) (PortState, string) {
			t.Fatal("reserved port probed")
			return PortUnknown, ""
		})
		if observation.State != PortReserved || observation.ReasonCode != reason || observation.ObservedAt.IsZero() {
			t.Fatal(observation)
		}
		if _, err := PreflightPortMutation(context.Background(), request); !errors.Is(err, ErrPortPreflight) {
			t.Fatal("reserved mutation accepted")
		}
	}
}

func TestPortAvailableTempScope(t *testing.T) {
	listener := tempPortListener(t)
	request := portFixtureRequest(t, listener.Addr().(*net.TCPAddr).Port)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	observation := ObservePort(context.Background(), request)
	if runtime.GOOS == "windows" {
		// Without trustworthy Next birth evidence, even a free port cannot
		// authorize a Windows mutation.
		if observation.State != PortUnknown || observation.ReasonCode != "next_identity_unavailable" || observation.ObservedAt.Before(before) {
			t.Fatal("unproven Windows Next identity accepted", observation)
		}
		if preflight, err := PreflightPortMutation(context.Background(), request); !errors.Is(err, ErrPortPreflight) || preflight.State != PortUnknown {
			t.Fatal("unproven Windows port mutation allowed", preflight, err)
		}
		return
	}
	if observation.State != PortAvailable || observation.ConfiguredPort != 8767 || observation.ObservedPort != request.CandidatePort || observation.ConfigRevision != request.ConfigRevision || observation.ObservedAt.Before(before) {
		t.Fatal(observation)
	}
}

func TestPortMutationReobservesWithoutAcquiringLock(t *testing.T) {
	listener := tempPortListener(t)
	request := portFixtureRequest(t, listener.Addr().(*net.TCPAddr).Port)
	_ = listener.Close()
	initial := ObservePort(context.Background(), request)
	if runtime.GOOS == "windows" {
		if initial.State != PortUnknown || initial.ReasonCode != "next_identity_unavailable" {
			t.Fatal("unproven Windows Next identity accepted", initial)
		}
	} else if initial.State != PortAvailable {
		t.Fatal("fixture unavailable")
	}
	release, err := AcquireDesktopMutation(context.Background(), request.Runtime.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	listener, err = net.Listen("tcp", net.JoinHostPort(request.Host, strconv.Itoa(request.CandidatePort)))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	observation, err := PreflightPortMutation(ctx, request)
	if err == nil || observation.State == PortAvailable || observation.State == PortOwnedByNext {
		t.Fatal("stale observation authorized mutation", observation)
	}
}

func TestPortOccupiedFixtures(t *testing.T) {
	// Classifier fixtures are intentionally independent of the kernel bind.
	// Windows may report WSAEACCES for an exclusive listener; that is an
	// unknown probe result, not authority to manufacture owner evidence.
	request := portFixtureRequest(t, 18768)
	for _, tc := range []struct {
		name             string
		pids             []int
		binary, instance string
		err              error
		want             PortState
	}{
		{"owned", []int{123}, request.Runtime.Binary, "creation-1", nil, PortOwnedByNext},
		{"foreign", []int{456}, filepath.Join(request.Runtime.Root, "foreign-secret"), "creation-2", nil, PortConflict},
		{"pid-reused", []int{123}, request.Runtime.Binary, "creation-2", nil, PortUnknown},
		{"another-next-instance", []int{456}, request.Runtime.Binary, "creation-1", nil, PortUnknown},
		{"ambiguous", []int{123, 456}, request.Runtime.Binary, "creation-1", nil, PortUnknown},
		{"denied", []int{123}, "", "", errors.New("secret-command-line"), PortUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, reason := classifyPortOwner(request, tc.pids, tc.binary, tc.instance, tc.err)
			if state != tc.want {
				t.Fatalf("state=%s reason=%s want=%s", state, reason, tc.want)
			}
			encoded, _ := json.Marshal(PortObservation{State: state, ReasonCode: reason})
			if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), request.Runtime.Root) {
				t.Fatal("private evidence leaked")
			}
		})
	}
}

func TestPortAddressInUsePlatform(t *testing.T) {
	if !portAddressInUse(syscall.EADDRINUSE) {
		t.Fatal("EADDRINUSE must trigger a conservative ownership probe")
	}
	// Windows net.Listen wraps Winsock error 10048 in net.OpError.
	if got := portAddressInUse(&net.OpError{Op: "listen", Net: "tcp", Err: syscall.Errno(10048)}); got != (runtime.GOOS == "windows") {
		t.Fatalf("WSAEADDRINUSE classification = %t on %s", got, runtime.GOOS)
	}
	if portAddressInUse(syscall.Errno(10013)) {
		t.Fatal("access denied is not listener ownership evidence")
	}
}

func TestPortProbePreservesCallerDeadline(t *testing.T) {
	listener := tempPortListener(t)
	request := portFixtureRequest(t, listener.Addr().(*net.TCPAddr).Port)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	deadline, _ := ctx.Deadline()
	called := false
	observation := observePort(ctx, request, func(probeCtx context.Context, _ PortObservationRequest) (PortState, string) {
		called = true
		if probeDeadline, ok := probeCtx.Deadline(); !ok || !probeDeadline.Equal(deadline) {
			t.Fatal("ownership probe extended caller deadline")
		}
		cancel()
		if !errors.Is(probeCtx.Err(), context.Canceled) {
			t.Fatal("ownership probe ignored caller cancellation")
		}
		return PortUnknown, "ownership_unavailable"
	})
	if !called && runtime.GOOS == "windows" && observation.State == PortUnknown && observation.ReasonCode == "probe_failed" {
		// A Windows exclusive listener can make Listen return WSAEACCES
		// instead of WSAEADDRINUSE. No owner probe is allowed in that case.
		return
	}
	if !called || observation.State != PortUnknown {
		t.Fatal("cancelled ownership probe accepted", observation)
	}
}

func TestPortInvalidIdentityAndScope(t *testing.T) {
	request := portFixtureRequest(t, 8767) // invalid inputs must not probe Next's real default.
	for _, mutate := range []func(*PortObservationRequest){
		func(r *PortObservationRequest) { r.Runtime.Variant = "stable" },
		func(r *PortObservationRequest) { r.Runtime.Root = "" },
		func(r *PortObservationRequest) { r.Host = "localhost" },
		func(r *PortObservationRequest) { r.CandidatePort = 0 },
		func(r *PortObservationRequest) { r.CandidatePort = 65536 },
	} {
		candidate := request
		mutate(&candidate)
		if ObservePort(context.Background(), candidate).State != PortUnknown {
			t.Fatal("invalid selection accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ObservePort(ctx, request).ReasonCode != "observation_cancelled" {
		t.Fatal("cancelled observation probed")
	}
}

func TestPortPlatformSourcesDoNotUseCommandPrefixOrLegacyFallback(t *testing.T) {
	for _, file := range []string{"port_observation_darwin.go", "port_observation_windows.go", "port_observation.go"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, unsafe := range []string{"AcquireDesktopMutation(", "launchctl", "command=", "ActiveCoreBinary("} {
			if strings.Contains(string(source), unsafe) {
				t.Fatalf("%s uses %s", file, unsafe)
			}
		}
	}
	for file, proofs := range map[string][]string{
		"port_observation_windows.go":    {"GetExtendedTcpTable", "QueryFullProcessImageName", "GetProcessTimes", "classifyPortOwner"},
		"port_observation_darwin_cgo.go": {"proc_pidpath", "PROC_PIDTBSDINFO", "pbi_start_tvsec", "pbi_start_tvusec"},
	} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, proof := range proofs {
			if !strings.Contains(string(source), proof) {
				t.Fatalf("%s missing identity proof %s", file, proof)
			}
		}
	}
}
