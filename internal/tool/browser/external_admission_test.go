package browser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

// TEST ONLY: fabricate authority for offline lifecycle fixtures. This is never
// live Edge authentication, process/profile evidence or a production attestor.
func grantExternalRouteForTest(d RouteDecision) RouteDecision {
	d.grant = &externalRouteGrant{scope: d.Scope, route: d.Route, start: d.Start,
		peer:      externalPeerIdentity{source: &connectorEvidenceIdentity{}, incarnation: &connectorEvidenceIdentity{}},
		expiresAt: time.Now().UTC().Add(5 * time.Second)}
	return d
}

type testExternalPeerVerifier func(context.Context, externalPeerIdentity, WorkerInfo) (externalPeerIdentity, error)

func (f testExternalPeerVerifier) VerifyExternalPeer(ctx context.Context, peer externalPeerIdentity, w WorkerInfo) (externalPeerIdentity, error) {
	return f(ctx, peer, w)
}

// TEST ONLY: echoing assertions is forbidden for a real verifier.
var testAcceptExternalPeer = testExternalPeerVerifier(func(_ context.Context, peer externalPeerIdentity, _ WorkerInfo) (externalPeerIdentity, error) {
	return peer, nil
})

func newTestExternalLeaseManager(b ExternalLeaseBackend) *ExternalLeaseManager {
	m := NewExternalLeaseManager(b)
	m.verifier = testAcceptExternalPeer
	return m
}

func TestExternalAdmissionPureResolverAndSerializationCannotStart(t *testing.T) {
	d, err := ResolveRoute(requiredRouteFixture(t))
	if err != nil || d.grant != nil {
		t.Fatalf("pure resolver: %v", err)
	}
	qualified := grantExternalRouteForTest(d)
	encoded, err := json.Marshal(qualified)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "grant") || strings.Contains(string(encoded), "incarnation") || strings.Contains(string(encoded), "source") {
		t.Fatal("private proof serialized")
	}
	var decoded RouteDecision
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, route := range []RouteDecision{d, decoded, {Scope: d.Scope, Route: d.Route, Start: d.Start}} {
		b := &externalFakeBackend{}
		_, _, err := newTestExternalLeaseManager(b).Acquire(context.Background(), route, "")
		assertBrowserCode(t, err, ErrRequiredRouteUnavailable)
		if b.starts != 0 {
			t.Fatal("unqualified fixture started connector")
		}
	}
}

func TestExternalAdmissionBindsEntireDecision(t *testing.T) {
	mutations := map[string]func(*RouteDecision){
		"route":      func(d *RouteDecision) { d.Route = browserpolicy.RouteRequiredExternal },
		"workspace":  func(d *RouteDecision) { d.Scope.WorkspaceID += "other" },
		"root":       func(d *RouteDecision) { d.Scope.CanonicalWorkspaceRoot += "/other" },
		"task":       func(d *RouteDecision) { d.Scope.OwnerTaskID += "other" },
		"session":    func(d *RouteDecision) { d.Scope.OwnerSessionID += "other" },
		"acp":        func(d *RouteDecision) { d.Scope.OwnerACPSessionID += "other" },
		"provenance": func(d *RouteDecision) { d.Scope.Provenance = ScopeRuntime },
		"profile":    func(d *RouteDecision) { d.Start.ProfileID += "other" },
		"connector":  func(d *RouteDecision) { d.Start.ConnectorID += "other" },
		"endpoint":   func(d *RouteDecision) { d.Start.Endpoint += "/other" },
		"path":       func(d *RouteDecision) { d.Start.ProfilePath += "/other" },
		"browser":    func(d *RouteDecision) { d.Start.Browser = BrowserChrome },
		"ownership":  func(d *RouteDecision) { d.Start.Ownership.Process = OwnerAgentDockIsolated },
		"expired":    func(d *RouteDecision) { d.grant.expiresAt = time.Now().UTC().Add(-time.Second) },
		"cross grant": func(d *RouteDecision) {
			other := externalRoute(externalStart())
			other.grant.scope.OwnerTaskID += "other"
			d.grant = other.grant
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			d := externalRoute(externalStart())
			mutate(&d)
			b := &externalFakeBackend{}
			if _, _, err := newTestExternalLeaseManager(b).Acquire(context.Background(), d, ""); err == nil || b.starts != 0 {
				t.Fatal("changed admission started connector")
			}
		})
	}
}

func TestExternalAdmissionCopiesAreOneShotAcrossManagers(t *testing.T) {
	d := externalRoute(externalStart())
	b := &externalFakeBackend{}
	var wg sync.WaitGroup
	var successes atomic.Int32
	for range 16 {
		wg.Add(1)
		go func(copy RouteDecision) {
			defer wg.Done()
			m := newTestExternalLeaseManager(b)
			meta, _, err := m.Acquire(context.Background(), copy, "")
			if err == nil {
				successes.Add(1)
				if _, err := m.Release(context.Background(), copy.Scope, meta.BrowserLeaseID); err != nil {
					t.Error(err)
				}
			}
		}(d)
	}
	wg.Wait()
	if successes.Load() != 1 || b.starts != 1 {
		t.Fatal("grant replay admitted")
	}
	_, _, err := newTestExternalLeaseManager(b).Acquire(context.Background(), d, "")
	assertBrowserCode(t, err, ErrRequiredRouteUnavailable)
}

func TestExternalAdmissionVerifierFailuresPreserveUserPages(t *testing.T) {
	for _, mode := range []string{"missing", "error", "source", "incarnation", "panic", "cancel", "timeout", "revoked after baseline", "expired after baseline", "canceled after baseline"} {
		t.Run(mode, func(t *testing.T) {
			b := &externalFakeBackend{}
			m := NewExternalLeaseManager(b)
			d := externalRoute(externalStart())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			checks := 0
			if mode != "missing" {
				m.verifier = testExternalPeerVerifier(func(ctx context.Context, peer externalPeerIdentity, w WorkerInfo) (externalPeerIdentity, error) {
					checks++
					if w.WorkerID == "" || w.State != WorkerReady {
						t.Error("no immutable started worker")
					}
					switch mode {
					case "error":
						return peer, errors.New("secret-token ws://private PID=123")
					case "source":
						peer.source = &connectorEvidenceIdentity{}
					case "incarnation":
						peer.incarnation = &connectorEvidenceIdentity{}
					case "panic":
						panic("secret-token")
					case "cancel":
						cancel()
					case "timeout":
						<-ctx.Done()
						return peer, ctx.Err()
					case "revoked after baseline":
						if checks == 2 {
							return externalPeerIdentity{}, nil
						}
					case "expired after baseline":
						if checks == 2 {
							d.grant.expiresAt = time.Now().UTC().Add(-time.Second)
						}
					case "canceled after baseline":
						if checks == 2 {
							cancel()
						}
					}
					return peer, nil
				})
			}
			_, _, err := m.Acquire(ctx, d, "")
			assertBrowserCode(t, err, ErrRequiredRouteUnavailable)
			if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "PID=") {
				t.Fatal("verifier error leaked")
			}
			wantBaseline := 0
			if strings.Contains(mode, "after baseline") {
				wantBaseline = 1
			}
			if b.starts != 1 || externalCallCount(b, "list_pages") != wantBaseline || externalCallCount(b, "new_page") != 0 || externalCallCount(b, "close_page") != 0 || externalCallCount(b, "stop") != 1 || len(m.leases) != 0 {
				t.Fatalf("unsafe rejection: %+v", b.calls)
			}
		})
	}
}

func TestExternalAdmissionPlannerHandoffAndIndependentVerification(t *testing.T) {
	status := qualifyRuntimeStatusForTest(unqualifiedRuntimeStatus())
	planner, scope := plannerFixture(t, testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) { return status, nil }))
	d, err := planner.Resolve(context.Background(), scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.grant == nil || d.grant.scope != d.Scope || d.grant.start != d.Start || !d.grant.expiresAt.Equal(status.ExpiresAt) {
		t.Fatal("handoff not sealed")
	}
	b := &externalFakeBackend{}
	m := NewExternalLeaseManager(b)
	checks := 0
	var expectedWorker WorkerInfo
	m.verifier = testExternalPeerVerifier(func(_ context.Context, expected externalPeerIdentity, worker WorkerInfo) (externalPeerIdentity, error) {
		checks++
		// TEST ONLY independent source model: use the fake source's known live
		// identities, rather than returning the planner's supplied assertions.
		observed := externalPeerIdentity{source: status.source, incarnation: status.incarnation}
		if expected != observed {
			t.Error("grant changed source")
		}
		if checks == 1 {
			expectedWorker = worker
		} else if worker != expectedWorker {
			t.Error("worker changed between checks")
		}
		return observed, nil
	})
	meta, _, err := m.Acquire(context.Background(), d, "")
	if err != nil || checks != 3 {
		t.Fatalf("verified synthetic handoff: checks=%d err=%v", checks, err)
	}
	if _, err := m.Release(context.Background(), d.Scope, meta.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
	if externalCallCount(b, "close_page") != 1 || externalCallCount(b, "stop") != 1 {
		t.Fatal("owned lease not cleaned")
	}
}

func TestExternalAdmissionPreCanceledCannotStart(t *testing.T) {
	b := &externalFakeBackend{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := newTestExternalLeaseManager(b).Acquire(ctx, externalRoute(externalStart()), "")
	assertBrowserCode(t, err, ErrRequiredRouteUnavailable)
	if b.starts != 0 {
		t.Fatal("canceled admission started")
	}
}

func TestExternalAdmissionCompanyVerifierFailureNeverFallsBackManaged(t *testing.T) {
	status := qualifyRuntimeStatusForTest(unqualifiedRuntimeStatus())
	planner, scope := plannerFixture(t, testConnectorStatus(func(context.Context, string) (ConnectorRuntimeStatus, error) { return status, nil }))
	bridge := newTestACPBridge(t)
	bridge.planner = planner
	managed := &fakeLeaseBackend{}
	bridge.managed.manager = NewManagedLeaseManager(managed)
	external := &externalFakeBackend{}
	bridge.external = NewExternalLeaseManager(external) // Deliberately no verifier.
	token, err := bridge.RegisterSession("company", "codex", scope.CanonicalWorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := bridge.Acquire(context.Background(), token, "")
	assertBrowserCode(t, err, ErrRequiredRouteUnavailable)
	if meta != (LeaseMetadata{}) || managed.starts != 0 || external.starts != 1 || externalCallCount(external, "list_pages") != 0 || externalCallCount(external, "stop") != 1 {
		t.Fatal("failed company admission fell back or touched pages")
	}
}

func TestExternalAdmissionFailedVerificationStillConsumesGrant(t *testing.T) {
	d := externalRoute(externalStart())
	b := &externalFakeBackend{}
	m := NewExternalLeaseManager(b)
	b.stopErr = errors.New("stop failed")
	if _, _, err := m.Acquire(context.Background(), d, ""); err == nil {
		t.Fatal("missing verifier admitted")
	}
	if len(m.orphans) != 1 {
		t.Fatal("failed Stop not recoverable")
	}
	if _, _, err := newTestExternalLeaseManager(b).Acquire(context.Background(), d, ""); err == nil || b.starts != 1 {
		t.Fatal("failed admission replayed")
	}
	b.stopErr = nil
	if err := m.RecoverFailed(time.Now().UTC().Add(time.Hour)); err != nil || len(m.orphans) != 0 {
		t.Fatal("connector recovery failed", err)
	}
	if externalCallCount(b, "close_page") != 0 {
		t.Fatal("user page closed")
	}
}

func TestExternalAdmissionNoncooperativeVerifierRetainsSlot(t *testing.T) {
	b := &externalFakeBackend{}
	m := NewExternalLeaseManager(b)
	entered, unblock, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	m.verifier = testExternalPeerVerifier(func(context.Context, externalPeerIdentity, WorkerInfo) (externalPeerIdentity, error) {
		calls.Add(1)
		close(entered)
		<-unblock
		close(exited)
		return externalPeerIdentity{}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := m.Acquire(ctx, externalRoute(externalStart()), ""); done <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(unblock)
		t.Fatal("verifier not entered")
	}
	cancel()
	select {
	case err := <-done:
		assertBrowserCode(t, err, ErrRequiredRouteUnavailable)
	case <-time.After(3 * time.Second):
		close(unblock)
		t.Fatal("cancellation blocked on verifier")
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	_, _, err := m.Acquire(ctx2, externalRoute(externalStart()), "")
	assertBrowserCode(t, err, ErrRequiredRouteUnavailable)
	close(unblock)
	<-exited
	if calls.Load() != 1 || externalCallCount(b, "stop") != 2 || externalCallCount(b, "list_pages") != 0 {
		t.Fatal("stuck verifier spawned more work or touched pages")
	}
}
