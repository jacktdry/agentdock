package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"time"

	"github.com/uvwt/agentdock/internal/mcpowneraudit"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, mcpowneraudit.Observe)) }

func run(args []string, out io.Writer, audit func(context.Context, mcpowneraudit.Options) mcpowneraudit.Report) int {
	o := mcpowneraudit.Options{}
	fs := flag.NewFlagSet("agentdock-next-mcp-owner-audit", flag.ContinueOnError)
	// Never echo rejected arguments, which may contain secrets.
	fs.SetOutput(io.Discard)
	fs.IntVar(&o.RootPID, "root-pid", 0, "explicit Codex app-server or adapter PID")
	fs.IntVar(&o.MaxNodes, "max-nodes", 256, "maximum observed nodes, including root (1..1024)")
	fs.IntVar(&o.MaxDepth, "max-depth", 8, "maximum ancestry depth (1..32)")
	fs.DurationVar(&o.Timeout, "timeout", 5*time.Second, "observation deadline (up to 10s)")
	code := 0
	var r mcpowneraudit.Report
	if err := fs.Parse(args); err == flag.ErrHelp {
		r = mcpowneraudit.Failure(o, "help: --root-pid PID [--max-nodes 256] [--max-depth 8] [--timeout 5s]")
	} else if err != nil || fs.NArg() != 0 || !o.Valid() {
		r = mcpowneraudit.Failure(o, "invalid_arguments")
		code = 2
	} else {
		r = audit(context.Background(), o)
		if r.Status != "observed" {
			code = 1
		}
	}
	if json.NewEncoder(out).Encode(r) != nil {
		return 1
	}
	return code
}
