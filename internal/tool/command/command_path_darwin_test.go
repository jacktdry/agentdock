//go:build darwin

package command

import (
	"github.com/uvwt/agentdock/internal/commandpath"
	"testing"
)

func TestPlatformCommandPathAddsUserAndHomebrewDirectories(t *testing.T) {
	got := commandpath.Path("/usr/bin:/opt/homebrew/bin:/bin:/usr/local/bin", "/Users/example")
	want := "/Users/example/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
	if got != want {
		t.Fatalf("commandpath.Path() = %q, want %q", got, want)
	}
}

func TestPlatformCommandPathWithoutHomeStillKeepsSystemPath(t *testing.T) {
	got := commandpath.Path("/usr/bin:/bin:/usr/sbin:/sbin", "")
	want := "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	if got != want {
		t.Fatalf("commandpath.Path() = %q, want %q", got, want)
	}
}
