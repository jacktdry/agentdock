package browser

import (
	"context"
	"fmt"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/uvwt/agentdock/internal/browserpolicy"
)

func TestDirectCDPSharedConnectionReleasesOnlyOwnedTarget(t *testing.T) {
	// A real loopback WebSocket with distinct target/session records. No browser,
	// user Profile, authorization prompt or process launch is part of this test.
	urls := make(map[string]string)
	sessions := make(map[string]string)
	next := 0
	root, f := newDispatcherFixture(t, func(_ *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
		var result any
		switch cmd.Method {
		case "Target.getBrowserContexts":
			result = map[string]any{"browserContextIds": []string{}, "defaultBrowserContextId": "company"}
		case "Target.createTarget":
			if _, present := cmd.Params["newWindow"]; present {
				dispatcherProtocolError(c, cmd)
				return true
			}
			next++
			id := fmt.Sprintf("target-%d", next)
			urls[id] = cmd.Params["url"].(string)
			result = map[string]any{"targetId": id}
		case "Target.attachToTarget":
			id := cmd.Params["targetId"].(string)
			session := "session-" + id
			sessions[session] = id
			result = map[string]any{"sessionId": session}
		case "Target.getTargetInfo":
			id := cmd.Params["targetId"].(string)
			u, ok := urls[id]
			if !ok {
				dispatcherProtocolError(c, cmd)
				return true
			}
			result = map[string]any{"targetInfo": map[string]any{"targetId": id, "type": "page", "url": u, "browserContextId": "company"}}
		case "Page.navigate":
			id := sessions[cmd.SessionID]
			if _, ok := urls[id]; !ok {
				dispatcherProtocolError(c, cmd)
				return true
			}
			urls[id] = cmd.Params["url"].(string)
			result = map[string]any{}
		case "Target.closeTarget":
			id := cmd.Params["targetId"].(string)
			_, exists := urls[id]
			delete(urls, id)
			result = map[string]any{"success": exists}
		default:
			return false
		}
		if err := dispatcherReply(c, cmd, result); err != nil {
			t.Error(err)
		}
		return true
	})
	ctx := context.Background()
	if err := root.bindBrowserContext(ctx, "company"); err != nil {
		t.Fatal(err)
	}
	scopeA, _ := browserpolicy.NewOriginScope([]string{"https://example.invalid"})
	scopeB, _ := browserpolicy.NewOriginScope([]string{"https://cms.example.com"})
	a, err := root.forkTarget(scopeA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := root.forkTarget(scopeB)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.OpenOwnedTarget(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err = b.OpenOwnedTarget(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if a.targetID == b.targetID || a.sessionID == b.sessionID {
		t.Fatal("sibling ownership aliased")
	}
	if err = a.Navigate(ctx, "https://example.invalid/"); err != nil {
		t.Fatal(err)
	}
	if err = b.Navigate(ctx, "https://cms.example.com/"); err != nil {
		t.Fatal(err)
	}
	before := len(f.recorded())
	if a.Navigate(ctx, "https://cms.example.com/") == nil || len(f.recorded()) != before {
		t.Fatal("sibling origin scope reached transport")
	}
	// Cancellation while queued has sent no bytes and must not discard the
	// shared authorization or kill another owner's still-active page.
	<-root.gate
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if closed, err := a.CloseOwnedTarget(canceled); err == nil || closed {
		t.Fatal("canceled cleanup claimed completion")
	}
	root.gate <- struct{}{}
	if !root.live() || !b.live() {
		t.Fatal("queued cancellation dropped sibling transport")
	}
	if closed, err := a.CloseOwnedTarget(ctx); err != nil || !closed {
		t.Fatal("first owned target close failed")
	}
	if !root.live() || !b.live() {
		t.Fatal("first release dropped authorized shared connection")
	}
	if _, err := a.Snapshot(ctx, false); err == nil || !root.live() || !b.live() {
		t.Fatal("released target snapshot discarded sibling connection")
	}
	if _, err := b.EvaluateScript(ctx, "() => 'still alive'"); err != nil {
		t.Fatal("sibling stopped after release", err)
	}
	if closed, err := b.CloseOwnedTarget(ctx); err != nil || !closed {
		t.Fatal("second owned target close failed")
	}
	if _, err := a.EvaluateScript(ctx, "() => 1"); err == nil {
		t.Fatal("released target reused")
	}
	if !root.live() || f.connections.Load() != 1 {
		t.Fatal("release redialed or closed source")
	}
	var previous int64
	for _, cmd := range f.recorded() {
		if cmd.ID <= previous {
			t.Fatal("command sequence reused across target dispatchers")
		}
		previous = cmd.ID
	}
	if commandCount(f.recorded(), "Target.closeTarget") != 2 || commandCount(f.recorded(), "Browser.close") != 0 {
		t.Fatal("cleanup crossed ownership")
	}
	root.Close()
	if _, err := root.forkTarget(scopeA); err == nil {
		t.Fatal("closed source reconnected")
	}
}
