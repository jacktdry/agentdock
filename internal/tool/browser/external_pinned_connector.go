package browser

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
)

var errPinnedConnectorUnavailable = errors.New("external pinned connector unavailable")

// PRIVATE source-only binding, without production callers or an
// externalTransportFence implementation. Its bounded frame window cannot
// semantically attribute background CDP traffic to an MCP operation.
type pinnedExternalConnector struct {
	registry *WorkerRegistry
	relay    *pinnedCDPRelay
	peer     externalPeerIdentity
	worker   WorkerInfo // Immutable generation, never returned as browser status.
	instance *managedWorker
	stopOnce sync.Once
	stopErr  error
	callMu   sync.Mutex
}

func newPinnedExternalConnector(ctx context.Context, relay *pinnedCDPRelay, expected externalPeerIdentity, options ExternalWorkerOptions, registry *WorkerRegistry) (*pinnedExternalConnector, error) {
	if ctx == nil || ctx.Err() != nil || relay == nil || registry == nil ||
		expected.source == nil || expected.incarnation == nil || expected.source == expected.incarnation ||
		validateExternalStart(options.Route) != nil {
		return nil, errPinnedConnectorUnavailable
	}
	registry.mu.Lock()
	available := !registry.closed && registry.workers != nil && registry.deps.Probe != nil && registry.deps.Open != nil
	registry.mu.Unlock()
	if !available {
		return nil, errPinnedConnectorUnavailable
	}
	relay.mu.Lock()
	token := strings.TrimPrefix(relay.path, "/devtools/browser/")
	decoded, tokenErr := hex.DecodeString(token)
	valid := !relay.lost && !relay.claimed && !relay.bound && relay.peer == expected &&
		relay.sourceEndpoint == options.Route.Start.Endpoint && pinnedCDPEndpointValid(relay.endpoint) &&
		relay.listener != nil && relay.up != nil && relay.server != nil && relay.cancel != nil && relay.done != nil &&
		tokenErr == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == token &&
		relay.endpoint == "ws://"+relay.listener.Addr().String()+relay.path && ctx.Err() == nil
	if !valid {
		relay.mu.Unlock()
		return nil, errPinnedConnectorUnavailable
	}
	relay.bound = true // Reservation spans start; never released for replay.
	relay.gate, relay.gatePeer = pinnedCDPBootstrap, expected
	relay.bootstrapCtx = ctx
	endpoint := relay.endpoint
	relay.mu.Unlock()
	stopBootstrap := context.AfterFunc(ctx, relay.Close)
	defer stopBootstrap()

	cfg := externalWorkerConfig(options)
	for i, arg := range cfg.Args {
		if strings.HasPrefix(arg, "--wsEndpoint=") {
			cfg.Args[i] = "--wsEndpoint=" + endpoint
		}
	}
	c := &pinnedExternalConnector{registry: registry, relay: relay, peer: expected}
	err := relay.withPinnedPeer(expected, func() error {
		if ctx.Err() != nil {
			return errPinnedConnectorUnavailable
		}
		var err error
		c.worker, err = registry.startWorkerWithCleanup(ctx, cfg, relay.Close, func() error {
			return relay.withPinnedPeer(expected, func() error {
				if ctx.Err() != nil {
					return errPinnedConnectorUnavailable
				}
				return nil
			})
		})
		if err != nil {
			return errPinnedConnectorUnavailable
		}
		c.instance, err = registry.worker(c.worker.WorkerID)
		if err != nil || !c.workerAlive() || ctx.Err() != nil {
			return errPinnedConnectorUnavailable
		}
		// Readiness alone does not establish that the one-shot connector joined.
		relay.mu.Lock()
		attached := !relay.lost && relay.peer == expected && relay.claimed && relay.down != nil &&
			relay.gate == pinnedCDPBootstrap && ctx.Err() == nil
		if attached {
			relay.gate, relay.gateWorker = pinnedCDPClosed, c.worker
			relay.bootstrapCtx = nil
		}
		relay.mu.Unlock()
		if !attached {
			return errPinnedConnectorUnavailable
		}
		return nil
	})
	if err != nil {
		_ = c.Stop()
		return nil, errPinnedConnectorUnavailable
	}
	// One bounded-lifetime watcher stops only the owned connector after loss,
	// including unsolicited idle outbound traffic. Stop never holds relay.mu
	// while waiting for worker.mu.
	go func() { <-relay.lifetime.Done(); _ = c.Stop() }()
	return c, nil
}

// Tests/future reviewed backend ONLY. No WorkerRegistry.CallExternal fallback.
// Reject overlap rather than queue another operation behind a live permit.
func (c *pinnedExternalConnector) callBound(ctx context.Context, expected WorkerInfo, peer externalPeerIdentity, tool string, args map[string]any) (result map[string]any, err error) {
	if c == nil || !c.callMu.TryLock() {
		return nil, errPinnedConnectorUnavailable
	}
	defer c.callMu.Unlock()
	defer func() {
		if err != nil {
			result = nil
			_ = c.Stop()
		}
	}()
	if ctx == nil || ctx.Err() != nil || expected != c.worker || peer != c.peer {
		return nil, errPinnedConnectorUnavailable
	}
	switch tool {
	case "list_pages", "new_page", "close_page", "navigate_page", "take_snapshot", "take_screenshot", "evaluate_script", "click", "fill", "press_key":
	default:
		return nil, errPinnedConnectorUnavailable
	}
	w, lookupErr := c.registry.worker(expected.WorkerID)
	if lookupErr != nil || w != c.instance {
		return nil, errPinnedConnectorUnavailable
	}
	w.mu.Lock()
	defer w.mu.Unlock() // Runs before the outer failure cleanup acquires worker.mu.
	if w.info != expected || expected.State != WorkerReady || workerSessionNil(w.session) || w.session.Info() != expected.Session {
		return nil, errPinnedConnectorUnavailable
	}
	session := w.session // Direct immutable session, held under worker.mu.
	opCtx, cancel := context.WithTimeout(ctx, pinnedCDPOperationLimit)
	stopLoss := context.AfterFunc(c.relay.lifetime, cancel)
	stopExpiry := context.AfterFunc(opCtx, c.relay.Close)
	p := &pinnedCDPPermit{ctx: opCtx, peer: peer, worker: expected}
	opened := false
	defer func() {
		if recover() != nil {
			err = errPinnedConnectorUnavailable
		}
		if opened && !c.relay.closePermit(p) || opCtx.Err() != nil {
			err = errPinnedConnectorUnavailable
		}
		stopExpiry()
		stopLoss()
		cancel()
	}()
	if c.relay.openPermit(p) != nil {
		return nil, errPinnedConnectorUnavailable
	}
	opened = true
	result, err = session.Call(opCtx, tool, args)
	// Close before post-call identity/result inspection. Even a slow Info must
	// not extend the outbound window after Session.Call has returned.
	permitOK := c.relay.closePermit(p)
	opened = false
	structured, _ := result["structuredContent"].(map[string]any)
	if !permitOK || err != nil || w.info != expected || session.Info() != expected.Session || externalReconnected(structured) {
		return nil, errPinnedConnectorUnavailable
	}
	return result, nil
}

func (c *pinnedExternalConnector) workerAlive() bool {
	w, err := c.registry.worker(c.worker.WorkerID)
	if err != nil || w != c.instance {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return c.worker.State == WorkerReady && w.info == c.worker && !workerSessionNil(w.session) && w.session.Info() == c.worker.Session
}

// No dispatch. Remote loss may remain unobserved until relay I/O; Close may
// race this observation. Any observed loss permanently closes the binding.
func (c *pinnedExternalConnector) checkAlive(ctx context.Context) error {
	if c == nil {
		return errPinnedConnectorUnavailable
	}
	if ctx == nil || ctx.Err() != nil || c.relay.withPinnedPeer(c.peer, func() error {
		if !c.workerAlive() {
			return errPinnedConnectorUnavailable
		}
		return nil
	}) != nil {
		_ = c.Stop()
		return errPinnedConnectorUnavailable
	}
	return nil
}

func (c *pinnedExternalConnector) Stop() error {
	if c == nil {
		return nil
	}
	c.stopOnce.Do(func() {
		// Close transport first, even if connector cleanup blocks or fails.
		c.relay.Close()
		if c.worker.WorkerID != "" && c.registry.Stop(c.worker.WorkerID) != nil {
			c.stopErr = errPinnedConnectorUnavailable
		}
	})
	return c.stopErr
}
