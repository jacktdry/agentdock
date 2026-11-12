//go:build darwin

package mcpowneraudit

import (
	"context"
	"os"
	"testing"
)

func TestDarwinKernelMetadataOwnProcess(t *testing.T) {
	// Self-only root; no existing adapter or user MCP process is targeted.
	o := fixtureOptions()
	o.RootPID = os.Getpid()
	r := Observe(context.Background(), o)
	if r.Status != "observed" || len(r.Nodes) != 1 || r.Nodes[0].PID != os.Getpid() || r.Nodes[0].UID != uint32(os.Geteuid()) || r.Nodes[0].BirthSec <= 0 || r.Nodes[0].Owner != "owner_unverified" {
		t.Fatalf("kernel self observation: %+v", r)
	}
}
