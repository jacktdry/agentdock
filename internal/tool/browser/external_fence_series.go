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
// Source-only callers/tests inject a factory; no real browser is contacted.
type externalFenceSeries struct {
	mu      sync.Mutex
	worker  WorkerInfo
	peer    externalPeerIdentity
	factory externalFencedOperationFactory
	closed  bool
}

var _ externalTransportFence = (*externalFenceSeries)(nil)

func newExternalFenceSeries(worker WorkerInfo, peer externalPeerIdentity, factory externalFencedOperationFactory) (*externalFenceSeries, error) {
	if worker.WorkerID == "" || worker.State != WorkerReady || worker.Session.PID <= 0 ||
		peer.source == nil || peer.incarnation == nil || peer.source == peer.incarnation || factory == nil {
		return nil, externalTransportFenceError()
	}
	return &externalFenceSeries{worker: worker, peer: peer, factory: factory}, nil
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
