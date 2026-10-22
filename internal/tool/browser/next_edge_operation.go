package browser

import (
	"context"
	"sync"
)

// A native permit has no black-box MCP child or outbound command queue. Its
// one synchronous Core call is joined before Close returns. The verified
// connection remains shared; an MCP permit must still dispose its transport.
type nativeEdgeOperation struct {
	mu     sync.Mutex
	worker *nativeEdgeWorker
	peer   externalPeerIdentity
	spent  bool
	closed bool
}

func (op *nativeEdgeOperation) Close() error {
	op.mu.Lock()
	defer op.mu.Unlock()
	op.closed = true
	return nil
}

func (op *nativeEdgeOperation) CallExternalFenced(ctx context.Context, info WorkerInfo, peer externalPeerIdentity, tool string, args map[string]any) (map[string]any, error) {
	op.mu.Lock()
	defer op.mu.Unlock()
	if ctx == nil || ctx.Err() != nil || op.closed || op.spent || peer != op.peer || info != op.worker.info {
		return nil, externalTransportFenceError()
	}
	op.spent = true
	w := op.worker
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped || !w.connection.root.live() || w.connection.peer != peer {
		return nil, externalTransportFenceError()
	}
	return w.call(ctx, tool, args)
}

func (w *nativeEdgeWorker) pages() map[string]any {
	pages := []any{}
	if w.target.targetID != "" && !w.target.closeConfirmed {
		pages = append(pages, map[string]any{"id": float64(1), "url": "about:blank", "title": "", "selected": false})
	}
	return map[string]any{"structuredContent": map[string]any{"pages": pages}}
}

func (w *nativeEdgeWorker) call(ctx context.Context, tool string, args map[string]any) (map[string]any, error) {
	d := w.target
	switch tool {
	case "list_pages":
		if d.targetID != "" && !d.closeConfirmed {
			ctx, unlock, err := d.lock(ctx)
			if err != nil {
				return nil, err
			}
			defer unlock()
			if err := d.verifyTargetLocked(ctx); err != nil {
				return nil, err
			}
		}
		return w.pages(), nil
	case "new_page":
		u, ok := args["url"].(string)
		background, _ := args["background"].(bool)
		if !ok || !background || d.opened || (u != "about:blank" && !d.originScope.Allows(u)) {
			return nil, externalTransportFenceError()
		}
		if err := d.OpenOwnedTarget(ctx, ""); err != nil {
			return nil, err
		}
		if err := d.enableOriginPolicy(ctx); err != nil {
			return nil, err
		}
		if u != "about:blank" {
			if err := d.Navigate(ctx, u); err != nil {
				return nil, err
			}
		}
		return w.pages(), nil
	}
	id, ok := args["pageId"].(float64)
	if !ok || id != 1 || d.targetID == "" {
		return nil, externalTransportFenceError()
	}
	switch tool {
	case "close_page":
		if closed, err := d.CloseOwnedTarget(ctx); err != nil || !closed {
			return nil, externalTransportFenceError()
		}
		return w.pages(), nil
	case "navigate_page":
		u, ok := args["url"].(string)
		if !ok {
			return nil, externalTransportFenceError()
		}
		if err := d.Navigate(ctx, u); err != nil {
			return nil, err
		}
		return directCDPTextResult("navigation completed"), nil
	case "take_snapshot":
		verbose, _ := args["verbose"].(bool)
		return d.Snapshot(ctx, verbose)
	case "take_screenshot":
		full, _ := args["fullPage"].(bool)
		return d.Screenshot(ctx, full)
	default:
		return nil, externalLeaseError(ErrActionInvalid, "", "company Edge connector permits read-only navigation, snapshot and screenshot", nil)
	}
}
