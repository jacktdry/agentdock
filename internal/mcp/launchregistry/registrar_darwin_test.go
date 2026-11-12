//go:build darwin

package launchregistry

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/uvwt/agentdock/internal/process"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestLaunchRegistryHelperProcess(t *testing.T) {
	mode := os.Getenv("LAUNCH_REG_HELPER")
	if mode == "" {
		return
	}
	switch mode {
	case "mcp":
		if os.Getenv("AGENTDOCK_NEXT_MCP_LAUNCH_SOCKET") != "" {
			os.Exit(21)
		}
		if marker := os.Getenv("LAUNCH_TEST_MARKER"); marker != "" {
			_ = os.WriteFile(marker, []byte("spawned"), 0600)
		}
		time.Sleep(180 * time.Millisecond) // registrar must observe the still-running child
		input := bufio.NewReader(os.Stdin)
		line, err := input.ReadString('\n')
		if err != nil {
			os.Exit(22)
		}
		_, _ = os.Stdout.WriteString("mcp:" + line)
		os.Exit(0) // Fixture MCP must not append Go test runner PASS to protocol stdout.
	case "wrapper":
		parent, err := process.ObserveBirthIdentity(context.Background(), os.Getppid())
		if err != nil {
			os.Exit(23)
		}
		if coreJSON := os.Getenv("LAUNCH_TEST_CORE_IDENTITY"); coreJSON != "" {
			if err := json.Unmarshal([]byte(coreJSON), &parent); err != nil {
				os.Exit(31)
			}
		}
		opts := RunOptions{Socket: os.Getenv("AGENTDOCK_NEXT_MCP_LAUNCH_SOCKET"), Server: os.Getenv("LAUNCH_TEST_SERVER"), ConfigDigest: os.Getenv("LAUNCH_TEST_DIGEST"),
			Program: os.Args[0], Args: []string{"-test.run=^TestLaunchRegistryHelperProcess$"},
			Env: append(os.Environ(), "LAUNCH_REG_HELPER=mcp"), Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, ExpectedRegistrar: parent}
		if err = Run(context.Background(), opts); err != nil {
			os.Exit(24)
		}
		os.Exit(0) // Wrapper fixture likewise cannot write Go test PASS to MCP stdout.
	case "adapter":
		cmd := exec.Command(os.Args[0], "-test.run=^TestLaunchRegistryHelperProcess$")
		cmd.Env = append(os.Environ(), "LAUNCH_REG_HELPER=appserver")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			os.Exit(32)
		}
		if err := os.WriteFile(os.Getenv("LAUNCH_TEST_APP_PID_FILE"), []byte(strconv.Itoa(cmd.Process.Pid)), 0600); err != nil {
			os.Exit(33)
		}
		if err := cmd.Wait(); err != nil {
			os.Exit(34)
		}
		os.Exit(0)
	case "appserver":
		// Hold native app-server until Core explicitly publishes its expected
		// Adapter/native scope, then create our dedicated stdio wrapper.
		if !waitFixtureFile(os.Getenv("LAUNCH_TEST_GO_FILE"), 5*time.Second) {
			os.Exit(35)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestLaunchRegistryHelperProcess$")
		cmd.Env = append(os.Environ(), "LAUNCH_REG_HELPER=wrapper")
		cmd.Stdin = strings.NewReader("hello\n")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			os.Exit(36)
		}
		if err := os.WriteFile(os.Getenv("LAUNCH_TEST_DONE_FILE"), []byte("done"), 0600); err != nil {
			os.Exit(37)
		}
		if !waitFixtureFile(os.Getenv("LAUNCH_TEST_RELEASE_FILE"), 5*time.Second) {
			os.Exit(38)
		}
		os.Exit(0)
	case "forge":
		raw, err := net.Dial("unix", os.Getenv("AGENTDOCK_NEXT_MCP_LAUNCH_SOCKET"))
		if err != nil {
			os.Exit(25)
		}
		c := raw.(*net.UnixConn)
		defer c.Close()
		_ = writeFrame(c, frame{Stage: "intent", Server: os.Getenv("LAUNCH_TEST_SERVER"), ConfigDigest: testDigest})
		ack, err := readFrame(bufio.NewReader(c))
		if err != nil || !validNonce(ack.Nonce) {
			os.Exit(26)
		}
		// Claim an unrelated or nonexistent child using actual kernel birth.
		forged, _ := process.ObserveBirthIdentity(context.Background(), os.Getppid())
		if os.Getenv("LAUNCH_TEST_FORGE_KIND") == "birth" {
			forged.PID = os.Getpid()
			forged.ParentPID = os.Getpid()
			forged.StartSec++
		}
		if os.Getenv("LAUNCH_TEST_FORGE_KIND") == "replay" {
			ack.Nonce = strings.Repeat("f", 64)
		}
		_ = writeFrame(c, frame{Stage: "started", Nonce: ack.Nonce, Child: &forged})
		_, err = readFrame(bufio.NewReader(c))
		if err == nil {
			os.Exit(27)
		}
	default:
		os.Exit(30)
	}
}

type acceptResult struct {
	observed Observation
	err      error
}

func openTestRegistrar(t *testing.T, expected process.BirthIdentity, server string) (string, <-chan acceptResult) {
	t.Helper()
	socket := shortTestRegistrarSocket(t)
	addr := &net.UnixAddr{Name: socket, Net: "unix"}
	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	ch := make(chan acceptResult, 1)
	go func() {
		conn, err := l.AcceptUnix()
		if err != nil {
			ch <- acceptResult{err: err}
			return
		}
		defer conn.Close()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		o, err := (Registrar{AppServer: expected, Server: server, ConfigDigest: testDigest}).Accept(c, conn)
		ch <- acceptResult{observed: o, err: err}
	}()
	return socket, ch
}

// The macOS sockaddr_un path limit is shorter than the nested t.TempDir
// paths allocated by GitHub Hosted runners. Keep only test-owned sockets in
// a private, short-lived directory; production socket locations are unchanged.
func shortTestRegistrarSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/private/tmp", "ad-mcp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove test socket directory: %v", err)
		}
	})
	socket := filepath.Join(dir, "m.sock")
	if len(socket) >= 100 {
		t.Fatalf("test Unix socket path is too long: %d bytes", len(socket))
	}
	return socket
}
func waitResult(t *testing.T, ch <-chan acceptResult) acceptResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(7 * time.Second):
		t.Fatal("registrar blocked")
		return acceptResult{}
	}
}
func spawnWrapper(t *testing.T, socket, server, digest, mode, forge, marker string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLaunchRegistryHelperProcess$")
	cmd.Env = append(os.Environ(), "LAUNCH_REG_HELPER="+mode, "LAUNCH_TEST_SERVER="+server, "LAUNCH_TEST_DIGEST="+digest, "AGENTDOCK_NEXT_MCP_LAUNCH_SOCKET="+socket, "LAUNCH_TEST_FORGE_KIND="+forge, "LAUNCH_TEST_MARKER="+marker)
	cmd.Stdin = strings.NewReader("hello\n")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := cmd.Run()
	return out.String(), err
}
func TestVerifiedStdioMCPLaunchAndNoProtocolPollution(t *testing.T) {
	ctx := context.Background()
	parent, err := process.ObserveBirthIdentity(ctx, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	socket, result := openTestRegistrar(t, parent, "echo")
	marker := filepath.Join(t.TempDir(), "created")
	out, runErr := spawnWrapper(t, socket, "echo", testDigest, "wrapper", "", marker)
	observed := waitResult(t, result)
	if runErr != nil || observed.err != nil || out != "mcp:hello\n" {
		t.Fatalf("wrapper=%v registrar=%v stdout=%q", runErr, observed.err, out)
	}
	if observed.observed.Wrapper.ParentPID != parent.PID || observed.observed.Child.ParentPID != observed.observed.Wrapper.PID || observed.observed.Child.PID == observed.observed.Wrapper.PID {
		t.Fatalf("invalid OS chain: %+v", observed.observed)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("child did not run: %v", err)
	}
}
func TestLaunchCannotSpawnWithoutVerifiedPreflight(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "should-not-exist")
	output, err := spawnWrapper(t, filepath.Join(t.TempDir(), "not-listening.sock"), "echo", testDigest, "wrapper", "", marker)
	if err == nil || output != "" {
		t.Fatalf("unverified launcher started: err=%v stdout=%q", err, output)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("preflight failure started child: %v", statErr)
	}
}
func TestRegistrarRejectsWrongParentBeforeSpawn(t *testing.T) {
	parent, err := process.ObserveBirthIdentity(context.Background(), os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	parent.StartSec++
	socket, result := openTestRegistrar(t, parent, "echo")
	marker := filepath.Join(t.TempDir(), "should-not-exist")
	_, runErr := spawnWrapper(t, socket, "echo", testDigest, "wrapper", "", marker)
	observed := waitResult(t, result)
	if runErr == nil || !errors.Is(observed.err, ErrUnavailable) {
		t.Fatalf("unauthorized launch: %v %v", runErr, observed.err)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("wrong parent created MCP: %v", statErr)
	}
}
func TestRegistrarRejectsWrongServerOrDigest(t *testing.T) {
	for _, tc := range []struct{ server, digest string }{{"other", testDigest}, {"echo", strings.Repeat("b", 64)}} {
		t.Run(tc.server+tc.digest[:1], func(t *testing.T) {
			parent, _ := process.ObserveBirthIdentity(context.Background(), os.Getpid())
			socket, result := openTestRegistrar(t, parent, "echo")
			marker := filepath.Join(t.TempDir(), "no-child")
			_, runErr := spawnWrapper(t, socket, tc.server, tc.digest, "wrapper", "", marker)
			observed := waitResult(t, result)
			if runErr == nil || !errors.Is(observed.err, ErrUnavailable) {
				t.Fatalf("accepted wrong config: %v %v", runErr, observed.err)
			}
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("bad config started child: %v", err)
			}
		})
	}
}
func TestRegistrarRefusesForgedChildAndNonce(t *testing.T) {
	for _, kind := range []string{"parent", "birth", "replay"} {
		t.Run(kind, func(t *testing.T) {
			parent, _ := process.ObserveBirthIdentity(context.Background(), os.Getpid())
			socket, result := openTestRegistrar(t, parent, "echo")
			_, runErr := spawnWrapper(t, socket, "echo", testDigest, "forge", kind, "")
			observed := waitResult(t, result)
			if runErr != nil || !errors.Is(observed.err, ErrUnavailable) {
				t.Fatalf("forgery accepted or test helper failed: %v %v", runErr, observed.err)
			}
		})
	}
}
func TestLaunchProtocolStrictFraming(t *testing.T) {
	for _, s := range []string{`{"stage":"intent","server":"echo","config_digest":"` + testDigest + `","unknown":true}` + "\n", `{"stage":"intent","stage":"other"}` + "\n", strings.Repeat("z", maxFrame+100) + "\n"} {
		_, err := readFrame(bufio.NewReader(strings.NewReader(s)))
		if err == nil {
			t.Fatalf("invalid frame accepted: %.45q", s)
		}
	}
	if got := strippedChildEnv([]string{"SAFE=1", "AGENTDOCK_NEXT_MCP_LAUNCH_SOCKET=secret", "AGENTDOCK_NEXT_MCP_LAUNCH_KEY=secret"}); len(got) != 1 || got[0] != "SAFE=1" {
		t.Fatalf("leaked env: %q", got)
	}
}

func waitFixtureFile(path string, timeout time.Duration) bool {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
