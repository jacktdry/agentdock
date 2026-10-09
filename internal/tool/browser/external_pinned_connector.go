package browser

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
)

var errPinnedConnectorUnavailable = errors.New("external pinned connector unavailable")

// PRIVATE source-only binding. No browser operation API, admission grant,
// production caller or externalTransportFence implementation. Peer checks are
// local pre/post observations, NOT atomic per-command transport fencing.
type pinnedExternalConnector struct {
	registry *WorkerRegistry
	relay    *pinnedCDPRelay
	peer     externalPeerIdentity
	worker   WorkerInfo // Immutable generation, never returned as browser status.
	instance *managedWorker
	stopOnce sync.Once
	stopErr  error
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
	endpoint := relay.endpoint
	relay.mu.Unlock()

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
		attached := relay.claimed && relay.down != nil
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
	return c, nil
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
