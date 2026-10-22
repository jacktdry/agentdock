package browser

import (
	"context"
	"sync"
)

// externalFencedOperation is one completely disposed transport permit. Close
// MUST join/terminate that permit's owned connector and its upstream writes
// before returning. A timed-out or failed Close permanently invalidates the
// series; merely checking status between legacy calls cannot fence queued CDP
// frames. This is an internal source-only abstraction, not a production Edge
// attestor, Browser Backend, or connector discovery mechanism.
type externalFencedOperation interface {
	externalTransportFence
	Close() error
}

type externalFencedOperationFactory func(context.Context, WorkerInfo, externalPeerIdentity) (externalFencedOperation, error)

// externalFenceSeries models multi-step use of separate single-operation
// transport permits under one immutable, prequalified worker and peer.
// It cannot mint that qualification; the production Runtime still has no
// trusted ConnectorStatusProvider or connected multi-step lease backend.
// Source-only callers/tests inject a factory and a verifier; no real browser is
// contacted. Production must pass ExternalLeaseManager.verifyPeer (which has
// bounded, single-flight independent attestation), not a config or identity
// echo. A verifier callback alone cannot establish transport-bound authority.
type externalPeerRevalidation func(context.Context, externalPeerIdentity, WorkerInfo) error

type externalFenceSeries struct {
	mu       sync.Mutex
	worker   WorkerInfo
	peer     externalPeerIdentity
	factory  externalFencedOperationFactory
	reverify externalPeerRevalidation
	closed   bool
}

var _ externalTransportFence = (*externalFenceSeries)(nil)

func newExternalFenceSeries(worker WorkerInfo, peer externalPeerIdentity, factory externalFencedOperationFactory, reverify externalPeerRevalidation) (*externalFenceSeries, error) {
	if worker.WorkerID == "" || worker.State != WorkerReady || worker.Session.PID <= 0 ||
		peer.source == nil || peer.incarnation == nil || peer.source == peer.incarnation || factory == nil || reverify == nil {
		return nil, externalTransportFenceError()
	}
	return &externalFenceSeries{worker: worker, peer: peer, factory: factory, reverify: reverify}, nil
}

// checkPeer delegates only to an injected independent verifier. Production
// must pass the existing ExternalLeaseManager.verifyPeer bounded/single-flight
// method; the callback cannot be satisfied by echoing the expected pointer.
// No verifier error, panic detail or identity escapes to the caller.
func (s *externalFenceSeries) checkPeer(ctx context.Context) (err error) {
	if ctx == nil || ctx.Err() != nil || s.reverify == nil {
		return externalTransportFenceError()
	}
	defer func() {
		if recover() != nil || ctx.Err() != nil {
			err = externalTransportFenceError()
		}
	}()
	if s.reverify(ctx, s.peer, s.worker) != nil {
		return externalTransportFenceError()
	}
	return nil
}

// CallExternalFenced serializes all operations including factory setup and
// join-on-close. A failed or canceled operation irreversibly closes the series.
// No raw factory/tool/close errors escape, and no ambiguous response is returned.
// This caller does not grant page ownership or authorize a browser action;
// ExternalLeaseManager remains responsible for scope and per-page admission.
func (s *externalFenceSeries) CallExternalFenced(ctx context.Context, worker WorkerInfo, peer externalPeerIdentity, tool string, args map[string]any) (result map[string]any, err error) {
	if s == nil {
		return nil, externalTransportFenceError()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, externalTransportFenceError()
	}
	if ctx == nil || ctx.Err() != nil || worker != s.worker || peer != s.peer || tool == "" {
		s.closed = true
		return nil, externalTransportFenceError()
	}
	// Reverify the live peer before opening a new socket, and after the socket
	// has joined. This callback MUST perform independent observation. A simple
	// repeat of expected identity is intentionally insufficient in production.
	if s.checkPeer(ctx) != nil {
		s.closed = true
		return nil, externalTransportFenceError()
	}
	// Opener/operation panics must not revive a series or leak raw data. An
	// eventual production factory must independently attest each fresh socket.
	var op externalFencedOperation
	defer func() {
		if recover() != nil {
			s.closed = true
			result, err = nil, externalTransportFenceError()
		}
		if op != nil {
			closeFailed := true
			func() {
				defer func() { _ = recover() }()
				closeFailed = op.Close() != nil
			}()
			if closeFailed {
				s.closed = true
				result, err = nil, externalTransportFenceError()
			}
		}
		// Closing the one-shot socket may complete after caller cancellation.
		// Responses cannot escape merely because the preceding call succeeded.
		if err == nil && !s.closed && ctx.Err() == nil {
			// A fresh after-action observation must be made after connector close.
			// No ambiguous result can be returned if the Edge peer changed.
			revalidationErr := s.checkPeer(ctx)
			if revalidationErr != nil {
				s.closed = true
				result, err = nil, externalTransportFenceError()
			}
		}
		if ctx.Err() != nil {
			s.closed = true
			result, err = nil, externalTransportFenceError()
		}
	}()
	op, err = s.factory(ctx, worker, peer)
	if err != nil || op == nil || ctx.Err() != nil {
		s.closed = true
		return nil, externalTransportFenceError()
	}
	result, err = op.CallExternalFenced(ctx, worker, peer, tool, args)
	if err != nil || ctx.Err() != nil {
		s.closed = true
		return nil, externalTransportFenceError()
	}
	return result, nil
}

// Close prevents future operations. Only the per-operation connectors created
// by this series may be closed; the externally owned Edge process is not.
func (s *externalFenceSeries) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}
