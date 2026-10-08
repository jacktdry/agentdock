package browser

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testEdgeExecutable = "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"

func TestEdgeCommandMatchesConservativeArguments(t *testing.T) {
	const dir = "/tmp/Edge Profile"
	for i, args := range []string{
		`--remote-debugging-port=9222 --user-data-dir="/tmp/Edge Profile"`,
		`--user-data-dir '/tmp/Edge Profile' --remote-debugging-port 9222`,
		`--remote-debugging-port="9222" --user-data-dir='/tmp/Edge Profile'`,
		`--remote-debugging-port '9222' --user-data-dir /tmp/Edge\ Profile`,
		`--no-first-run --remote-debugging-port 9222 --user-data-dir=/tmp/Edge\ Profile --lang=en`,
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			if !edgeCommandMatches("  "+testEdgeExecutable+"  "+args+" \n", 9222, dir) {
				t.Fatal("unambiguous explicit port/profile fixture rejected")
			}
		})
	}
}

func TestEdgeCommandMatchesRejectsSpoofedOrAmbiguousArguments(t *testing.T) {
	const valid = `--remote-debugging-port=9222 --user-data-dir=/tmp/edge-fixture`
	for i, args := range []string{
		`--x--remote-debugging-port=9222 --user-data-dir=/tmp/edge-fixture`,
		`--remote-debugging-port-suffix=9222 --user-data-dir=/tmp/edge-fixture`,
		`--remote-debugging-port=9222suffix --user-data-dir=/tmp/edge-fixture`,
		`--remote-debugging-port=09222 --user-data-dir=/tmp/edge-fixture`,
		`--remote-debugging-port=9223 --user-data-dir=/tmp/edge-fixture`,
		`--remote-debugging-port=9222 --x--user-data-dir=/tmp/edge-fixture`,
		`--remote-debugging-port=9222 --user-data-dir-suffix=/tmp/edge-fixture`,
		`"--remote-debugging-port=9222" --user-data-dir=/tmp/edge-fixture`,
		`--remote-debugging-port=9222 '--user-data-dir=/tmp/edge-fixture'`,
		`--note="--remote-debugging-port=9222" --user-data-dir=/tmp/edge-fixture`,
		`--remote-debugging-port=9222 --note='--user-data-dir=/tmp/edge-fixture'`,
		`--remote-debugging-port=9222 --user-data-dir=/tmp/edge-fixture-suffix`,
		`--remote-debugging-port=9222 --user-data-dir=/tmp/../tmp/edge-fixture`,
		`--remote-debugging-port=9222 --user-data-dir=tmp/edge-fixture`,
		`--remote-debugging-port --user-data-dir=/tmp/edge-fixture`,
		`--remote-debugging-port=9222 --user-data-dir`,
		`--remote-debugging-port=9222 --user-data-dir=`,
		`--remote-debugging-port=9222 --user-data-dir=""`,
		`--remote-debugging-port=9222 --user-data-dir='--hidden'`,
		`--remote-debugging-port="9222 --hidden" --user-data-dir=/tmp/edge-fixture`,
		`--remote-debugging-port=9222 --user-data-dir='/tmp/edge-fixture`,
		`--remote-debugging-port=9222 --user-data-dir="/tmp/edge-fixture`,
		`--remote-debugging-port=9222 --user-data-dir=/tmp/edge-'fixture'`,
		`--remote-debugging-port=9222 --user-data-dir='/tmp/edge-'fixture`,
		`--remote-debugging-port=9222 --user-data-dir=/tmp/edge\-fixture`,
		`--remote-debugging-port=9222 --user-data-dir="/tmp/edge\ fixture"`,
		`--remote-debugging-port=9222 --user-data-dir=/tmp/edge-fixture\`,
		valid + ` --remote-debugging-port=9222`,
		valid + ` --remote-debugging-port 9223`,
		valid + ` --user-data-dir=/tmp/edge-fixture`,
		valid + ` --user-data-dir /tmp/other`,
		valid + ` --note="text --remote-debugging-port=9223"`,
		valid + ` --note='text --user-data-dir=/tmp/other'`,
		valid + ` -- --user-data-dir=/tmp/other`,
		valid + ` --note --remote-debugging-port=9222`,
		valid + ` --note=--hidden`,
		valid + ` --lang=en --lang=zh`,
		valid + "\t--hidden", valid + "\x00", valid + "\r", valid + "\n\n",
		valid + "\n--hidden", valid + "\x7f", valid + "\u200b--hidden",
		valid + "\u00a0--hidden", valid + "\xff",
		valid + strings.Repeat(" ", 8193),
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			// Invalid text must stop before image/signer work and expose no evidence.
			f := edgeFixture()
			f.commandsRaw[0] = testEdgeExecutable + " " + args
			e, err := readEdgeProcessPreflight(context.Background(),
				"ws://127.0.0.1:9222/devtools/browser/candidate", "/tmp/edge-fixture", f)
			if !errors.Is(err, errEdgeProcessUnqualified) || e != (edgeProcessPreflight{}) || f.imageCalls != 0 || f.signatureCalls != 0 {
				t.Fatal("ambiguous argv advanced preflight or exposed evidence")
			}
		})
	}
	for _, cmd := range []string{testEdgeExecutable + "-helper " + valid, `"` + testEdgeExecutable + `" ` + valid, "\n" + testEdgeExecutable + " " + valid} {
		if edgeCommandMatches(cmd, 9222, "/tmp/edge-fixture") {
			t.Fatal("non-fixed executable accepted")
		}
	}
	for _, dir := range []string{"", "relative", "/tmp/../tmp/edge-fixture"} {
		if edgeCommandMatches(testEdgeExecutable+" "+valid, 9222, dir) {
			t.Fatal("noncanonical expected path accepted")
		}
	}
	for _, port := range []int{0, -1, 65536} {
		if edgeCommandMatches(testEdgeExecutable+" "+valid, port, "/tmp/edge-fixture") {
			t.Fatal("invalid expected port accepted")
		}
	}
}

type fakeEdgeProcessProbe struct {
	listenerCalls  int
	commandCalls   int
	startCalls     int
	imageCalls     int
	signatureCalls int
	signatureErr   error
	cancelOnSign   bool
	listenersRaw   []string
	commandsRaw    []string
	startsRaw      []string
	imagesRaw      []string
	err            error
	cancel         context.CancelFunc
}

func (f *fakeEdgeProcessProbe) listeners(ctx context.Context, _ int) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	i := f.listenerCalls
	f.listenerCalls++
	if i >= len(f.listenersRaw) {
		return "", errors.New("missing fake listener fixture")
	}
	if f.cancel != nil && i == 0 {
		f.cancel()
	}
	return f.listenersRaw[i], nil
}
func (f *fakeEdgeProcessProbe) command(ctx context.Context, _ int) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	i := f.commandCalls
	f.commandCalls++
	if i >= len(f.commandsRaw) {
		return "", errors.New("missing fake process fixture")
	}
	return f.commandsRaw[i], nil
}
func (f *fakeEdgeProcessProbe) startToken(ctx context.Context, _ int) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	i := f.startCalls
	f.startCalls++
	if i >= len(f.startsRaw) {
		return "", errors.New("missing fake start fixture")
	}
	return f.startsRaw[i], nil
}

func (f *fakeEdgeProcessProbe) image(ctx context.Context, _ int) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	i := f.imageCalls
	f.imageCalls++
	if i >= len(f.imagesRaw) {
		return "", errors.New("missing image fixture")
	}
	return f.imagesRaw[i], nil
}

func (f *fakeEdgeProcessProbe) signature(ctx context.Context) error {
	f.signatureCalls++
	if f.cancelOnSign && f.cancel != nil {
		f.cancel()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return f.signatureErr
}

func edgeFixture() *fakeEdgeProcessProbe {
	cmd := testEdgeExecutable + " --remote-debugging-port=9222 --user-data-dir=/tmp/edge-fixture"
	raw := "p1245\nf7\nn127.0.0.1:9222\n"
	return &fakeEdgeProcessProbe{
		listenersRaw: []string{raw, raw},
		commandsRaw:  []string{cmd, cmd},
		startsRaw:    []string{"501:Fri Oct 9 2026", "501:Fri Oct 9 2026"},
		imagesRaw:    []string{"p1245\nftxt\nn" + testEdgeExecutable + "\n", "p1245\nftxt\nn" + testEdgeExecutable + "\n"},
	}
}

func TestEdgeProcessPreflightMatchesOnlyOSProcessEvidence(t *testing.T) {
	f := edgeFixture()
	p, err := readEdgeProcessPreflight(context.Background(),
		"ws://127.0.0.1:9222/devtools/browser/candidate", "/tmp/edge-fixture", f)
	if err != nil {
		t.Fatal(err)
	}
	if p.pid != 1245 || p.userDataDir != "/tmp/edge-fixture" || p.startToken != f.startsRaw[0] ||
		p.endpoint != "ws://127.0.0.1:9222/devtools/browser/candidate" ||
		p.executable != testEdgeExecutable || time.Since(p.observedAt) > time.Second {
		t.Fatal("read-only process preflight lost source identity")
	}
	if f.listenerCalls != 2 || f.startCalls != 2 || f.commandCalls != 2 || f.imageCalls != 2 || f.signatureCalls != 1 {
		t.Fatal("did not recheck process evidence after observation")
	}
}

func TestEdgeProcessPreflightRejectsAllAmbiguousEvidence(t *testing.T) {
	cases := map[string]func(*fakeEdgeProcessProbe){
		"unrelated executable": func(f *fakeEdgeProcessProbe) {
			f.commandsRaw[0] = strings.Replace(f.commandsRaw[0], testEdgeExecutable, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", 1)
		},
		"unsigned positional lookalike": func(f *fakeEdgeProcessProbe) {
			f.commandsRaw[0] = "/tmp/Microsoft Edge --remote-debugging-port=9222 --user-data-dir=/tmp/edge-fixture"
		},
		"wrong declared port": func(f *fakeEdgeProcessProbe) {
			f.commandsRaw[0] = strings.Replace(f.commandsRaw[0], "9222", "9223", 1)
		},
		"duplicated port flags": func(f *fakeEdgeProcessProbe) {
			f.commandsRaw[0] += " --remote-debugging-port=9222"
		},
		"missing remote flag": func(f *fakeEdgeProcessProbe) {
			f.commandsRaw[0] = testEdgeExecutable + " --user-data-dir=/tmp/edge-fixture"
		},
		"other data dir": func(f *fakeEdgeProcessProbe) {
			f.commandsRaw[0] = strings.Replace(f.commandsRaw[0], "/tmp/edge-fixture", "/tmp/other", 1)
		},
		"no explicit profile path": func(f *fakeEdgeProcessProbe) {
			f.commandsRaw[0] = testEdgeExecutable + " --remote-debugging-port=9222"
		},
		"process start rollover": func(f *fakeEdgeProcessProbe) {
			f.startsRaw[1] = "501:Fri Oct 9 2027"
		},
		"command changed": func(f *fakeEdgeProcessProbe) {
			f.commandsRaw[1] += " --renderer-type=test"
		},
		"wrong mapped executable": func(f *fakeEdgeProcessProbe) {
			f.imagesRaw[0] = "p1245\nftxt\nn/Applications/Google Chrome.app/Contents/MacOS/Google Chrome\n"
		},
		"mapped image changed":   func(f *fakeEdgeProcessProbe) { f.imagesRaw[1] = "p1245\nftxt\nn/tmp/Microsoft Edge\n" },
		"mapped image wrong PID": func(f *fakeEdgeProcessProbe) { f.imagesRaw[0] = "p1320\nftxt\nn" + testEdgeExecutable + "\n" },
		"listener PID replaced": func(f *fakeEdgeProcessProbe) {
			f.listenersRaw[1] = "p1320\nf7\nn127.0.0.1:9222\n"
		},
		"listener rebound": func(f *fakeEdgeProcessProbe) {
			f.listenersRaw[1] = "p1245\nf7\nn[::1]:9222\n"
		},
		"wildcard listener": func(f *fakeEdgeProcessProbe) {
			f.listenersRaw[0] = "p1245\nf7\nn*:9222\n"
		},
		"nonloopback": func(f *fakeEdgeProcessProbe) {
			f.listenersRaw[0] = "p1245\nf7\nn0.0.0.0:9222\n"
		},
		"two owners": func(f *fakeEdgeProcessProbe) {
			f.listenersRaw[0] += "p1320\nf7\nn127.0.0.1:9222\n"
		},
		"dual bind ambiguity": func(f *fakeEdgeProcessProbe) {
			f.listenersRaw[0] += "p1245\nf8\nn[::1]:9222\n"
		},
		"no listener": func(f *fakeEdgeProcessProbe) {
			f.listenersRaw[0] = ""
		},
		"bad lsof row": func(f *fakeEdgeProcessProbe) {
			f.listenersRaw[0] = "n127.0.0.1:9222\n"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := edgeFixture()
			mutate(f)
			evidence, err := readEdgeProcessPreflight(context.Background(),
				"ws://127.0.0.1:9222/devtools/browser/candidate", "/tmp/edge-fixture", f)
			if !errors.Is(err, errEdgeProcessUnqualified) || evidence != (edgeProcessPreflight{}) {
				t.Fatalf("untrusted process was accepted: %+v %v", evidence, err)
			}
		})
	}
}

func TestEdgeProcessPreflightRejectsUnsignedAndChangedDuringSigning(t *testing.T) {
	t.Run("missing or invalid Microsoft signature", func(t *testing.T) {
		f := edgeFixture()
		f.signatureErr = errors.New("codesign failed: sensitive detail must not escape")
		e, err := readEdgeProcessPreflight(context.Background(),
			"ws://127.0.0.1:9222/devtools/browser/candidate", "/tmp/edge-fixture", f)
		if !errors.Is(err, errEdgeProcessUnqualified) || e != (edgeProcessPreflight{}) ||
			strings.Contains(err.Error(), "sensitive") || f.signatureCalls != 1 {
			t.Fatal("unsigned or wrong-team Edge was accepted, or error leaked details")
		}
	})
	t.Run("listener changed across signature", func(t *testing.T) {
		f := edgeFixture()
		f.listenersRaw[1] = "p4321\nf7\nn127.0.0.1:9222\n"
		e, err := readEdgeProcessPreflight(context.Background(),
			"ws://127.0.0.1:9222/devtools/browser/candidate", "/tmp/edge-fixture", f)
		if !errors.Is(err, errEdgeProcessUnqualified) || e != (edgeProcessPreflight{}) || f.signatureCalls != 1 {
			t.Fatal("process turnover during signature was accepted")
		}
	})
	t.Run("signature canceled", func(t *testing.T) {
		f := edgeFixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f.cancel = cancel
		f.cancelOnSign = true
		e, err := readEdgeProcessPreflight(ctx,
			"ws://127.0.0.1:9222/devtools/browser/candidate", "/tmp/edge-fixture", f)
		if !errors.Is(err, errEdgeProcessUnqualified) || e != (edgeProcessPreflight{}) || f.listenerCalls != 1 {
			t.Fatal("signature cancellation was accepted or continued to OS recheck")
		}
	})
	t.Run("invalid endpoint never verifies signer", func(t *testing.T) {
		f := edgeFixture()
		_, _ = readEdgeProcessPreflight(context.Background(),
			"ws://localhost:9222/devtools/browser/candidate", "/tmp/edge-fixture", f)
		if f.signatureCalls != 0 {
			t.Fatal("invalid endpoint caused on-disk signature work")
		}
	})
}

func TestEdgeProcessPreflightRejectsUnsafeInputsAndCancellation(t *testing.T) {
	for _, endpoint := range []string{
		"ws://localhost:9222/devtools/browser/candidate",
		"ws://192.168.1.100:9222/devtools/browser/candidate",
		"ws://127.0.0.1:9222/devtools/page/candidate",
		"ws://127.0.0.1:9222/devtools/browser/candidate?token=sensitive",
		"ws://127.0.0.1:9222/devtools/browser/candidate#secret",
	} {
		_, err := readEdgeProcessPreflight(context.Background(), endpoint, "/tmp/edge-fixture", edgeFixture())
		if !errors.Is(err, errEdgeProcessUnqualified) {
			t.Fatal("noncanonical/broad endpoint unexpectedly trusted", endpoint, err)
		}
	}
	for _, directory := range []string{"", "../edge", "/tmp/../tmp/edge-fixture"} {
		_, err := readEdgeProcessPreflight(context.Background(), "ws://127.0.0.1:9222/devtools/browser/candidate", directory, edgeFixture())
		if !errors.Is(err, errEdgeProcessUnqualified) {
			t.Fatal("unsafe profile dir", directory)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := edgeFixture()
	_, err := readEdgeProcessPreflight(ctx, "ws://127.0.0.1:9222/devtools/browser/candidate", "/tmp/edge-fixture", f)
	if !errors.Is(err, errEdgeProcessUnqualified) || f.listenerCalls != 0 {
		t.Fatal("canceled evidence probe continued")
	}
	f2 := edgeFixture()
	ctx2, cancel2 := context.WithCancel(context.Background())
	f2.cancel = cancel2
	defer cancel2()
	_, err = readEdgeProcessPreflight(ctx2, "ws://127.0.0.1:9222/devtools/browser/candidate", "/tmp/edge-fixture", f2)
	if !errors.Is(err, errEdgeProcessUnqualified) {
		t.Fatal("post-observation cancel failed closed")
	}
}

func TestEdgeProcessPreflightExposesNoAuthOrRouteGrant(t *testing.T) {
	f := edgeFixture()
	e, err := readEdgeProcessPreflight(context.Background(), "ws://127.0.0.1:9222/devtools/browser/candidate", "/tmp/edge-fixture", f)
	if err != nil {
		t.Fatal(err)
	}
	serialized, marshalErr := json.Marshal(e)
	if marshalErr != nil || string(serialized) != "{}" {
		t.Fatal("private OS process evidence escaped through JSON")
	}
	status := unqualifiedRuntimeStatus()
	planner, scope := plannerFixture(t, testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) {
		return status, nil
	}))
	d, err := planner.Resolve(context.Background(), scope, nil)
	assertRouteCode(t, err, ErrRequiredRouteUnavailable)
	if d != (RouteDecision{}) {
		t.Fatal("process preflight qualified Edge or minted a route grant")
	}
}

func TestUniqueEdgeListenerParser(t *testing.T) {
	for _, port := range []int{1, 9222, 65535} {
		raw := "p500\nf7\nn127.0.0.1:" + strconv.Itoa(port) + "\n"
		got, ok := parseUniqueEdgeListener(raw, port)
		if !ok || got.pid != 500 {
			t.Fatal("valid loopback owner rejected", port)
		}
	}
}
