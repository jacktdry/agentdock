package browser

import (
	"context"
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

// edgeProcessPreflight is a PRIVATE, read-only prerequisite, not an attestation
// of browser authentication, profile login, no-focus, or safe release. It must
// never be converted into ConnectorRuntimeStatus.Verified/Authenticated or used
// to mint connectorQualification/externalPeerIdentity without an independently
// reviewed runtime and transport-bound attestor.
type edgeProcessPreflight struct {
	pid         int
	endpoint    string
	executable  string
	userDataDir string
	startToken  string
	observedAt  time.Time
}

// edgeProcessProbe is OS evidence only. Its concrete macOS implementation
// executes bounded read-only ps/lsof. Unit tests inject only fixed strings.
type edgeProcessProbe interface {
	listeners(context.Context, int) (string, error)
	command(context.Context, int) (string, error)
	startToken(context.Context, int) (string, error)
	image(context.Context, int) (string, error)
}

type edgeListener struct {
	pid  int
	addr string
}

var errEdgeProcessUnqualified = errors.New("external Edge process identity unqualified")

// readEdgeProcessPreflight never probes CDP, browser tabs, login state, or the
// user's profile on disk. All raw OS evidence stays package-private.
func readEdgeProcessPreflight(ctx context.Context, canonicalEndpoint, expectedDataDir string, probe edgeProcessProbe) (edgeProcessPreflight, error) {
	fail := func() (edgeProcessPreflight, error) {
		return edgeProcessPreflight{}, errEdgeProcessUnqualified
	}
	if ctx == nil || probe == nil || ctx.Err() != nil ||
		browserpolicy.ValidateCanonicalBrowserEndpoint(canonicalEndpoint) != nil ||
		!filepath.IsAbs(expectedDataDir) || filepath.Clean(expectedDataDir) != expectedDataDir ||
		strings.TrimSpace(expectedDataDir) == "" {
		return fail()
	}
	u, err := url.Parse(canonicalEndpoint)
	if err != nil {
		return fail()
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		// 'localhost' aliases may resolve to multiple interfaces over time.
		// Keep process-owner qualification narrower than the catalog contract.
		return fail()
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return fail()
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	firstRaw, err := probe.listeners(probeCtx, port)
	if err != nil || probeCtx.Err() != nil {
		return fail()
	}
	first, ok := parseUniqueEdgeListener(firstRaw, port)
	if !ok {
		return fail()
	}
	start, err := probe.startToken(probeCtx, first.pid)
	if err != nil || strings.TrimSpace(start) == "" || probeCtx.Err() != nil {
		return fail()
	}
	args, err := probe.command(probeCtx, first.pid)
	if err != nil || probeCtx.Err() != nil ||
		!edgeCommandMatches(args, port, expectedDataDir) {
		return fail()
	}
	mapped, err := probe.image(probeCtx, first.pid)
	if err != nil || !mappedExecutableIsEdge(mapped, first.pid) || probeCtx.Err() != nil {
		return fail()
	}
	// Re-observe the OS listener and process incarnation after collecting
	// command-line evidence. This detects common PID reuse/restarts in the
	// observation window; it is NOT an atomic fence on future browser calls.
	lastRaw, err := probe.listeners(probeCtx, port)
	if err != nil || probeCtx.Err() != nil {
		return fail()
	}
	last, ok := parseUniqueEdgeListener(lastRaw, port)
	if !ok || last != first {
		return fail()
	}
	endStart, err := probe.startToken(probeCtx, first.pid)
	if err != nil || endStart != start || probeCtx.Err() != nil {
		return fail()
	}
	lastMapped, err := probe.image(probeCtx, first.pid)
	if err != nil || !mappedExecutableIsEdge(lastMapped, first.pid) || probeCtx.Err() != nil {
		return fail()
	}
	endArgs, err := probe.command(probeCtx, first.pid)
	if err != nil || endArgs != args || probeCtx.Err() != nil {
		return fail()
	}
	return edgeProcessPreflight{
		pid: first.pid, endpoint: canonicalEndpoint,
		executable:  "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		userDataDir: expectedDataDir, startToken: start, observedAt: time.Now().UTC(),
	}, nil
}

func parseUniqueEdgeListener(raw string, port int) (edgeListener, bool) {
	var current int
	var matches []edgeListener
	for _, line := range strings.Split(raw, "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			value, err := strconv.Atoi(line[1:])
			if err != nil || value <= 0 {
				return edgeListener{}, false
			}
			current = value
		case 'n':
			if current <= 0 {
				return edgeListener{}, false
			}
			host, p, err := net.SplitHostPort(line[1:])
			if err != nil {
				return edgeListener{}, false
			}
			n, err := strconv.Atoi(p)
			ip := net.ParseIP(host)
			if err != nil || n != port || ip == nil || !ip.IsLoopback() {
				// Reject wildcard/nonloopback listeners even when another
				// line from the same process happens to be loopback.
				return edgeListener{}, false
			}
			matches = append(matches, edgeListener{pid: current, addr: net.JoinHostPort(ip.String(), p)})
		}
	}
	if len(matches) != 1 {
		return edgeListener{}, false
	}
	return matches[0], true
}

func edgeCommandMatches(cmd string, port int, dataDir string) bool {
	executable := "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"
	trimmed := strings.TrimSpace(cmd)
	if !strings.HasPrefix(trimmed, executable+" ") {
		return false
	}
	// Do not infer the default data directory from an absent command-line flag.
	// Having matching args is a precondition, not proof of authenticated profile.
	matches := remoteDebuggingPortPattern.FindAllStringSubmatch(trimmed, -1)
	if len(matches) != 1 || matches[0][1] != strconv.Itoa(port) {
		return false
	}
	if !strings.Contains(trimmed, "--user-data-dir") || !filepath.IsAbs(dataDir) {
		return false
	}
	extracted := extractUserDataDir(trimmed)
	return extracted == dataDir
}

// mappedExecutableIsEdge checks the first macOS lsof text mapping rather
// than trusting argv[0]. It is still not a code-signing or login assertion.
func mappedExecutableIsEdge(raw string, pid int) bool {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 3 || lines[0] != "p"+strconv.Itoa(pid) {
		return false
	}
	for i := 1; i < len(lines)-1; i++ {
		if lines[i] == "ftxt" {
			return lines[i+1] == "n/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"
		}
	}
	return false
}
