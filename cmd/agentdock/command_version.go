package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/uvwt/agentdock/internal/buildinfo"
)

func printVersion(output io.Writer) {
	info := buildinfo.Current()
	fmt.Fprintf(output, "AgentDock v%s\n", strings.TrimPrefix(info.Version, "v"))
	fmt.Fprintf(output, "commit: %s\n", info.Commit)
	fmt.Fprintf(output, "built: %s\n", info.BuildDate)
	fmt.Fprintf(output, "distribution: %s\n", info.Distribution)
	fmt.Fprintf(output, "update-policy: %s\n", info.UpdatePolicy)
	fmt.Fprintf(output, "go: %s\n", info.GoVersion)
	fmt.Fprintf(output, "platform: %s\n", info.Platform)
}
