//go:build darwin

package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// The existing repo uses ps for macOS process discovery. x/sys exposes kinfo
// process metadata but not live RSS; fixed numeric columns avoid command parsing
// and a cgo/libproc dependency. No shell or user-supplied arguments are used.
func observeProcesses(ctx context.Context) ([]observedProcess, error) {
	cmd := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,ppid=,stat=,rss=")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("observe ps: %w", err)
	}
	return parsePS(string(output))
}
