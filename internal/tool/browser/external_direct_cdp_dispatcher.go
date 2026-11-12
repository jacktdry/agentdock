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

type directCDPProtocolError struct {
	code   int
	reason string
}

func (e *directCDPProtocolError) Error() string { return errDirectCDPAction.Error() }
func (e *directCDPProtocolError) Unwrap() error { return errDirectCDPAction }

func directCDPProtocolReason(message string) string {
	message = strings.ToLower(message)
	switch {
	case strings.Contains(message, "browser context"), strings.Contains(message, "browsercontext"):
		return "browser_context_rejected"
	case strings.Contains(message, "hidden"):
		return "hidden_target_rejected"
	case strings.Contains(message, "background"):
		return "background_target_rejected"
	case strings.Contains(message, "permission"), strings.Contains(message, "not allowed"):
		return "browser_permission_required"
	default:
		return "protocol_rejected"
	}
}

// Private Core dispatcher. Runtime qualification requires independent process,
// Profile and held-socket verification. Hidden/background are protocol requests;
// no-focus and real site behavior still require native Edge acceptance.
type directCDPDispatcher struct {
	relay *directCDPRelay
	down  *websocket.Conn
	// A channel mutex permits cancellation while queued. It covers whole target
	// lifecycle operations, including create/attach and their exact-ID cleanup.
	gate                                   chan struct{}
	retired                                atomic.Bool
	closeOnce                              sync.Once
	nextID                                 int64
	sharedSequence                         *int64 // Protected by the shared gate.
	keepTransportOnRelease                 bool
	targetID, sessionID                    string
	opened, closeAttempted, closeConfirmed bool
	ownedHiddenTarget                      bool
	closeErr                               error
	snapshotSeq                            uint64
	uidToBackend                           map[string]int64
	originScope                            browserpolicy.OriginScope
	bootstrap                              bool
	browserContextID                       string
	defaultBrowserContext                  bool
	policy                                 *directCDPPolicyState // Shared, protected by gate.
	policyEnabled                          bool
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
	d := &directCDPDispatcher{relay: relay, down: c, gate: make(chan struct{}, 1), uidToBackend: make(map[string]int64), originScope: originScope, bootstrap: true,
		policy: &directCDPPolicyState{targets: make(map[string]*directCDPDispatcher), pending: make(map[int64]string)}}
	d.gate <- struct{}{}
	return d, nil
}

func (d *directCDPDispatcher) Close() {
	d.retired.Store(true)
	d.closeOnce.Do(func() { _ = d.down.Close(); d.relay.Close() })
}

// forkTarget shares the single Core-owned, already-attached transport. Every
// target retains its own ID/session/UIDs/origin scope; whole operations share
// the connection gate and command sequence. Closing one owned target does not
// close sibling targets or ask the user to authorize another WebSocket.
// The root owns transport shutdown; any ambiguous I/O still retires it.
func (d *directCDPDispatcher) forkTarget(scope browserpolicy.OriginScope) (*directCDPDispatcher, error) {
	if scope == "" || d.browserContextID == "" || !d.live() {
		return nil, errDirectCDPRetired
	}
	for _, origin := range strings.Split(string(scope), "\n") {
		if !d.originScope.Allows(origin) {
			return nil, errDirectCDPAction
		}
	}
	sequence := d.sharedSequence
	if sequence == nil {
		sequence = &d.nextID
	}
	return &directCDPDispatcher{relay: d.relay, down: d.down, gate: d.gate, sharedSequence: sequence, keepTransportOnRelease: true,
		uidToBackend: make(map[string]int64), originScope: scope, bootstrap: true,
		browserContextID: d.browserContextID, defaultBrowserContext: d.defaultBrowserContext, policy: d.policy}, nil
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
	if d.retired.Load() {
		return errDirectCDPRetired
	}
	if !d.live() {
		d.Close()
		return errDirectCDPRetired
	}
	sequence := &d.nextID
	if d.sharedSequence != nil {
		sequence = d.sharedSequence
	}
	if *sequence == math.MaxInt64 {
		d.Close()
		return errDirectCDPRetired
	}
	*sequence++
	commandID := *sequence
	data, err := json.Marshal(directCDPCommand{commandID, method, session, params})
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
	var ownResult json.RawMessage
	var ownError *directCDPProtocolError
	ownSeen := false
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
			Params    json.RawMessage `json:"params"`
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
			if err := d.handlePolicyEventLocked(ctx, reply.Method, reply.SessionID, reply.Params); err != nil {
				d.Close()
				return errDirectCDPRetired
			}
			continue // Events can never supply an operation result.
		}
		if reply.Method != "" || (reply.Result == nil) == (reply.Error == nil) {
			d.Close()
			return errDirectCDPRetired
		}
		if *reply.ID == commandID {
			if ownSeen || reply.SessionID != session {
				d.Close()
				return errDirectCDPRetired
			}
			ownSeen = true
			ownResult = reply.Result
			if reply.Error != nil {
				ownError = &directCDPProtocolError{code: reply.Error.Code, reason: directCDPProtocolReason(reply.Error.Message)}
			}
		} else if d.policy != nil {
			expected, exists := d.policy.pending[*reply.ID]
			if !exists || reply.SessionID != expected || reply.Error != nil || len(reply.Result) == 0 || string(reply.Result) == "null" {
				d.Close()
				return errDirectCDPRetired
			}
			delete(d.policy.pending, *reply.ID)
		} else {
			d.Close()
			return errDirectCDPRetired
		}
		if !ownSeen || (d.policy != nil && len(d.policy.pending) != 0) {
			continue
		}
		// Stop/wait-free cancellation fencing: a callback already running, or a
		// canceled context, means completion cannot be trusted for later reuse.
		if !stop() || ctx.Err() != nil || !d.live() {
			d.Close()
			return errDirectCDPRetired
		}
		if ownError != nil {
			return ownError
		}
		if string(ownResult) == "null" || json.Unmarshal(ownResult, result) != nil {
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
	contextID := d.browserContextID
	if d.defaultBrowserContext {
		contextID = ""
	}
	// Chromium rejects the presence of newWindow for a hidden target, even
	// when its boolean value is false. Omit it; hidden+background creates no
	// native window. Do not retry by creating a visible target on failure.
	err = d.callLocked(ctx, "Target.createTarget", "", struct {
		URL              string `json:"url"`
		Background       bool   `json:"background"`
		Hidden           bool   `json:"hidden"`
		ForTab           bool   `json:"forTab"`
		BrowserContextID string `json:"browserContextId,omitempty"`
	}{u, true, true, false, contextID}, &created)
	if err != nil {
		d.Close()
		return err
	}
	if created.TargetID == "" {
		d.Close()
		return errDirectCDPRetired
	}
	d.targetID = created.TargetID
	d.ownedHiddenTarget = true
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
	if err != nil || info.TargetID != d.targetID || !d.ownedTargetTypeMatches(info.Type) ||
		!d.targetContextMatchesLocked(ctx, info) {
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
	if d.closeConfirmed {
		d.retirePolicySessionLocked()
	}
	return d.closeErr
}

// The bool is exact-target close confirmation, never a browser health signal.
func (d *directCDPDispatcher) CloseOwnedTarget(ctx context.Context) (bool, error) {
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		if !d.keepTransportOnRelease {
			d.Close()
		}
		return false, err
	}
	defer unlock()
	defer func() {
		if d.keepTransportOnRelease {
			d.retired.Store(true)
		} else {
			d.Close()
		}
	}()
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
		LoaderID   string `json:"loaderId"`
	}
	if err = d.callLocked(ctx, "Page.navigate", d.sessionID, struct {
		URL string `json:"url"`
	}{u}, &result); err != nil {
		return err
	}
	if result.ErrorText != "" || result.IsDownload {
		return errDirectCDPAction
	}
	// Page.navigate can answer before the new document commits. Drain scoped
	// Fetch events while observing our exact target, so the following snapshot
	// does not mistake its transient empty/about:blank URL for identity loss.
	for {
		info, err := d.targetInfoLocked(ctx)
		if err != nil || info.TargetID != d.targetID || !d.ownedTargetTypeMatches(info.Type) || !d.targetContextMatchesLocked(ctx, info) {
			d.Close()
			return errDirectCDPRetired
		}
		ready := d.originScope.Allows(info.URL)
		if !ready && info.URL != "" && info.URL != "about:blank" {
			d.Close()
			return errDirectCDPRetired
		}
		if ready && result.LoaderID != "" {
			var tree struct {
				FrameTree struct {
					Frame struct {
						LoaderID string `json:"loaderId"`
						URL      string `json:"url"`
					} `json:"frame"`
				} `json:"frameTree"`
			}
			if err := d.callLocked(ctx, "Page.getFrameTree", d.sessionID, struct{}{}, &tree); err != nil {
				return err
			}
			ready = tree.FrameTree.Frame.LoaderID == result.LoaderID && d.originScope.Allows(tree.FrameTree.Frame.URL)
		}
		if ready {
			d.bootstrap = false
			return nil
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			d.Close()
			return errDirectCDPRetired
		case <-timer.C:
		}
	}
}
