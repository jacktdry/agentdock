//go:build darwin

package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type macEdgeProcessProbe struct{}

const (
	edgeSystemBundle = "/Applications/Microsoft Edge.app"
	// Apple-issued Developer ID, Microsoft team and Edge identifier are all mandatory.
	// Using codesign --verify alone would accept an unrelated signed executable.
	edgeSignerRequirement = `anchor apple generic and certificate leaf[subject.OU] = "UBF8T346G9" and identifier "com.microsoft.edgemac"`
)

// The bounded preflight checks the signed app seal and executable separately.
// Avoid --deep here: nested framework traversal exceeds the shared two-second
// process-evidence deadline on real Edge installations. This is intentionally
// not a recursive verification of nested helpers/frameworks.
// This checks the fixed on-disk system Edge bundle and executable without
// accessing any running browser or profile. It is NOT a code identity/peer fence
// on the live CDP transport; no raw signer output is returned to callers.
func (macEdgeProcessProbe) signature(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return errEdgeProcessUnqualified
	}
	for _, args := range [][]string{
		{"--verify", "--strict", "-R=" + edgeSignerRequirement, edgeSystemBundle},
		{"--verify", "--strict", "-R=" + edgeSignerRequirement, edgeSystemBundle + "/Contents/MacOS/Microsoft Edge"},
	} {
		if err := exec.CommandContext(ctx, "/usr/bin/codesign", args...).Run(); err != nil || ctx.Err() != nil {
			return errEdgeProcessUnqualified
		}
	}
	return nil
}

// OS commands are read-only, bounded by the caller's two-second context,
// and never print raw process command lines, profile paths or credentials.
func macProcessEvidenceCommand(ctx context.Context, executable string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, executable, args...).Output()
	if err != nil || len(out) == 0 || len(out) > 8192 {
		return "", errors.New("macOS process evidence unavailable")
	}
	return string(out), nil
}

func (macEdgeProcessProbe) listeners(ctx context.Context, port int) (string, error) {
	return macProcessEvidenceCommand(ctx, "/usr/sbin/lsof",
		"-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fpn")
}

func (macEdgeProcessProbe) command(ctx context.Context, pid int) (string, error) {
	return macProcessEvidenceCommand(ctx, "/bin/ps",
		"-p", strconv.Itoa(pid), "-o", "command=")
}

func (macEdgeProcessProbe) image(ctx context.Context, pid int) (string, error) {
	return macProcessEvidenceCommand(ctx, "/usr/sbin/lsof", "-nP", "-a", "-p", strconv.Itoa(pid), "-d", "txt", "-Fpn")
}

func (macEdgeProcessProbe) startToken(ctx context.Context, pid int) (string, error) {
	raw, err := macProcessEvidenceCommand(ctx, "/bin/ps",
		"-p", strconv.Itoa(pid), "-o", "uid=", "-o", "lstart=")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(raw)
	if len(fields) != 6 {
		return "", errors.New("macOS process epoch unavailable")
	}
	uid, err := strconv.Atoi(fields[0])
	if err != nil || uid != os.Getuid() {
		return "", errors.New("external browser process owner unavailable")
	}
	// UID + launch timestamp distinguish process restarts from a recycled PID.
	// This is a bounded observation, NOT a cryptographic/CDP session identity.
	return fmt.Sprintf("%d:%s", uid, strings.Join(fields[1:], " ")), nil
}

func inspectNativeEdgeProcess(ctx context.Context, canonicalEndpoint, expectedDataDir string) (edgeProcessPreflight, error) {
	return readEdgeProcessPreflight(ctx, canonicalEndpoint, expectedDataDir, macEdgeProcessProbe{})
}
