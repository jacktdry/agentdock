//go:build darwin && cgo

package mcpowneraudit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/process"
)

// All helper PIDs are created by this test. No existing MCP/Codex process is
// inspected, closed, or reaped here.
func TestDarwinOwnedStdioFixtureHelper(t *testing.T) {
	switch os.Getenv("AGENTDOCK_PIPE_FIXTURE") {
	case "native", "native-miswired":
		child := exec.Command(os.Args[0], "-test.run=^TestDarwinOwnedStdioFixtureHelper$")
		child.Env = append(os.Environ(), "AGENTDOCK_PIPE_FIXTURE=child")
		stdin, err := child.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		var stdout io.ReadCloser
		if os.Getenv("AGENTDOCK_PIPE_FIXTURE") == "native-miswired" {
			// Child inherits native's stdout, whose peer is held by the test,
			// not the native parent. The observer must reject this binding.
			child.Stdout = os.Stdout
		} else {
			stdout, err = child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(os.Stdout, child.Process.Pid)
		var signal [1]byte
		_, _ = os.Stdin.Read(signal[:])
		_ = stdin.Close()
		drainDone := make(chan struct{})
		if stdout != nil {
			go func() { _, _ = io.Copy(io.Discard, stdout); close(drainDone) }()
		} else {
			close(drainDone)
		}
		if err := child.Wait(); err != nil {
			t.Fatal(err)
		}
		<-drainDone
	case "child":
		_, _ = io.Copy(io.Discard, os.Stdin)
	}
}

func TestDarwinKernelPipeObserverOwnedLiveFixture(t *testing.T) {
	native := exec.Command(os.Args[0], "-test.run=^TestDarwinOwnedStdioFixtureHelper$")
	native.Env = append(os.Environ(), "AGENTDOCK_PIPE_FIXTURE=native")
	stdin, err := native.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := native.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := native.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = stdin.Write([]byte("x"))
		_ = stdin.Close()
		done := make(chan error, 1)
		go func() { done <- native.Wait() }()
		select {
		case err := <-done:
			_ = stdout.Close()
			if err != nil {
				t.Errorf("owned helper exited uncleanly: %v", err)
			}
		case <-time.After(5 * time.Second):
			// A test-owned PID only, and only after bounded graceful shutdown.
			_ = native.Process.Kill()
			<-done
			t.Error("owned helper did not terminate gracefully")
		}
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("invalid owned child PID: %v", err)
	}
	ctx := context.Background()
	adapterID, err := process.ObserveBirthIdentity(ctx, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	nativeID, err := process.ObserveBirthIdentity(ctx, native.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	childID, err := process.ObserveBirthIdentity(ctx, childPID)
	if err != nil {
		t.Fatal(err)
	}
	if nativeID.ParentPID != adapterID.PID || childID.ParentPID != nativeID.PID {
		t.Fatal("fixture ancestry invalid")
	}

	req := diagnosticRequest{binding: adapterBinding{incarnation: 1, adapter: adapterID, native: nativeID}, issued: time.Now()}
	req.challenge[0] = 0x42
	c := []transportChild{{transport: "test-owned", thread: "test-thread", child: childID}}
	observer := darwinKernelPipeObserver{}
	result, err := observer.pipeSnapshot(ctx, req, c)
	if err != nil {
		t.Fatalf("expected kernel pipe evidence for owned child: %v", err)
	}
	if result.stamp.provenance != kernelStdioPipeObservation || result.stamp.challenge != req.challenge || result.stamp.binding != req.binding || !result.stamp.complete || len(result.children) != 1 || result.children[0].child != childID || result.children[0].pipes.stdin == 0 || result.children[0].pipes.stdout == 0 || result.children[0].pipes.stdin == result.children[0].pipes.stdout {
		t.Fatalf("unverified pipe evidence: %+v", result)
	}
	again, err := observer.pipeSnapshot(ctx, req, c)
	if err != nil || !reflect.DeepEqual(again.children, result.children) {
		t.Fatalf("unstable fixture pipe identities: %v", err)
	}

	rejects := []struct {
		name   string
		mutate func(*diagnosticRequest, *[]transportChild, *context.Context)
	}{
		{"wrong_adapter_birth", func(r *diagnosticRequest, _ *[]transportChild, _ *context.Context) { r.binding.adapter.StartSec++ }},
		{"wrong_native_birth", func(r *diagnosticRequest, _ *[]transportChild, _ *context.Context) { r.binding.native.StartUsec++ }},
		{"wrong_child_birth", func(_ *diagnosticRequest, c *[]transportChild, _ *context.Context) { (*c)[0].child.StartUsec++ }},
		{"wrong_child_parent", func(_ *diagnosticRequest, c *[]transportChild, _ *context.Context) {
			(*c)[0].child.ParentPID = os.Getpid()
		}},
		{"duplicate_child", func(_ *diagnosticRequest, c *[]transportChild, _ *context.Context) { *c = append(*c, (*c)[0]) }},
		{"empty_children", func(_ *diagnosticRequest, c *[]transportChild, _ *context.Context) { *c = nil }},
		{"too_many_children", func(_ *diagnosticRequest, c *[]transportChild, _ *context.Context) {
			*c = make([]transportChild, maxKernelPipeChildren+1)
		}},
		{"expired_challenge", func(r *diagnosticRequest, _ *[]transportChild, _ *context.Context) {
			r.issued = time.Now().Add(-correlationWindow - time.Second)
		}},
		{"future_challenge", func(r *diagnosticRequest, _ *[]transportChild, _ *context.Context) {
			r.issued = time.Now().Add(time.Minute)
		}},
		{"cancelled", func(_ *diagnosticRequest, _ *[]transportChild, ctx *context.Context) {
			cc, cancel := context.WithCancel(*ctx)
			cancel()
			*ctx = cc
		}},
	}
	for _, tc := range rejects {
		t.Run(tc.name, func(t *testing.T) {
			changed, children, cctx := req, append([]transportChild(nil), c...), ctx
			tc.mutate(&changed, &children, &cctx)
			observed, err := observer.pipeSnapshot(cctx, changed, children)
			if !errors.Is(err, errUnavailable) || observed.stamp.complete || len(observed.children) > 0 {
				t.Fatalf("partial/stale evidence accepted: %+v, %v", observed, err)
			}
		})
	}
}

func TestDarwinKernelPipeObserverRejectsUnownedPeer(t *testing.T) {
	native := exec.Command(os.Args[0], "-test.run=^TestDarwinOwnedStdioFixtureHelper$")
	native.Env = append(os.Environ(), "AGENTDOCK_PIPE_FIXTURE=native-miswired")
	stdin, err := native.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := native.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := native.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = stdin.Write([]byte("x"))
		_ = stdin.Close()
		done := make(chan error, 1)
		go func() { done <- native.Wait() }()
		select {
		case err := <-done:
			_ = stdout.Close()
			if err != nil {
				t.Errorf("owned mismatched fixture: %v", err)
			}
		case <-time.After(5 * time.Second):
			_ = native.Process.Kill() // test-created child only
			<-done
			t.Error("owned mismatched fixture stuck")
		}
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	adapter, err := process.ObserveBirthIdentity(ctx, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	nativeID, err := process.ObserveBirthIdentity(ctx, native.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	child, err := process.ObserveBirthIdentity(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	req := diagnosticRequest{binding: adapterBinding{incarnation: 1, adapter: adapter, native: nativeID}, issued: time.Now()}
	got, err := (darwinKernelPipeObserver{}).pipeSnapshot(ctx, req, []transportChild{{child: child}})
	if !errors.Is(err, errUnavailable) || got.stamp.complete || len(got.children) != 0 {
		t.Fatalf("unowned kernel pipe peer falsely correlated: %+v, %v", got, err)
	}
}

func TestSameObservedPipeListDetectsMutationAndDuplicates(t *testing.T) {
	a := []observedPipeFD{{fd: 0, handle: 10, peer: 11}, {fd: 8, handle: 20, peer: 21}}
	if !sameObservedPipes(a, append([]observedPipeFD(nil), a...)) || !sameObservedPipes(a, []observedPipeFD{a[1], a[0]}) {
		t.Fatal("valid stable snapshots rejected")
	}
	if sameObservedPipes(a, []observedPipeFD{a[0], a[0]}) || sameObservedPipes(a, []observedPipeFD{a[0], {fd: 8, handle: 20, peer: 22}}) || sameObservedPipes(a, a[:1]) {
		t.Fatal("mutated or partial snapshot accepted")
	}
}
