// Command agentdock-next-mcp-launch is a source-only, opt-in Codex ACP
// per-server stdio wrapper. It is NOT installed, auto-configured, or connected
// to the Next Core. A verified registrar must be configured separately.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/uvwt/agentdock/internal/mcp/launchregistry"
	"github.com/uvwt/agentdock/internal/process"
)

func main() {
	if err := execute(os.Args[1:]); err != nil {
		// Do not leak secret config argv/env/PIDs or the underlying transport error.
		_, _ = fmt.Fprintln(os.Stderr, "AgentDock Next MCP launch unavailable")
		os.Exit(1)
	}
}

func execute(args []string) error {
	fs := flag.NewFlagSet("agentdock-next-mcp-launch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var socket, server, digest string
	fs.StringVar(&socket, "socket", "", "Next-owned private Unix socket")
	fs.StringVar(&server, "server", "", "logical MCP server ID")
	fs.StringVar(&digest, "config-digest", "", "SHA-256 of approved server configuration")
	if err := fs.Parse(args); err != nil {
		return launchregistry.ErrUnavailable
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return launchregistry.ErrUnavailable
	}
	// Trusted Next Core birth snapshot is passed as *private launcher metadata*.
	// The child environment is independently stripped in Run.
	var expected process.BirthIdentity
	value := os.Getenv("AGENTDOCK_NEXT_MCP_LAUNCH_CORE_IDENTITY")
	dec := json.NewDecoder(strings.NewReader(value))
	dec.DisallowUnknownFields()
	if value == "" || dec.Decode(&expected) != nil || expected.PID <= 0 {
		return launchregistry.ErrUnavailable
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return launchregistry.ErrUnavailable
	}
	return launchregistry.Run(context.Background(), launchregistry.RunOptions{
		Socket: socket, Server: server, ConfigDigest: digest, Program: rest[0], Args: rest[1:],
		Env: os.Environ(), Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, ExpectedRegistrar: expected,
	})
}
