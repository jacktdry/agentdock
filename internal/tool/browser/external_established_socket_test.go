package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeEdgeSocketProbe struct {
	raw                                   [2]string
	tokens                                [2]string
	connectionErr, tokenErr, signatureErr error
	calls                                 string
	cancel                                context.CancelFunc
	wait                                  bool
	expectedPort                          int
}

func (f *fakeEdgeSocketProbe) connections(ctx context.Context, pid, port int) (string, error) {
	i := strings.Count(f.calls, "c")
	f.calls += "c"
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > edgeSocketWindow || pid != 1245 || port != f.expectedPort {
		return "", errors.New("bad probe contract")
	}
	if f.wait {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return f.raw[i], f.connectionErr
}
func (f *fakeEdgeSocketProbe) startToken(context.Context, int) (string, error) {
	i := strings.Count(f.calls, "t")
	f.calls += "t"
	return f.tokens[i], f.tokenErr
}
func (f *fakeEdgeSocketProbe) liveSignature(context.Context, int) error {
	f.calls += "s"
	if f.cancel != nil {
		f.cancel()
	}
	return f.signatureErr
}

func edgeSocketFixture(t *testing.T) (*net.TCPConn, edgeProcessPreflight, *fakeEdgeSocketProbe) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	c, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	serverConn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverConn.Close() })
	source := edgeProcessPreflight{pid: 1245, endpoint: "ws://" + listener.Addr().String() + "/devtools/browser/fixture",
		executable: "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", userDataDir: filepath.Join(os.TempDir(), "agentdock-edge-socket-fixture"),
		startToken: "501:Fri Oct 9 12:00:00 2026", observedAt: time.Now()}
	raw := fmt.Sprintf("p1245\nf7\nn%s->%s\nTST=ESTABLISHED\nTQR=0\nTQS=0\n", c.RemoteAddr(), c.LocalAddr())
	return c.(*net.TCPConn), source, &fakeEdgeSocketProbe{raw: [2]string{raw, raw}, tokens: [2]string{source.startToken, source.startToken}, expectedPort: c.RemoteAddr().(*net.TCPAddr).Port}
}

func TestEdgeEstablishedSocketSuccess(t *testing.T) {
	c, source, probe := edgeSocketFixture(t)
	e, err := readEdgeEstablishedSocketEvidence(context.Background(), c, source, probe)
	if err != nil || e.pid != source.pid || e.startToken != source.startToken || e.server.String() != c.RemoteAddr().String() ||
		e.client.String() != c.LocalAddr().String() || e.observedAt.IsZero() || probe.calls != "ctscts" {
		t.Fatalf("evidence/check order incorrect: %v", err)
	}
}

func TestEdgeEstablishedSocketReject(t *testing.T) {
	for _, name := range []string{"nil-context", "nil-conn", "typed-nil-conn", "non-tcp", "nil-probe", "typed-nil-probe", "canceled", "stale", "future", "pid", "large-pid", "epoch", "executable", "profile-path", "profile-root", "profile-nul", "endpoint", "nonloopback", "missing", "ambiguous", "pid-mismatch", "direction", "restart", "signature", "raw-error", "token-error", "last-missing", "malformed", "huge", "cancel-during"} {
		t.Run(name, func(t *testing.T) {
			c, source, f := edgeSocketFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var contextArg context.Context = ctx
			var conn net.Conn = c
			var probe edgeSocketProbe = f
			switch name {
			case "nil-context":
				contextArg = nil
			case "nil-conn":
				conn = nil
			case "typed-nil-conn":
				conn = (*net.TCPConn)(nil)
			case "non-tcp":
				a, b := net.Pipe()
				defer a.Close()
				defer b.Close()
				conn = a
			case "nil-probe":
				probe = nil
			case "typed-nil-probe":
				probe = (*fakeEdgeSocketProbe)(nil)
			case "canceled":
				cancel()
			case "stale":
				source.observedAt = time.Now().Add(-3 * time.Second)
			case "future":
				source.observedAt = time.Now().Add(time.Second)
			case "pid":
				source.pid = 0
			case "large-pid":
				source.pid = 2147483648
			case "epoch":
				source.startToken = "canary"
			case "executable":
				source.executable = "/tmp/Edge"
			case "profile-path":
				source.userDataDir = "/private/../edge"
			case "profile-root":
				source.userDataDir = "/"
			case "profile-nul":
				source.userDataDir = "/private/edge\x00"
			case "endpoint":
				source.endpoint = "ws://127.0.0.1:1/devtools/browser/fixture"
			case "nonloopback":
				source.endpoint = "ws://192.0.2.1:1/devtools/browser/fixture"
			case "missing":
				f.raw[0] = "p1245\n"
			case "ambiguous":
				f.raw[0] += strings.ReplaceAll(strings.TrimPrefix(f.raw[0], "p1245\n"), "f7", "f8")
			case "pid-mismatch":
				f.raw[0] = strings.ReplaceAll(f.raw[0], "p1245", "p1246")
			case "direction":
				f.raw[0] = fmt.Sprintf("p1245\nf7\nn%s->%s\nTST=ESTABLISHED\n", c.LocalAddr(), c.RemoteAddr())
			case "restart":
				f.tokens[1] = "501:Fri Oct 9 12:00:01 2026"
			case "signature":
				f.signatureErr = errors.New("secret signer canary")
			case "raw-error":
				f.connectionErr = errors.New("secret process canary")
			case "token-error":
				f.tokenErr = errors.New("secret epoch canary")
			case "last-missing":
				f.raw[1] = "p1245\n"
			case "malformed":
				f.raw[0] += "TST=ESTABLISHED\n"
			case "huge":
				f.raw[0] = strings.Repeat("x", edgeSocketLimit+1)
			case "cancel-during":
				f.cancel = cancel
			}
			e, err := readEdgeEstablishedSocketEvidence(contextArg, conn, source, probe)
			if err != errEdgeProcessUnqualified || e != (edgeSocketEvidence{}) {
				t.Fatalf("failure exposed evidence or raw error: %v", err)
			}
		})
	}
}

func TestEdgeEstablishedSocketDeadline(t *testing.T) {
	c, source, f := edgeSocketFixture(t)
	f.wait = true
	start := time.Now()
	e, err := readEdgeEstablishedSocketEvidence(context.Background(), c, source, f)
	if err != errEdgeProcessUnqualified || e != (edgeSocketEvidence{}) || time.Since(start) > 3*time.Second {
		t.Fatal("deadline failed")
	}
}

func TestEdgeEstablishedSocketParser(t *testing.T) {
	for _, family := range []string{"v4", "v6"} {
		t.Run(family, func(t *testing.T) {
			s, c := "127.0.0.1:9222", "127.0.0.1:50100"
			if family == "v6" {
				s, c = "[::1]:9222", "[::1]:50100"
			}
			server, client := netip.MustParseAddrPort(s), netip.MustParseAddrPort(c)
			raw := "p1245\nf7\nn" + s + "->" + c + "\nTST=ESTABLISHED\nTQR=0\nTQS=0\n"
			if !edgeEstablishedSocketMatches(raw, 1245, server, client) {
				t.Fatal("canonical tuple rejected")
			}
			bad := []string{strings.ReplaceAll(raw, "ESTABLISHED", "LISTEN"), strings.ReplaceAll(raw, "TST=ESTABLISHED\n", ""), raw + "p1245\n", raw + "n" + s + "->" + c + "\n", raw + "TQR=0\n", strings.ReplaceAll(raw, "f7", "f07"), strings.ReplaceAll(raw, s, "*:9222"), strings.ReplaceAll(raw, s, "localhost:9222"), strings.ReplaceAll(raw, c, "127.0.0.1:0"), strings.ReplaceAll(raw, "TQS=0", "TQS=-1"), raw + "Xunknown\n", strings.TrimSuffix(raw, "\n"), strings.ReplaceAll(raw, s, "[::ffff:127.0.0.1]:9222"), strings.ReplaceAll(raw, s, "[::1%lo0]:9222"), strings.ReplaceAll(raw, s, "[0:0:0:0:0:0:0:1]:9222")}
			for i, text := range bad {
				if edgeEstablishedSocketMatches(text, 1245, server, client) {
					t.Fatalf("ambiguous record accepted: %d", i)
				}
			}
		})
	}
}
