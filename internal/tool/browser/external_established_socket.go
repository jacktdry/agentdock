package browser

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const edgeSocketLimit = 8192
const edgeSocketWindow = 2 * time.Second

// PRIVATE non-atomic OS process evidence only: not login authentication, socket
// proof, a complete attestor, externalPeerIdentity, or a trusted route grant.
type edgeSocketEvidence struct {
	pid            int
	startToken     string
	client, server netip.AddrPort
	observedAt     time.Time
}

// Implementations must honor ctx; native commands and signature workers are
// bounded. No socket I/O, profile access, CDP discovery or relay wiring occurs.
type edgeSocketProbe interface {
	connections(context.Context, int) (string, error)
	startToken(context.Context, int) (string, error)
	liveSignature(context.Context, int) error
}

func edgeSocketStartTokenValid(token string) bool {
	uid, stamp, ok := strings.Cut(token, ":")
	n, err := strconv.ParseUint(uid, 10, 32)
	if !ok || err != nil || strconv.FormatUint(n, 10) != uid || len(token) > 64 {
		return false
	}
	t, err := time.Parse("Mon Jan 2 15:04:05 2006", stamp)
	return err == nil && t.Format("Mon Jan 2 15:04:05 2006") == stamp
}

func edgeSocketTuple(conn *net.TCPConn) (netip.AddrPort, netip.AddrPort, bool) {
	local, lok := conn.LocalAddr().(*net.TCPAddr)
	remote, rok := conn.RemoteAddr().(*net.TCPAddr)
	if !lok || !rok || local == nil || remote == nil || local.Zone != "" || remote.Zone != "" {
		return netip.AddrPort{}, netip.AddrPort{}, false
	}
	a, b := local.AddrPort(), remote.AddrPort()
	a = netip.AddrPortFrom(a.Addr().Unmap(), a.Port())
	b = netip.AddrPortFrom(b.Addr().Unmap(), b.Port())
	return a, b, a.IsValid() && b.IsValid() && a.Addr().IsLoopback() && b.Addr().IsLoopback() &&
		a.Port() != 0 && b.Port() != 0 && a != b && a.Addr().Is4() == b.Addr().Is4()
}

func readEdgeEstablishedSocketEvidence(ctx context.Context, conn net.Conn, source edgeProcessPreflight, probe edgeSocketProbe) (edgeSocketEvidence, error) {
	fail := func() (edgeSocketEvidence, error) { return edgeSocketEvidence{}, errEdgeProcessUnqualified }
	tcp, ok := conn.(*net.TCPConn)
	age := time.Since(source.observedAt)
	if ctx == nil || ctx.Err() != nil || !ok || tcp == nil || probe == nil || source.pid <= 0 ||
		int64(source.pid) > 2147483647 || !edgeSocketStartTokenValid(source.startToken) ||
		source.observedAt.IsZero() || age < 0 || age > edgeSocketWindow ||
		source.executable != "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge" ||
		len(source.userDataDir) > 4096 || !filepath.IsAbs(source.userDataDir) || filepath.Clean(source.userDataDir) != source.userDataDir ||
		source.userDataDir == "/" || strings.ContainsRune(source.userDataDir, 0) || strings.Count(source.userDataDir, "/") > edgeProfileMaxComponents ||
		len(source.endpoint) > edgeSocketLimit ||
		(!pinnedCDPEndpointValid(source.endpoint) && !(source.defaultLaunch && directCDPEndpointValid(source.endpoint))) {
		return fail()
	}
	if v := reflect.ValueOf(probe); v.Kind() == reflect.Ptr && v.IsNil() {
		return fail()
	}
	client, server, ok := edgeSocketTuple(tcp)
	u, err := url.Parse(source.endpoint)
	if !ok || err != nil || u.Host != server.String() {
		return fail()
	}
	probeCtx, cancel := context.WithTimeout(ctx, edgeSocketWindow)
	defer cancel()
	// Bracket independent epoch/signature observations with two OS socket reads.
	raw, err := probe.connections(probeCtx, source.pid)
	if err != nil || probeCtx.Err() != nil || !edgeEstablishedSocketMatches(raw, source.pid, server, client) {
		return fail()
	}
	for i := 0; i < 2; i++ {
		token, err := probe.startToken(probeCtx, source.pid)
		if err != nil || token != source.startToken || probeCtx.Err() != nil {
			return fail()
		}
		if probe.liveSignature(probeCtx, source.pid) != nil || probeCtx.Err() != nil {
			return fail()
		}
		if i == 0 {
			raw, err = probe.connections(probeCtx, source.pid)
			if err != nil || probeCtx.Err() != nil || !edgeEstablishedSocketMatches(raw, source.pid, server, client) {
				return fail()
			}
		}
	}
	endClient, endServer, ok := edgeSocketTuple(tcp)
	if !ok || endClient != client || endServer != server || probeCtx.Err() != nil {
		return fail()
	}
	return edgeSocketEvidence{source.pid, source.startToken, client, server, time.Now().UTC()}, nil
}

// Exact observed macOS -FpnT grammar. Unknown fields/formats fail closed.
// A record is delimited by f, not n; each needs one name and ESTABLISHED state.
func edgeEstablishedSocketMatches(raw string, pid int, server, client netip.AddrPort) bool {
	if len(raw) == 0 || len(raw) > edgeSocketLimit || !strings.HasSuffix(raw, "\n") {
		return false
	}
	lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
	if len(lines) < 4 || lines[0] != "p"+strconv.Itoa(pid) {
		return false
	}
	seenFD, seenTuple := map[string]bool{}, map[string]bool{}
	var name string
	var active, state, qr, qs bool
	matches := 0
	finish := func() bool {
		if !active || name == "" || !state || seenTuple[name] {
			return false
		}
		seenTuple[name] = true
		a, b, ok := strings.Cut(name, "->")
		local, lok := edgeSocketAddress(a)
		remote, rok := edgeSocketAddress(b)
		if !ok || !lok || !rok || local == remote || local.Addr().Is4() != remote.Addr().Is4() {
			return false
		}
		if local == server && remote == client {
			matches++
		}
		return true
	}
	for _, line := range lines[1:] {
		if len(line) < 2 || len(line) > 512 {
			return false
		}
		switch line[0] {
		case 'f':
			fd, err := strconv.ParseUint(line[1:], 10, 32)
			if err != nil || strconv.FormatUint(fd, 10) != line[1:] || seenFD[line] || len(seenFD) >= 64 || active && !finish() {
				return false
			}
			seenFD[line] = true
			active, state, qr, qs, name = true, false, false, false, ""
		case 'n':
			if !active || name != "" {
				return false
			}
			name = line[1:]
		case 'T':
			if !active {
				return false
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				return false
			}
			switch key {
			case "TST":
				if state || value != "ESTABLISHED" {
					return false
				}
				state = true
			case "TQR", "TQS":
				n, err := strconv.ParseUint(value, 10, 32)
				if err != nil || strconv.FormatUint(n, 10) != value {
					return false
				}
				if key == "TQR" {
					if qr {
						return false
					}
					qr = true
				} else {
					if qs {
						return false
					}
					qs = true
				}
			default:
				return false
			}
		default:
			return false
		}
	}
	return finish() && matches == 1
}

func edgeSocketAddress(raw string) (netip.AddrPort, bool) {
	a, err := netip.ParseAddrPort(raw)
	return a, err == nil && a.String() == raw && a.Addr().Zone() == "" && !a.Addr().Is4In6() && a.Addr().IsLoopback() && a.Port() != 0
}
