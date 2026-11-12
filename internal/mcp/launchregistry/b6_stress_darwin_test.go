//go:build darwin

package launchregistry

import (
	"context"
	"fmt"
	"github.com/uvwt/agentdock/internal/process"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Real test-owned MCP child processes, not synthetic caller-provided PIDs.
// These twenty rounds are deterministic validation, never a substitute for
// normal-user production telemetry or real authenticated Next UAT.
func TestB6TwentyStdioMCPRegistrationRounds(t *testing.T) {
	parent, err := process.ObserveBirthIdentity(context.Background(), os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprintf("cycle-%02d", i), func(t *testing.T) {
			socket, ch := openTestRegistrar(t, parent, "echo")
			marker := filepath.Join(t.TempDir(), "owned-child")
			out, err := spawnWrapper(t, socket, "echo", testDigest, "wrapper", "", marker)
			observed := waitResult(t, ch)
			if err != nil || observed.err != nil || out != "mcp:hello\n" {
				t.Fatalf("round=%d: %v %v output=%q", i, err, observed.err, out)
			}
			if observed.observed.Child.ParentPID != observed.observed.Wrapper.PID || observed.observed.Wrapper.ParentPID != parent.PID {
				t.Fatalf("wrong owner chain: %+v", observed.observed)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatal("owned fixture never spawned: ", err)
			}
			// A completed/closed MCP process must never be reclaimed using stale PID.
			verdict := Reconcile(context.Background(), observed.observed, 0)
			if verdict.CleanupAllowed || verdict.ReleaseAction != "none" {
				t.Fatalf("unsafe release action: %+v", verdict)
			}
		})
	}
}

func TestB6EightParallelMCPChildBirthsDoNotCrossAuthorize(t *testing.T) {
	parent, err := process.ObserveBirthIdentity(context.Background(), os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	const total = 8
	var wg sync.WaitGroup
	errors := make(chan error, total)
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// t.TempDir/t.Fatal helpers are not safe outside the test goroutine.
			dir, err := os.MkdirTemp("/private/tmp", "next-b6-parallel-")
			if err != nil {
				errors <- err
				return
			}
			defer os.RemoveAll(dir)
			sock := filepath.Join(dir, "m.sock")
			listener, err := openParallelTestRegistrar(sock, parent)
			if err != nil {
				errors <- err
				return
			}
			defer listener.Close()
			out, err := spawnParallelTestWrapper(sock)
			if err != nil || !strings.Contains(out, "mcp:hello\n") {
				errors <- fmt.Errorf("%d: %v: %q", i, err, out)
				return
			}
			select {
			case observed := <-listener.results:
				if observed.err != nil || observed.observed.Child.ParentPID != observed.observed.Wrapper.PID {
					errors <- fmt.Errorf("%d provenance rejected: %+v", i, observed)
				}
			case <-time.After(6 * time.Second):
				errors <- fmt.Errorf("%d stalled", i)
			}
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
}

// The parallel test never opens any user-owned socket; each registrar only
// accepts one child from its own temporary Unix listener.
type parallelRegistrar struct {
	listener *net.UnixListener
	results  chan acceptResult
}

func openParallelTestRegistrar(sock string, parent process.BirthIdentity) (*parallelRegistrar, error) {
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		return nil, err
	}
	p := &parallelRegistrar{listener: l, results: make(chan acceptResult, 1)}
	go func() {
		conn, e := l.AcceptUnix()
		if e != nil {
			p.results <- acceptResult{err: e}
			return
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ob, e := (Registrar{AppServer: parent, Server: "echo", ConfigDigest: testDigest}).Accept(ctx, conn)
		p.results <- acceptResult{observed: ob, err: e}
	}()
	return p, nil
}
func (p *parallelRegistrar) Close() error { return p.listener.Close() }
func spawnParallelTestWrapper(sock string) (string, error) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestLaunchRegistryHelperProcess$")
	cmd.Env = append(os.Environ(), "LAUNCH_REG_HELPER=wrapper", "LAUNCH_TEST_SERVER=echo", "LAUNCH_TEST_DIGEST="+testDigest, "AGENTDOCK_NEXT_MCP_LAUNCH_SOCKET="+sock)
	cmd.Stdin = strings.NewReader("hello\n")
	out, err := cmd.Output()
	return string(out), err
}

func TestB6CancelledMCPRegistrationNeverStartsUnderlyingProgram(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "not-started")
	alreadyCancelled, cancel := context.WithCancel(context.Background())
	cancel()
	// A live client cannot authorize a new MCP after cancellation, even if
	// it can still reach an existing socket.
	err := Run(alreadyCancelled, RunOptions{Socket: filepath.Join(t.TempDir(), "none.sock"), Server: "echo", ConfigDigest: testDigest, Program: os.Args[0], Args: []string{"-test.run=^TestLaunchRegistryHelperProcess$"}, Env: append(os.Environ(), "LAUNCH_REG_HELPER=mcp", "LAUNCH_TEST_MARKER="+marker), Stdin: strings.NewReader("hello\n"), Stdout: io.Discard, Stderr: io.Discard})
	if err == nil {
		t.Fatal("canceled launch erroneously succeeded")
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("canceled MCP child was started: %v", statErr)
	}
}
