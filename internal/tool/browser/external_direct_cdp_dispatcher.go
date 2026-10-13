package browser

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uvwt/agentdock/internal/browserpolicy"
)

const directCDPCallTimeout = 10 * time.Second

var (
	errDirectCDPRetired = errors.New("direct CDP dispatcher retired; target cleanup unconfirmed")
	errDirectCDPAction  = errors.New("direct CDP operation failed")
)

// PRIVATE Core foundation: proves transport and command/target provenance only,
// NOT login, consent, or no-focus. Edge remains UNQUALIFIED. Hidden=true +
// Background=true are protocol requests; live Edge no-focus remains UAT. No production caller.
type directCDPDispatcher struct {
	relay *directCDPRelay
	down  *websocket.Conn
	// A channel mutex permits cancellation while queued. It covers whole target
	// lifecycle operations, including create/attach and their exact-ID cleanup.
	gate                                   chan struct{}
	retired                                atomic.Bool
	closeOnce                              sync.Once
	nextID                                 int64
	targetID, sessionID                    string
	opened, closeAttempted, closeConfirmed bool
	closeErr                               error
	snapshotSeq                            uint64
	uidToBackend                           map[string]int64
	originScope                            browserpolicy.OriginScope
	bootstrap                              bool
}

func newDirectCDPDispatcher(ctx context.Context, relay *directCDPRelay, originScope browserpolicy.OriginScope) (*directCDPDispatcher, error) {
	if relay == nil || originScope == "" {
		return nil, errDirectCDPRetired
	}
	if ctx == nil || ctx.Err() != nil || relay.Lost() {
		return nil, errDirectCDPRetired
	}
	dialCtx, cancel := context.WithTimeout(ctx, directCDPAttachTimeout)
	defer cancel()
	c, resp, err := (&websocket.Dialer{Proxy: nil, HandshakeTimeout: directCDPAttachTimeout}).DialContext(dialCtx, relay.Endpoint(), nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil || dialCtx.Err() != nil {
		if c != nil {
			_ = c.Close()
		}
		return nil, errDirectCDPRetired
	}
	c.SetReadLimit(pinnedCDPLimit)
	d := &directCDPDispatcher{relay: relay, down: c, gate: make(chan struct{}, 1), uidToBackend: make(map[string]int64), originScope: originScope, bootstrap: true}
	d.gate <- struct{}{}
	return d, nil
}

func (d *directCDPDispatcher) Close() {
	d.retired.Store(true)
	d.closeOnce.Do(func() { _ = d.down.Close(); d.relay.Close() })
}

func (d *directCDPDispatcher) live() bool { return !d.retired.Load() && !d.relay.Lost() }

func (d *directCDPDispatcher) lock(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, errDirectCDPAction
	}
	ctx, cancel := context.WithTimeout(ctx, directCDPCallTimeout)
	if err := ctx.Err(); err != nil {
		cancel()
		return nil, nil, err
	}
	select {
	case <-ctx.Done():
		cancel()
		return nil, nil, ctx.Err()
	case <-d.gate:
	}
	if err := ctx.Err(); err != nil {
		d.gate <- struct{}{}
		cancel()
		return nil, nil, err
	}
	return ctx, func() { d.gate <- struct{}{}; cancel() }, nil
}

type directCDPCommand struct {
	ID        int64  `json:"id"`
	Method    string `json:"method"`
	SessionID string `json:"sessionId,omitempty"`
	Params    any    `json:"params"`
}

// callLocked never reconnects or retries. Error payloads are deliberately not
// exposed: protocol/action failures do not confer browser authority or health.
func (d *directCDPDispatcher) callLocked(ctx context.Context, method, session string, params, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !d.live() {
		d.Close()
		return errDirectCDPRetired
	}
	if d.nextID == math.MaxInt64 {
		d.Close()
		return errDirectCDPRetired
	}
	d.nextID++
	data, err := json.Marshal(directCDPCommand{d.nextID, method, session, params})
	if err != nil {
		return errDirectCDPAction
	}
	deadline, _ := ctx.Deadline()
	if d.down.SetWriteDeadline(deadline) != nil || d.down.SetReadDeadline(deadline) != nil {
		d.Close()
		return errDirectCDPRetired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// From this boundary a command may have been sent. Cancellation permanently
	// retires both sockets, interrupting a blocked read without another reader.
	stop := context.AfterFunc(ctx, d.Close)
	defer stop()
	if err = d.down.WriteMessage(websocket.TextMessage, data); err != nil {
		d.Close()
		return errDirectCDPRetired
	}
	for {
		kind, data, err := d.down.ReadMessage()
		if err != nil || kind != websocket.TextMessage {
			d.Close()
			return errDirectCDPRetired
		}
		var reply struct {
			ID        *int64          `json:"id"`
			Method    string          `json:"method"`
			SessionID string          `json:"sessionId"`
			Result    json.RawMessage `json:"result"`
			Error     *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &reply) != nil {
			d.Close()
			return errDirectCDPRetired
		}
		if reply.ID == nil || *reply.ID == 0 {
			if reply.Method == "" || reply.Result != nil || reply.Error != nil {
				d.Close()
				return errDirectCDPRetired
			}
			continue // Events can never supply an operation result.
		}
		if *reply.ID != d.nextID || reply.SessionID != session || reply.Method != "" ||
			(reply.Result == nil) == (reply.Error == nil) {
			d.Close()
			return errDirectCDPRetired
		}
		// Stop/wait-free cancellation fencing: a callback already running, or a
		// canceled context, means completion cannot be trusted for later reuse.
		if !stop() || ctx.Err() != nil || !d.live() {
			d.Close()
			return errDirectCDPRetired
		}
		if reply.Error != nil {
			return errDirectCDPAction
		}
		if string(reply.Result) == "null" || json.Unmarshal(reply.Result, result) != nil {
			d.Close()
			return errDirectCDPRetired
		}
		return nil
	}
}

func directCDPURL(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "about:blank", nil
	}
	u, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(raw) != raw || u.User != nil {
		return "", errDirectCDPAction
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return "", errDirectCDPAction
		}
	case "about":
		if raw != "about:blank" {
			return "", errDirectCDPAction
		}
	default:
		return "", errDirectCDPAction
	}
	return raw, nil
}

func (d *directCDPDispatcher) OpenOwnedTarget(ctx context.Context, initialURL string) error {
	u, err := directCDPURL(initialURL)
	if err != nil {
		return err
	}
	if u != "about:blank" {
		return errDirectCDPAction
	}
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if !d.live() {
		return errDirectCDPRetired
	}
	if d.opened {
		return errDirectCDPAction
	}
	d.opened = true
	var created struct {
		TargetID string `json:"targetId"`
	}
	err = d.callLocked(ctx, "Target.createTarget", "", struct {
		URL        string `json:"url"`
		Background bool   `json:"background"`
		NewWindow  bool   `json:"newWindow"`
		Hidden     bool   `json:"hidden"`
		ForTab     bool   `json:"forTab"`
	}{u, true, false, true, false}, &created)
	if err != nil {
		d.Close()
		return err
	}
	if created.TargetID == "" {
		d.Close()
		return errDirectCDPRetired
	}
	d.targetID = created.TargetID
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	err = d.callLocked(ctx, "Target.attachToTarget", "", struct {
		TargetID string `json:"targetId"`
		Flatten  bool   `json:"flatten"`
	}{d.targetID, true}, &attached)
	if err == nil && attached.SessionID == "" {
		err = errDirectCDPAction
	}
	if err != nil {
		if d.live() {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), directCDPCallTimeout)
			_ = d.closeTargetLocked(cleanupCtx)
			cancel()
		}
		d.Close()
		return err
	}
	d.sessionID = attached.SessionID
	info, err := d.targetInfoLocked(ctx)
	if err != nil || info.TargetID != d.targetID || info.Type != "page" {
		if d.live() {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), directCDPCallTimeout)
			_ = d.closeTargetLocked(cleanupCtx)
			cancel()
		}
		d.Close()
		return errDirectCDPRetired
	}
	return nil
}

func (d *directCDPDispatcher) closeTargetLocked(ctx context.Context) error {
	if d.closeAttempted {
		return d.closeErr
	}
	d.closeAttempted = true
	var result struct {
		Success bool `json:"success"`
	}
	d.closeErr = d.callLocked(ctx, "Target.closeTarget", "", struct {
		TargetID string `json:"targetId"`
	}{d.targetID}, &result)
	if d.closeErr == nil && !result.Success {
		d.closeErr = errDirectCDPAction
	}
	d.closeConfirmed = d.closeErr == nil
	return d.closeErr
}

// The bool is exact-target close confirmation, never a browser health signal.
func (d *directCDPDispatcher) CloseOwnedTarget(ctx context.Context) (bool, error) {
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		d.Close()
		return false, err
	}
	defer unlock()
	defer d.Close()
	if d.closeAttempted {
		return d.closeConfirmed, d.closeErr
	}
	if d.targetID == "" || !d.live() {
		return false, errDirectCDPRetired
	}
	d.clearUIDsLocked()
	err = d.closeTargetLocked(ctx)
	return d.closeConfirmed, err
}

func (d *directCDPDispatcher) evaluateLocked(ctx context.Context, expression string) (json.RawMessage, error) {
	if d.sessionID == "" {
		return nil, errDirectCDPAction
	}
	var result struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	err := d.callLocked(ctx, "Runtime.evaluate", d.sessionID, struct {
		Expression    string `json:"expression"`
		ReturnByValue bool   `json:"returnByValue"`
		AwaitPromise  bool   `json:"awaitPromise"`
		UserGesture   bool   `json:"userGesture"`
	}{expression, true, true, false}, &result)
	if err != nil {
		return nil, err
	}
	if len(result.ExceptionDetails) != 0 && string(result.ExceptionDetails) != "null" {
		return nil, errDirectCDPAction
	}
	return result.Result.Value, nil
}

func (d *directCDPDispatcher) Evaluate(ctx context.Context, expression string) (json.RawMessage, error) {
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if !d.live() {
		return nil, errDirectCDPRetired
	}
	if err := d.verifyTargetLocked(ctx); err != nil {
		return nil, err
	}
	return d.evaluateLocked(ctx, expression)
}

func (d *directCDPDispatcher) Navigate(ctx context.Context, rawURL string) error {
	u, err := directCDPURL(rawURL)
	if err != nil {
		return err
	}
	if !d.originScope.Allows(u) {
		return errDirectCDPAction
	}
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if !d.live() {
		return errDirectCDPRetired
	}
	if d.sessionID == "" {
		return errDirectCDPAction
	}
	if err := d.verifyTargetLocked(ctx); err != nil {
		return err
	}
	d.clearUIDsLocked()
	var result struct {
		ErrorText  string `json:"errorText"`
		IsDownload bool   `json:"isDownload"`
	}
	if err = d.callLocked(ctx, "Page.navigate", d.sessionID, struct {
		URL string `json:"url"`
	}{u}, &result); err != nil {
		return err
	}
	if result.ErrorText != "" || result.IsDownload {
		return errDirectCDPAction
	}
	d.bootstrap = false
	return nil
}
