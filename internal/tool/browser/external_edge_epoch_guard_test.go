package browser

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

const fixtureEdgeEndpoint = "ws://127.0.0.1:9222/devtools/browser/candidate"

func expandedEdgeEpochProbe(t *testing.T, observations int) (edgeProcessPreflight, *fakeEdgeProcessProbe) {
	t.Helper()
	f := edgeFixture()
	for i := 1; i < observations; i++ {
		f.listenersRaw = append(f.listenersRaw, f.listenersRaw[0], f.listenersRaw[0])
		f.startsRaw = append(f.startsRaw, f.startsRaw[0], f.startsRaw[0])
		f.commandsRaw = append(f.commandsRaw, f.commandsRaw[0], f.commandsRaw[0])
		f.imagesRaw = append(f.imagesRaw, f.imagesRaw[0], f.imagesRaw[0])
	}
	src, e := readEdgeProcessPreflight(context.Background(), fixtureEdgeEndpoint, "/tmp/edge-fixture", f)
	if e != nil {
		t.Fatal(e)
	}
	return src, f
}

// The process/profile reader is deliberately macOS-specific: it recognizes
// lsof/ps and bounded POSIX profile ancestors. On Windows the same synthetic
// macOS observation must fail closed before any OS listener/signature probe.
// Exercise that denial rather than manufacturing a Windows process identity.
func edgeEpochPositiveFixtureAvailable(t *testing.T) bool {
	t.Helper()
	if runtime.GOOS != "windows" {
		return true
	}
	probe := edgeFixture()
	source, err := readEdgeProcessPreflight(context.Background(), fixtureEdgeEndpoint, "/tmp/edge-fixture", probe)
	if err != errEdgeProcessUnqualified || source != (edgeProcessPreflight{}) ||
		probe.listenerCalls != 0 || probe.signatureCalls != 0 || probe.liveSignatureCalls != 0 {
		t.Fatalf("unsupported Windows Edge process evidence advanced or qualified: %v", err)
	}
	w, peer := seriesTestIdentity()
	guard, err := newEdgeProcessEpochRevalidation(source, w, peer, probe)
	if guard != nil || err == nil || err.Error() != externalTransportFenceError().Error() {
		t.Fatalf("unsupported process identity could create an external transport fence: %v", err)
	}
	return false
}

func TestEdgeEpochRevalidationAndSeriesBeforeAfterEveryOperation(t *testing.T) {
	if !edgeEpochPositiveFixtureAvailable(t) {
		return // Windows verified rejection; macOS exercises the full positive fixture.
	}
	src, f := expandedEdgeEpochProbe(t, 7) // admission + 3*(pre,post) = 7 OS observations
	w, p := seriesTestIdentity()
	verify, err := newEdgeProcessEpochRevalidation(src, w, p, f)
	if err != nil {
		t.Fatal(err)
	}
	var opened, closed int
	series, err := newExternalFenceSeries(w, p, func(context.Context, WorkerInfo, externalPeerIdentity) (externalFencedOperation, error) {
		opened++
		return &seriesTestOp{call: func(context.Context, WorkerInfo, externalPeerIdentity, string, map[string]any) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		}, close: func() error { closed++; return nil }}, nil
	}, verify)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		v, e := series.CallExternalFenced(context.Background(), w, p, "list_pages", nil)
		if e != nil || v["ok"] != true {
			t.Fatalf("operation %d rejected: %v", i, e)
		}
	}
	if opened != 3 || closed != 3 || f.listenerCalls != 14 || f.startCalls != 14 || f.signatureCalls != 7 || f.liveSignatureCalls != 7 || f.profileCalls != 14 {
		t.Fatalf("missing repeated OS proof: opened=%d closed=%d probes=%+v", opened, closed, f)
	}
	series.Close()
}

func TestEdgeEpochRevalidationRejectsChangedObservedIdentity(t *testing.T) {
	if !edgeEpochPositiveFixtureAvailable(t) {
		return // Windows verified rejection; macOS exercises the full positive fixture.
	}
	cases := []struct {
		name   string
		mutate func(*fakeEdgeProcessProbe)
	}{
		{"recycled pid start epoch", func(f *fakeEdgeProcessProbe) {
			f.startsRaw[2] = "501:Fri Oct 9 2027"
			f.startsRaw[3] = "501:Fri Oct 9 2027"
		}},
		{"endpoint now on other listener", func(f *fakeEdgeProcessProbe) { f.listenersRaw[2] = "p1246\nf7\nn127.0.0.1:9222\n" }},
		{"process image swapped", func(f *fakeEdgeProcessProbe) { f.imagesRaw[2] = "p1245\nftxt\nn/tmp/Edge\n" }},
		{"process command changed", func(f *fakeEdgeProcessProbe) { f.commandsRaw[2] = strings.ReplaceAll(f.commandsRaw[2], "9222", "9223") }},
		{"signature now invalid", func(f *fakeEdgeProcessProbe) { f.signatureErr = errors.New("sensitive signing output") }},
		{"profile replaced but new metadata internally stable", func(f *fakeEdgeProcessProbe) {
			f.onProfile = func(ctx context.Context, dir string, index int) (edgeProfileMetadata, error) {
				m, err := profileFixtureMetadata(ctx, dir)
				if index >= 2 {
					m.components[m.count-1].inode++
				}
				return m, err
			}
		}},
		{"profile data unavailable", func(f *fakeEdgeProcessProbe) {
			f.onProfile = func(context.Context, string, int) (edgeProfileMetadata, error) {
				return edgeProfileMetadata{}, errors.New("macOS privacy denied: secret path")
			}
		}},
		{"listener read failure", func(f *fakeEdgeProcessProbe) { f.err = errors.New("secret kernel error") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, f := expandedEdgeEpochProbe(t, 3)
			w, p := seriesTestIdentity()
			tc.mutate(f)
			guard, e := newEdgeProcessEpochRevalidation(src, w, p, f)
			if e != nil {
				t.Fatal(e)
			}
			if err := guard(context.Background(), p, w); err == nil || err.Error() != externalTransportFenceError().Error() {
				t.Fatalf("changed Edge process accepted or leaked error: %v", err)
			}
		})
	}
}

func TestEdgeEpochRevalidationConstructorRejectsAmbiguity(t *testing.T) {
	if !edgeEpochPositiveFixtureAvailable(t) {
		return // Windows verified rejection; macOS exercises the full positive fixture.
	}
	src, f := expandedEdgeEpochProbe(t, 1)
	w, p := seriesTestIdentity()
	cases := []struct {
		name   string
		source edgeProcessPreflight
		worker WorkerInfo
		peer   externalPeerIdentity
		probe  edgeProcessProbe
	}{
		{"missing process", edgeProcessPreflight{}, w, p, f},
		{"stale snapshot", func() edgeProcessPreflight { s := src; s.observedAt = time.Now().Add(-3 * time.Second); return s }(), w, p, f},
		{"future snapshot", func() edgeProcessPreflight { s := src; s.observedAt = time.Now().Add(time.Second); return s }(), w, p, f},
		{"no worker", src, WorkerInfo{}, p, f},
		{"no peer", src, w, externalPeerIdentity{}, f},
		{"no probe", src, w, p, nil},
		{"typed nil probe", src, w, p, (*fakeEdgeProcessProbe)(nil)},
		{"incorrect executable", func() edgeProcessPreflight { s := src; s.executable = "/tmp/Edge"; return s }(), w, p, f},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, e := newEdgeProcessEpochRevalidation(tc.source, tc.worker, tc.peer, tc.probe)
			if v != nil || e == nil {
				t.Fatal("unsafe process epoch allowed")
			}
		})
	}
}

func TestEdgeEpochRevalidationWrongWorkerOrOpaquePeerBlocksProbe(t *testing.T) {
	if !edgeEpochPositiveFixtureAvailable(t) {
		return // Windows verified rejection; macOS exercises the full positive fixture.
	}
	src, f := expandedEdgeEpochProbe(t, 2)
	w, p := seriesTestIdentity()
	guard, e := newEdgeProcessEpochRevalidation(src, w, p, f)
	if e != nil {
		t.Fatal(e)
	}
	otherWorker := w
	otherWorker.WorkerID = "different-owner"
	for _, tc := range []struct {
		peer   externalPeerIdentity
		worker WorkerInfo
		ctx    context.Context
	}{{pinnedTestPeer(), w, context.Background()}, {p, otherWorker, context.Background()}, {p, w, nil}} {
		if err := guard(tc.ctx, tc.peer, tc.worker); err == nil {
			t.Fatal("foreign owner admitted")
		}
	}
	if f.listenerCalls != 2 || f.signatureCalls != 1 {
		t.Fatal("foreign owner reached OS preflight")
	}
}

// Re-checking the OS process AFTER closing the owned transport must quarantine
// the action's already-produced data when Edge restarts during that action.
func TestEdgeEpochRevalidationSeriesQuarantinesRestartAfterAction(t *testing.T) {
	if !edgeEpochPositiveFixtureAvailable(t) {
		return // Windows verified rejection; macOS exercises the full positive fixture.
	}
	for _, name := range []string{"new-process-incarnation", "listener-changed", "profile-denied", "profile-replaced"} {
		t.Run(name, func(t *testing.T) {
			src, f := expandedEdgeEpochProbe(t, 3) // initial, pre-action, post-action
			switch name {
			case "new-process-incarnation":
				f.startsRaw[4] = "501:Fri Oct 9 2027"
				f.startsRaw[5] = "501:Fri Oct 9 2027"
			case "listener-changed":
				f.listenersRaw[4] = "p1246\nf7\nn127.0.0.1:9222\n"
			case "profile-replaced":
				f.onProfile = func(ctx context.Context, dir string, index int) (edgeProfileMetadata, error) {
					m, err := profileFixtureMetadata(ctx, dir)
					if index >= 4 {
						m.components[m.count-1].inode++
					}
					return m, err
				}
			case "profile-denied":
				f.onProfile = func(_ context.Context, _ string, index int) (edgeProfileMetadata, error) {
					if index >= 4 {
						return edgeProfileMetadata{}, errors.New("macOS refused profile metadata")
					}
					return profileFixtureMetadata(context.Background(), "/tmp/edge-fixture")
				}
			}
			w, p := seriesTestIdentity()
			guard, e := newEdgeProcessEpochRevalidation(src, w, p, f)
			if e != nil {
				t.Fatal(e)
			}
			opened, called, closed := 0, 0, 0
			factory := func(context.Context, WorkerInfo, externalPeerIdentity) (externalFencedOperation, error) {
				opened++
				return &seriesTestOp{
					call: func(context.Context, WorkerInfo, externalPeerIdentity, string, map[string]any) (map[string]any, error) {
						called++
						return map[string]any{"secret": "must-not-escape"}, nil
					},
					close: func() error { closed++; return nil }}, nil
			}
			series, e := newExternalFenceSeries(w, p, factory, guard)
			if e != nil {
				t.Fatal(e)
			}
			result, err := series.CallExternalFenced(context.Background(), w, p, "list_pages", nil)
			if result != nil || err == nil || err.Error() != externalTransportFenceError().Error() {
				t.Fatalf("changed process leaked data: %v %v", result, err)
			}
			if opened != 1 || called != 1 || closed != 1 {
				t.Fatal("owned connector was not cleaned before postprocess rejection")
			}
			if r, e := series.CallExternalFenced(context.Background(), w, p, "list_pages", nil); r != nil || e == nil {
				t.Fatal("closed identity guard permitted replay")
			}
			if opened != 1 {
				t.Fatal("second transport opened after process changed")
			}
		})
	}
}

func TestEdgeEpochRevalidationCanceledDuringOSProbe(t *testing.T) {
	if !edgeEpochPositiveFixtureAvailable(t) {
		return // Windows verified rejection; macOS exercises the full positive fixture.
	}
	src, f := expandedEdgeEpochProbe(t, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.onProfile = func(ctx context.Context, dir string, index int) (edgeProfileMetadata, error) {
		cancel()
		return edgeProfileMetadata{}, ctx.Err()
	}
	w, p := seriesTestIdentity()
	guard, e := newEdgeProcessEpochRevalidation(src, w, p, f)
	if e != nil {
		t.Fatal(e)
	}
	if e := guard(ctx, p, w); e == nil || e.Error() != externalTransportFenceError().Error() {
		t.Fatalf("cancelled OS probe accepted or leaked: %v", e)
	}
}
