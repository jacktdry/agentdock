package browser

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

// Private, non-serializable, immutable except used. Copies of RouteDecision
// share the nonce. Package browser is the trust boundary, not a code sandbox.
type externalRouteGrant struct {
	scope     RequestScope
	route     browserpolicy.RouteKind
	start     ResolvedStart
	peer      externalPeerIdentity
	expiresAt time.Time
	used      atomic.Bool
	started   atomic.Bool // The native backend may start one worker for a consumed grant.
}

type externalPeerIdentity struct {
	source, incarnation *connectorEvidenceIdentity
}

// A future reviewed source must independently observe the live peer bound to
// this exact immutable started worker, and return its own source/incarnation.
// Planner assertions, WS reachability and worker readiness cannot attest this.
// No production implementation or wiring exists; offline fixtures are TEST ONLY.
type externalPeerVerifier interface {
	VerifyExternalPeer(context.Context, externalPeerIdentity, WorkerInfo) (externalPeerIdentity, error)
}

// externalTransportFence is a required admission capability, not an attestation.
// At EACH dispatch, the owning connector transport must atomically pin and
// compare the actual socket/session peer against the immutable expected peer.
// Reject missing, invalid or mismatched identity before any browser operation.
// Echoing expected identity or rechecking around legacy CallExternal is forbidden
// in production: neither provides transport-bound CDP fencing. WorkerRegistry
// does not implement this capability; no real bound transport exists yet.
type externalTransportFence interface {
	CallExternalFenced(context.Context, WorkerInfo, externalPeerIdentity, string, map[string]any) (map[string]any, error)
}

func externalTransportFenceError() error {
	return externalLeaseError(ErrRequiredRouteUnavailable, "", "external transport fence unavailable or mismatched", nil)
}

func (m *ExternalLeaseManager) callExternalFenced(ctx context.Context, worker WorkerInfo, peer externalPeerIdentity, tool string, args map[string]any) (map[string]any, error) {
	fence, ok := m.backend.(externalTransportFence)
	if !ok || peer.source == nil || peer.incarnation == nil || peer.source == peer.incarnation {
		return nil, externalTransportFenceError()
	}
	return fence.CallExternalFenced(ctx, worker, peer, tool, args)
}

func (g *externalRouteGrant) fresh() bool {
	return g != nil && time.Now().UTC().Before(g.expiresAt)
}

func consumeExternalRouteGrant(d RouteDecision) bool {
	g := d.grant
	return g.fresh() && g.scope == d.Scope && g.route == d.Route && g.start == d.Start &&
		g.peer.source != nil && g.peer.incarnation != nil && g.peer.source != g.peer.incarnation &&
		g.used.CompareAndSwap(false, true)
}

const externalPeerVerificationTimeout = 2 * time.Second

func (m *ExternalLeaseManager) verifyAdmissionPeer(ctx context.Context, g *externalRouteGrant, worker WorkerInfo) error {
	if !g.fresh() {
		return externalPeerVerificationError()
	}
	if err := m.verifyPeer(ctx, g.peer, worker); err != nil {
		return err
	}
	if !g.fresh() {
		return externalPeerVerificationError()
	}
	return nil
}

func externalPeerVerificationError() error {
	// Never expose provider errors/panics, proof identities, URLs or PIDs.
	return externalLeaseError(ErrRequiredRouteUnavailable, "", "external peer verification unavailable or mismatched", nil)
}

// Active leases verify the saved identity, without renewing or requiring the
// short-lived planning grant used only for admission.
func (m *ExternalLeaseManager) verifyPeer(ctx context.Context, expected externalPeerIdentity, worker WorkerInfo) error {
	reject := func() error {
		return externalPeerVerificationError()
	}
	m.mu.Lock()
	verifier := m.verifier
	m.mu.Unlock()
	if verifier == nil || ctx.Err() != nil || expected.source == nil || expected.incarnation == nil || expected.source == expected.incarnation {
		return reject()
	}
	verifyCtx, cancel := context.WithTimeout(ctx, externalPeerVerificationTimeout)
	defer cancel()
	// Hold the slot until the verifier actually exits, even after timeout.
	// A noncooperative verifier cannot create unbounded background work.
	select {
	case m.verificationInFlight <- struct{}{}:
	case <-verifyCtx.Done():
		return reject()
	}
	if verifyCtx.Err() != nil {
		<-m.verificationInFlight
		return reject()
	}
	type verificationResult struct {
		peer externalPeerIdentity
		ok   bool
	}
	results := make(chan verificationResult, 1)
	go func() {
		defer func() { <-m.verificationInFlight }()
		var result verificationResult
		defer func() {
			_ = recover()
			results <- result
		}()
		peer, err := verifier.VerifyExternalPeer(verifyCtx, expected, worker)
		result = verificationResult{peer: peer, ok: err == nil}
	}()
	select {
	case result := <-results:
		if !result.ok || result.peer != expected || verifyCtx.Err() != nil {
			return reject()
		}
		return nil
	case <-verifyCtx.Done():
		return reject()
	}
}
