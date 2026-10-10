//go:build darwin

// Source-only prototype: NOT packaged, registered with launchd, or used
// by any production AgentDock Next runtime. Strictly read-only PID observer.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/uvwt/agentdock/internal/process"
)

func main() {
	var req process.IndependentExitRequest
	dec := json.NewDecoder(io.LimitReader(os.Stdin, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		os.Exit(1)
	}
	// Do not silently accept a second request or trailing garbage.
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		os.Exit(1)
	}
	// A single handshake request, never a long-running control channel.
	if err := process.WatchIndependentWorkerExit(context.Background(), req, func() error {
		_, err := fmt.Fprintln(os.Stdout, "OBSERVER_ARMED")
		return err
	}); err != nil {
		os.Exit(1)
	}
}
