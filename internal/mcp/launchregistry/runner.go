package launchregistry

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/uvwt/agentdock/internal/process"
)

// RunOptions are passed only by a Next-authorized, explicit per-server stdio
// configuration. There is NO global Codex CLI config edit or implicit launch.
type RunOptions struct {
	Socket            string
	Server            string
	ConfigDigest      string
	Program           string
	Args              []string
	Env               []string
	Stdin             io.Reader
	Stdout, Stderr    io.Writer
	ExpectedRegistrar process.BirthIdentity
}

// Run first obtains an authenticated preflight for its *wrapper process*, then
// launches one MCP child and reports its kernel birth over that same socket.
// The wrapper owns its own cmd.Wait; no arbitrary PID termination is exposed.
// Diagnostics sent via Unix socket are NEVER written to MCP stdout.
func Run(ctx context.Context, opts RunOptions) error {
	if ctx == nil || ctx.Err() != nil || !validServer(opts.Server) || !validDigest(opts.ConfigDigest) || opts.Socket == "" || opts.Program == "" || opts.Stdin == nil || opts.Stdout == nil || opts.Stderr == nil {
		return ErrUnavailable
	}
	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	raw, err := (&net.Dialer{}).DialContext(dialCtx, "unix", opts.Socket)
	if err != nil {
		return ErrUnavailable
	}
	conn, ok := raw.(*net.UnixConn)
	if !ok {
		_ = raw.Close()
		return ErrUnavailable
	}
	defer conn.Close()
	if !verifyRegistrarPeer(ctx, conn, opts.ExpectedRegistrar) {
		return ErrUnavailable
	}
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	if writeFrame(conn, frame{Stage: "intent", Server: opts.Server, ConfigDigest: opts.ConfigDigest}) != nil {
		return ErrUnavailable
	}
	reader := bufio.NewReaderSize(conn, maxFrame)
	accepted, err := readFrame(reader)
	if err != nil || accepted.Stage != "accepted" || !validNonce(accepted.Nonce) || accepted.Child != nil || accepted.Server != "" || accepted.ConfigDigest != "" {
		return ErrUnavailable
	}
	cmd := exec.Command(opts.Program, opts.Args...)
	cmd.Env = strippedChildEnv(opts.Env)
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	// Keep the native app-server process-group semantics rather than assigning
	// a new unmanaged group; on normal exit the wrapper waits for its own child.
	if err := cmd.Start(); err != nil {
		return err
	}
	child, observedErr := process.ObserveBirthIdentity(ctx, cmd.Process.Pid)
	if observedErr != nil || child.ParentPID != os.Getpid() || child.EffectiveUID != uint32(os.Geteuid()) {
		// Continue waiting for *our own* child rather than abandoning it on a
		// diagnostic failure. This call grants no orphan-recovery authority.
		_ = cmd.Wait()
		return ErrUnavailable
	}
	reported := writeFrame(conn, frame{Stage: "started", Nonce: accepted.Nonce, Child: &child})
	var recorded frame
	if reported == nil {
		recorded, reported = readFrame(reader)
	}
	if reported != nil || recorded.Stage != "recorded" || recorded.Nonce != accepted.Nonce {
		// Cannot unsafely cancel another client's task; wait for the child and
		// return registration failure even if its exit code was successful.
		_ = cmd.Wait()
		return ErrUnavailable
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}
