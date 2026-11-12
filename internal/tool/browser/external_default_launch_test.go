package browser

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEdgeDefaultLaunchIsSeparateFromLegacyMCPProof(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, "Library", "Application Support", "Microsoft Edge")
	for _, suffix := range []string{"", " --no-startup-window", " --restart"} {
		command := testEdgeExecutable + suffix + "\n"
		if edgeCommandMatches(command, 9222, dir) {
			t.Fatal("legacy MCP command proof weakened")
		}
		f := edgeFixture()
		f.commandsRaw = []string{command, command}
		source, err := readEdgeProcessPreflightCommand(context.Background(), fixtureEdgeEndpoint, dir, f, true)
		if err != nil || !source.defaultLaunch || f.signatureCalls != 1 || f.liveSignatureCalls != 1 {
			t.Fatal("native startup prerequisite did not require both signing checks")
		}
	}
	for _, command := range []string{testEdgeExecutable + " --user-data-dir=/other", testEdgeExecutable + " --no-startup-window --other", testEdgeExecutable + " --restart --remote-debugging-port=9222", testEdgeExecutable + "-helper", testEdgeExecutable + "\r", testEdgeExecutable + "\n\n"} {
		if edgePreflightCommandMatches(command, 9222, dir, true) {
			t.Fatal("unknown native command admitted")
		}
	}
	if edgePreflightCommandMatches(testEdgeExecutable, 9222, "/other/edge", true) {
		t.Fatal("implicit arbitrary Profile root admitted")
	}
}
