package browser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// TEST ONLY: mutable, independently held synthetic observation. Never live trust.
type controlledExternalPeer struct {
	mu       sync.Mutex
	observed externalPeerIdentity
	err      error
	checks   int
}

func (v *controlledExternalPeer) VerifyExternalPeer(context.Context, externalPeerIdentity, WorkerInfo) (externalPeerIdentity, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.checks++
	return v.observed, v.err
}

func (v *controlledExternalPeer) count() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.checks
}

func acquiredControlledPeer(t *testing.T) (*ExternalLeaseManager, *externalFakeBackend, *controlledExternalPeer, LeaseMetadata, RouteDecision) {
	t.Helper()
	b := &externalFakeBackend{}
	m := NewExternalLeaseManager(b)
	d := externalRoute(externalStart())
	v := &controlledExternalPeer{observed: d.grant.peer}
	m.verifier = v
	meta, _, err := m.Acquire(context.Background(), d, "")
	if err != nil {
		t.Fatal(err)
	}
	return m, b, v, meta, d
}

func TestExternalLeasePeerRevalidationOutlivesAdmission(t *testing.T) {
	m, b, v, meta, d := acquiredControlledPeer(t)
	// Expired admission is deliberately irrelevant to an already acquired lease.
	d.grant.expiresAt = time.Now().UTC().Add(-time.Hour)
	for range 3 {
		if _, err := m.Call(context.Background(), d.Scope, meta.BrowserLeaseID, "click", nil); err != nil {
			t.Fatal(err)
		}
	}
	cleaned, err := m.Release(context.Background(), d.Scope, meta.BrowserLeaseID)
	if err != nil || cleaned.CleanupReason != "owned external page closed and connector stopped" || v.count() != 10 {
		t.Fatalf("missing repeated peer checks: checks=%d cleanup=%+v err=%v", v.count(), cleaned, err)
	}
	if externalCallCount(b, "list_pages") != 5 || externalCallCount(b, "close_page") != 1 || externalCallCount(b, "stop") != 1 {
		t.Fatal("valid peer lifecycle changed")
	}
	encoded, err := json.Marshal(cleaned)
	if err != nil || strings.Contains(string(encoded), "incarnation") || strings.Contains(string(encoded), "source") || strings.Contains(string(encoded), "peer") {
		t.Fatal("private lease evidence serialized", err)
	}
}

func rejectControlledPeer(m *ExternalLeaseManager, v *controlledExternalPeer, mode string) {
	v.mu.Lock()
	switch mode {
	case "source":
		v.observed.source = &connectorEvidenceIdentity{}
	case "incarnation":
		v.observed.incarnation = &connectorEvidenceIdentity{}
	case "revoked":
		v.observed = externalPeerIdentity{}
	case "unavailable":
		v.err = errors.New("peer-secret ws://private PID=123")
	}
	v.mu.Unlock()
	if mode == "missing verifier" {
		m.mu.Lock()
		m.verifier = nil
		m.mu.Unlock()
	}
}

func TestExternalLeasePeerLossLatchesBeforeAnyPageOperation(t *testing.T) {
	for _, mode := range []string{"source", "incarnation", "revoked", "unavailable", "missing verifier", "canceled", "cancel during verification", "panic", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			m, b, v, meta, d := acquiredControlledPeer(t)
			rejectControlledPeer(m, v, mode)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			if mode == "cancel during verification" || mode == "panic" || mode == "timeout" {
				m.mu.Lock()
				m.verifier = testExternalPeerVerifier(func(ctx context.Context, peer externalPeerIdentity, _ WorkerInfo) (externalPeerIdentity, error) {
					switch mode {
					case "panic":
						panic("peer-secret")
					case "timeout":
						<-ctx.Done()
					default:
						cancel()
					}
					return peer, nil
				})
				m.mu.Unlock()
			}
			_, err := m.Call(ctx, d.Scope, meta.BrowserLeaseID, "click", nil)
			assertBrowserCode(t, err, ErrRequiredRouteUnavailable)
			if strings.Contains(err.Error(), "peer-secret") || strings.Contains(err.Error(), "ws://") || strings.Contains(err.Error(), "PID=") {
				t.Fatal("peer provider error escaped")
			}
			v.mu.Lock()
			v.observed, v.err = d.grant.peer, nil
			v.mu.Unlock()
			m.mu.Lock()
			m.verifier = v
			m.mu.Unlock()
			checks := v.count()
			_, err = m.Call(context.Background(), d.Scope, meta.BrowserLeaseID, "click", nil)
			assertBrowserCode(t, err, ErrLeaseTargetMismatch)
			cleaned, err := m.Release(context.Background(), d.Scope, meta.BrowserLeaseID)
			if err != nil || cleaned.CleanupState != CleanupComplete || !strings.Contains(cleaned.CleanupReason, "page close unconfirmed") || v.count() != checks {
				t.Fatalf("lost peer resumed verification/cleanup: %+v %v", cleaned, err)
			}
			if strings.Contains(cleaned.CleanupError, "peer-secret") || externalCallCount(b, "list_pages") != 1 || externalCallCount(b, "click") != 0 || externalCallCount(b, "close_page") != 0 || externalCallCount(b, "stop") != 1 {
				t.Fatal("lost peer reached pages or leaked provider error")
			}
		})
	}
}

func TestExternalLeaseReleaseAndSweepRejectPeerBeforePages(t *testing.T) {
	for _, mode := range []string{"source", "incarnation", "revoked", "unavailable", "missing verifier", "canceled", "sweep", "stop failure"} {
		t.Run(mode, func(t *testing.T) {
			m, b, v, meta, d := acquiredControlledPeer(t)
			rejectMode := mode
			if mode == "sweep" || mode == "stop failure" {
				rejectMode = "revoked"
			}
			rejectControlledPeer(m, v, rejectMode)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			if mode == "stop failure" {
				b.mu.Lock()
				b.stopErr = errors.New("stop failed")
				b.mu.Unlock()
			}
			var cleaned LeaseMetadata
			var err error
			if mode == "sweep" {
				err = m.SweepExpired(meta.ExpiresAt.Add(time.Second))
				l, _ := m.lease(meta.BrowserLeaseID)
				l.mu.Lock()
				cleaned = l.metadata
				l.mu.Unlock()
			} else {
				cleaned, err = m.Release(ctx, d.Scope, meta.BrowserLeaseID)
			}
			if mode == "stop failure" {
				assertBrowserCode(t, err, ErrActionFailed)
				if cleaned.CleanupState != CleanupFailed {
					t.Fatal("Stop failure not recoverable")
				}
				b.mu.Lock()
				b.stopErr = nil
				b.mu.Unlock()
				v.mu.Lock()
				v.observed = d.grant.peer
				v.mu.Unlock()
				checks := v.count()
				if err := m.RecoverFailed(time.Now().UTC().Add(time.Hour)); err != nil || v.count() != checks {
					t.Fatal("recovery reverified peer", err)
				}
				cleaned, err = m.Release(context.Background(), d.Scope, meta.BrowserLeaseID)
			}
			if err != nil || cleaned.CleanupState != CleanupComplete || !strings.Contains(cleaned.CleanupReason, "page close unconfirmed") || strings.Contains(cleaned.CleanupError, "peer-secret") {
				t.Fatalf("inaccurate cleanup: %+v %v", cleaned, err)
			}
			wantStops := 1
			if mode == "stop failure" {
				wantStops = 2
			}
			if externalCallCount(b, "list_pages") != 1 || externalCallCount(b, "close_page") != 0 || externalCallCount(b, "stop") != wantStops {
				t.Fatal("cleanup/recovery touched pages under uncertain identity")
			}
		})
	}
}

func TestExternalLeasePeerOwnershipFirstAndConcurrentLoss(t *testing.T) {
	m, b, v, meta, d := acquiredControlledPeer(t)
	rejectControlledPeer(m, v, "revoked")
	wrongScope := d.Scope
	wrongScope.OwnerTaskID += "other"
	_, err := m.Call(context.Background(), wrongScope, meta.BrowserLeaseID, "click", nil)
	assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
	_, err = m.Release(context.Background(), wrongScope, meta.BrowserLeaseID)
	assertBrowserCode(t, err, ErrLeaseOwnerMismatch)
	if v.count() != 3 {
		t.Fatal("wrong owner reached verifier")
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.Call(context.Background(), d.Scope, meta.BrowserLeaseID, "click", nil)
			var e *Error
			if !errors.As(err, &e) || (e.Code != ErrRequiredRouteUnavailable && e.Code != ErrLeaseTargetMismatch) {
				t.Errorf("lost peer accepted concurrent call: %v", err)
			}
		}()
	}
	wg.Wait()
	if v.count() != 4 || externalCallCount(b, "list_pages") != 1 || externalCallCount(b, "click") != 0 {
		t.Fatal("concurrent copies bypassed latch")
	}
	if _, err := m.Release(context.Background(), d.Scope, meta.BrowserLeaseID); err != nil {
		t.Fatal(err)
	}
}
