package main

import (
	"errors"
	"testing"

	"github.com/uvwt/agentdock/internal/mcp/launchregistry"
)

func TestNextOnlyLauncherFailsClosedOnMissingOrMalformedCoreIdentity(t *testing.T) {
	base := []string{"--socket", "/tmp/nonexistent-registry.sock", "--server", "echo", "--config-digest", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--", "/usr/bin/true"}
	for _, value := range []string{"", `{"PID":23,"PID":24}`, `{}`, `{"PID":0}`} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("AGENTDOCK_NEXT_MCP_LAUNCH_CORE_IDENTITY", value)
			if err := execute(base); !errors.Is(err, launchregistry.ErrUnavailable) {
				t.Fatalf("untrusted launcher configuration ran a child: %v", err)
			}
		})
	}
	t.Setenv("AGENTDOCK_NEXT_MCP_LAUNCH_CORE_IDENTITY", `{"PID":12345}`)
	if err := execute([]string{"--server", "echo"}); !errors.Is(err, launchregistry.ErrUnavailable) {
		t.Fatalf("missing underlying program accepted: %v", err)
	}
}
