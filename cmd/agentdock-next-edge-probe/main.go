//go:build darwin

// A separate source-built diagnostic: never starts a Core, installs an App,
// changes launchd, imports Stable credentials or enables a Browser route.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/uvwt/agentdock/internal/tool/browser"
)

func main() {
	profile := flag.Bool("owned-profile-probe", false, "read Edge version metadata in one owned hidden target; close only that target")
	sites := flag.Bool("read-only-sites", false, "check the four approved Oprueba/HomiPlay origins with separate owned hidden targets on one connection")
	workspace := flag.String("workspace", "", "exact approved company worktree; required for --read-only-sites")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments")
		os.Exit(2)
	}
	if *sites && *workspace == "" {
		fmt.Fprintln(os.Stderr, "--read-only-sites requires --workspace")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var result browser.NextEdgeProbeResult
	var err error
	if *sites {
		result, err = browser.ProbeNextEdgeSites(ctx, *workspace)
	} else {
		result, err = browser.ProbeNextEdge(ctx, *profile)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, "NEXT Edge diagnostic unqualified; no Browser route enabled")
		os.Exit(1)
	}
}
