package browser

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/gorilla/websocket"
)

// These maps belong to the one connection gate, not the selected page. Events
// from sibling owned sessions are checked against THAT session's immutable
// scope. No browser-wide discovery, request body, credential or auth UI is used.
type directCDPPolicyState struct {
	targets      map[string]*directCDPDispatcher
	pending      map[int64]string
	retired      map[string]bool
	retiredOrder []string
}

func (d *directCDPDispatcher) enableOriginPolicy(ctx context.Context) error {
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if d.policy == nil || d.policyEnabled || d.sessionID == "" || len(d.policy.targets) >= 1024 {
		return errDirectCDPAction
	}
	if existing := d.policy.targets[d.sessionID]; existing != nil {
		d.Close()
		return errDirectCDPRetired
	}
	if err := d.verifyTargetLocked(ctx); err != nil {
		return err
	}
	d.policy.targets[d.sessionID] = d
	d.policyEnabled = true
	var result struct{}
	err = d.callLocked(ctx, "Fetch.enable", d.sessionID, map[string]any{
		"patterns": []any{
			map[string]any{"urlPattern": "*", "resourceType": "Document", "requestStage": "Request"},
			map[string]any{"urlPattern": "*", "resourceType": "Document", "requestStage": "Response"},
		}, "handleAuthRequests": true,
	}, &result)
	if err != nil {
		d.Close()
		return err
	}
	return nil
}

func (d *directCDPDispatcher) handlePolicyEventLocked(ctx context.Context, method, session string, params json.RawMessage) error {
	if method != "Fetch.requestPaused" && method != "Fetch.authRequired" {
		return nil
	}
	if d.policy == nil {
		return errDirectCDPRetired
	}
	target := d.policy.targets[session]
	if target == nil {
		if d.policy.retired[session] {
			return nil
		}
		return errDirectCDPRetired
	}
	if !target.policyEnabled {
		return errDirectCDPRetired
	}
	if target.retired.Load() {
		return nil
	} // A closed owned session grants no further writes.
	var event struct {
		RequestID    string `json:"requestId"`
		ResourceType string `json:"resourceType"`
		Request      struct {
			URL    string `json:"url"`
			Method string `json:"method"`
		} `json:"request"`
		ResponseStatusCode  *int    `json:"responseStatusCode"`
		ResponseErrorReason *string `json:"responseErrorReason"`
		ResponseHeaders     []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"responseHeaders"`
	}
	if json.Unmarshal(params, &event) != nil || event.RequestID == "" || len(event.RequestID) > 256 {
		return errDirectCDPRetired
	}
	if method == "Fetch.authRequired" {
		return d.policyCommandLocked(ctx, session, "Fetch.continueWithAuth", map[string]any{"requestId": event.RequestID, "authChallengeResponse": map[string]any{"response": "CancelAuth"}})
	}
	if event.ResourceType != "Document" {
		return errDirectCDPRetired
	}
	allowed := target.originScope.Allows(event.Request.URL) && (event.Request.Method == "GET" || event.Request.Method == "HEAD")
	response := event.ResponseStatusCode != nil || event.ResponseErrorReason != nil
	if event.ResponseErrorReason != nil {
		allowed = false
	}
	for _, header := range event.ResponseHeaders {
		if strings.EqualFold(header.Name, "Content-Disposition") && strings.HasPrefix(strings.ToLower(strings.TrimSpace(header.Value)), "attachment") {
			allowed = false
		}
	}
	command := "Fetch.continueRequest"
	arguments := map[string]any{"requestId": event.RequestID}
	if !allowed {
		command = "Fetch.failRequest"
		arguments["errorReason"] = "BlockedByClient"
	} else if response {
		command = "Fetch.continueResponse"
	}
	return d.policyCommandLocked(ctx, session, command, arguments)
}

func (d *directCDPDispatcher) retirePolicySessionLocked() {
	if d.policy == nil || !d.policyEnabled {
		return
	}
	delete(d.policy.targets, d.sessionID)
	if d.policy.retired == nil {
		d.policy.retired = make(map[string]bool)
	}
	if d.policy.retired[d.sessionID] {
		return
	}
	d.policy.retired[d.sessionID] = true
	d.policy.retiredOrder = append(d.policy.retiredOrder, d.sessionID)
	if len(d.policy.retiredOrder) > 1024 {
		delete(d.policy.retired, d.policy.retiredOrder[0])
		d.policy.retiredOrder = d.policy.retiredOrder[1:]
	}
}

// Only the synchronous Core dispatcher writes control commands; each is
// correlated and acknowledged before the original operation can complete.
// This avoids a second reader/writer or unbounded background worker per page.
func (d *directCDPDispatcher) policyCommandLocked(ctx context.Context, session, method string, params any) error {
	if ctx.Err() != nil || !d.live() || len(d.policy.pending) >= 128 {
		return errDirectCDPRetired
	}
	sequence := &d.nextID
	if d.sharedSequence != nil {
		sequence = d.sharedSequence
	}
	if *sequence == math.MaxInt64 {
		return errDirectCDPRetired
	}
	*sequence++
	id := *sequence
	raw, err := json.Marshal(directCDPCommand{ID: id, Method: method, SessionID: session, Params: params})
	if err != nil {
		return errDirectCDPRetired
	}
	d.policy.pending[id] = session
	return d.down.WriteMessage(websocket.TextMessage, raw)
}
