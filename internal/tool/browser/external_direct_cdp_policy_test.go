package browser

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uvwt/agentdock/internal/browserpolicy"
)

func TestDirectCDPPolicyChecksTheEventOwnersOriginAndResponse(t *testing.T) {
	for _, tc := range []struct {
		name, session, method, url, verb, want string
		response                               bool
		headers                                []any
	}{
		{name: "allowed request", url: "https://example.invalid/page", verb: "GET", want: "Fetch.continueRequest"},
		{name: "exact origin only", url: "https://example.invalid.evil/page", verb: "GET", want: "Fetch.failRequest"},
		{name: "redirect hop outside allowlist", url: "https://evil.invalid/redirect", verb: "GET", want: "Fetch.failRequest"},
		{name: "post document", url: "https://example.invalid/page", verb: "POST", want: "Fetch.failRequest"},
		{name: "allowed response", url: "https://example.invalid/page", verb: "GET", response: true, want: "Fetch.continueResponse"},
		{name: "attachment", url: "https://example.invalid/page", verb: "GET", response: true, headers: []any{map[string]any{"name": "CONTENT-Disposition", "value": " Attachment; filename=a.pdf"}}, want: "Fetch.failRequest"},
		{name: "auth challenge canceled", method: "Fetch.authRequired", url: "https://example.invalid/", want: "Fetch.continueWithAuth"},
		{name: "sibling allowed only by sibling scope", session: "sibling-session", url: "https://cms.example.com/", verb: "GET", want: "Fetch.continueRequest"},
		{name: "sibling cannot borrow callers scope", session: "sibling-session", url: "https://example.invalid/", verb: "GET", want: "Fetch.failRequest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := tc.session
			if session == "" {
				session = "owned-session"
			}
			method := tc.method
			if method == "" {
				method = "Fetch.requestPaused"
			}
			d, f := newDispatcherFixture(t, func(_ *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
				switch cmd.Method {
				case "Fetch.enable":
					_ = dispatcherReply(c, cmd, map[string]any{})
					return true
				case "Runtime.evaluate":
					params := map[string]any{"requestId": "paused", "resourceType": "Document", "request": map[string]any{"url": tc.url, "method": tc.verb}}
					if tc.response {
						params["responseStatusCode"], params["responseHeaders"] = 200, tc.headers
					}
					_ = c.WriteJSON(map[string]any{"method": method, "sessionId": session, "params": params})
					// The operation result arrives BEFORE the policy ACK. It must
					// remain quarantined until the generated control is acknowledged.
					_ = dispatcherReply(c, cmd, map[string]any{"result": map[string]any{"value": "ok"}})
					return true
				case "Fetch.continueRequest", "Fetch.failRequest", "Fetch.continueResponse", "Fetch.continueWithAuth":
					_ = dispatcherReply(c, cmd, map[string]any{})
					return true
				}
				return false
			})
			ctx := context.Background()
			if err := d.OpenOwnedTarget(ctx, ""); err != nil {
				t.Fatal(err)
			}
			d.originScope, _ = browserpolicy.NewOriginScope([]string{"https://example.invalid"})
			if err := d.enableOriginPolicy(ctx); err != nil {
				t.Fatal(err)
			}
			siblingScope, _ := browserpolicy.NewOriginScope([]string{"https://cms.example.com"})
			d.policy.targets["sibling-session"] = &directCDPDispatcher{originScope: siblingScope, policyEnabled: true}
			if _, err := d.Evaluate(ctx, "'read only'"); err != nil {
				t.Fatal(err)
			}
			var controls []dispatcherCommand
			for _, cmd := range f.recorded() {
				if cmd.Method == tc.want {
					controls = append(controls, cmd)
				}
			}
			if len(controls) != 1 || controls[0].SessionID != session || controls[0].Params["requestId"] != "paused" || len(d.policy.pending) != 0 {
				t.Fatal("policy control escaped event ownership or returned before ACK", controls)
			}
			if tc.want == "Fetch.continueWithAuth" && controls[0].Params["authChallengeResponse"].(map[string]any)["response"] != "CancelAuth" {
				t.Fatal("authentication prompted or supplied credentials")
			}
		})
	}
}

func TestDirectCDPPolicyUnacknowledgedControlCancelsWholeSource(t *testing.T) {
	controlSent, unblock := make(chan struct{}), make(chan struct{})
	d, f := newDispatcherFixture(t, func(_ *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
		switch cmd.Method {
		case "Fetch.enable":
			_ = dispatcherReply(c, cmd, map[string]any{})
			return true
		case "Runtime.evaluate":
			_ = c.WriteJSON(map[string]any{"method": "Fetch.requestPaused", "sessionId": cmd.SessionID, "params": map[string]any{"requestId": "paused", "resourceType": "Document", "request": map[string]any{"url": "https://example.invalid/", "method": "GET"}}})
			_ = dispatcherReply(c, cmd, map[string]any{"result": map[string]any{"value": "must-not-return"}})
			return true
		case "Fetch.continueRequest":
			close(controlSent)
			<-unblock
			return true
		}
		return false
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.OpenOwnedTarget(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := d.enableOriginPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := d.Evaluate(ctx, "'read only'"); done <- err }()
	pinnedWait(t, controlSent)
	select {
	case <-done:
		t.Error("operation returned before policy ACK")
	default:
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || d.live() {
			t.Error("canceled control returned data or retained authority")
		}
	case <-time.After(time.Second):
		t.Error("policy read did not join cancellation")
	}
	close(unblock)
	before := len(f.recorded())
	if _, err := d.Snapshot(context.Background(), false); err == nil || len(f.recorded()) != before {
		t.Fatal("retired source accepted further commands")
	}
}

func TestDirectCDPPolicyRetiredSessionDoesNotWrite(t *testing.T) {
	d, f := newDispatcherFixture(t, nil)
	d.policy.retired = map[string]bool{"released": true}
	ctx, unlock, err := d.lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	before := len(f.recorded())
	if err := d.handlePolicyEventLocked(ctx, "Fetch.requestPaused", "released", json.RawMessage(`{}`)); err != nil || len(f.recorded()) != before {
		t.Fatal("released session emitted a control")
	}
	if err := d.handlePolicyEventLocked(ctx, "Fetch.requestPaused", "unknown-user-session", json.RawMessage(`{}`)); err == nil {
		t.Fatal("unknown session was trusted")
	}
}
