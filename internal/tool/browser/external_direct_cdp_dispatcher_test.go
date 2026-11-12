package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/uvwt/agentdock/internal/browserpolicy"
)

type dispatcherCommand struct {
	ID        int64          `json:"id"`
	Method    string         `json:"method"`
	SessionID string         `json:"sessionId"`
	Params    map[string]any `json:"params"`
}
type dispatcherFixture struct {
	mu            sync.Mutex
	commands      []dispatcherCommand
	connections   atomic.Int32
	snapshotCalls int
	currentURL    string
}
type dispatcherResponder func(*dispatcherFixture, *websocket.Conn, dispatcherCommand) bool

func dispatcherReply(c *websocket.Conn, cmd dispatcherCommand, result any) error {
	return c.WriteJSON(struct {
		ID        int64  `json:"id"`
		SessionID string `json:"sessionId,omitempty"`
		Result    any    `json:"result"`
	}{cmd.ID, cmd.SessionID, result})
}
func dispatcherProtocolError(c *websocket.Conn, cmd dispatcherCommand) {
	_ = c.WriteJSON(map[string]any{"id": cmd.ID, "sessionId": cmd.SessionID, "error": map[string]any{"code": -32000, "message": "synthetic"}})
}

func dispatcherDefaultReply(t *testing.T, f *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
	t.Helper()
	var result any = map[string]any{}
	switch cmd.Method {
	case "Target.createTarget":
		if _, present := cmd.Params["newWindow"]; present {
			dispatcherProtocolError(c, cmd)
			return true
		}
		f.mu.Lock()
		f.currentURL, _ = cmd.Params["url"].(string)
		f.mu.Unlock()
		result = map[string]any{"targetId": "owned-target"}
	case "Target.attachToTarget":
		result = map[string]any{"sessionId": "owned-session"}
	case "Target.getTargetInfo":
		f.mu.Lock()
		currentURL := f.currentURL
		f.mu.Unlock()
		result = map[string]any{"targetInfo": map[string]any{"targetId": "owned-target", "type": "page", "url": currentURL}}
	case "Target.closeTarget":
		result = map[string]any{"success": true}
	case "Runtime.evaluate":
		result = map[string]any{"result": map[string]any{"value": "eval-ok"}}
	case "Accessibility.getFullAXTree":
		f.mu.Lock()
		f.snapshotCalls++
		g := f.snapshotCalls
		f.mu.Unlock()
		base := 100 * g
		result = map[string]any{"nodes": []any{map[string]any{"backendDOMNodeId": base + 1, "role": map[string]any{"value": "button"}, "name": map[string]any{"value": fmt.Sprintf("Submit-%d", g)}}, map[string]any{"backendDOMNodeId": base + 2, "role": map[string]any{"value": "textbox"}, "name": map[string]any{"value": "Name"}}}}
	case "DOM.resolveNode":
		backend, _ := cmd.Params["backendNodeId"].(float64)
		result = map[string]any{"object": map[string]any{"objectId": fmt.Sprintf("object-%d", int64(backend))}}
	case "Runtime.callFunctionOn":
		result = map[string]any{"result": map[string]any{"value": true}}
	case "Page.navigate":
		f.mu.Lock()
		f.currentURL, _ = cmd.Params["url"].(string)
		f.mu.Unlock()
		result = map[string]any{}
	case "Runtime.releaseObject", "Input.dispatchKeyEvent":
		result = map[string]any{}
	case "Page.captureScreenshot":
		result = map[string]any{"data": "aGVsbG8="}
	default:
		t.Errorf("unexpected method %s", cmd.Method)
		return false
	}
	if err := dispatcherReply(c, cmd, result); err != nil {
		t.Error(err)
		return false
	}
	return true
}

func newDispatcherFixture(t *testing.T, responder dispatcherResponder) (*directCDPDispatcher, *dispatcherFixture) {
	t.Helper()
	f := &dispatcherFixture{}
	done := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.connections.Add(1)
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer close(done)
		defer c.Close()
		_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
		for {
			kind, raw, err := c.ReadMessage()
			if err != nil {
				return
			}
			var cmd dispatcherCommand
			if kind != websocket.TextMessage || json.Unmarshal(raw, &cmd) != nil {
				t.Error("invalid command")
				return
			}
			f.mu.Lock()
			f.commands = append(f.commands, cmd)
			f.mu.Unlock()
			if responder != nil && responder(f, c, cmd) {
				continue
			}
			if !dispatcherDefaultReply(t, f, c, cmd) {
				return
			}
		}
	}))
	t.Cleanup(s.Close)
	peer := pinnedTestPeer()
	relay, err := newDirectCDPRelay(context.Background(), "ws"+strings.TrimPrefix(s.URL, "http")+"/devtools/browser/fixture", peer, pinnedTestAttestor(peer))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { relay.Close(); pinnedWait(t, relay.Done()); pinnedWait(t, done) })
	scope, err := browserpolicy.NewOriginScope([]string{"https://example.invalid", "https://cms.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := newDirectCDPDispatcher(context.Background(), relay, scope)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d, f
}
func (f *dispatcherFixture) recorded() []dispatcherCommand {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]dispatcherCommand(nil), f.commands...)
}
func commandCount(cmds []dispatcherCommand, m string) int {
	n := 0
	for _, c := range cmds {
		if c.Method == m {
			n++
		}
	}
	return n
}

func TestDirectCDPDispatcherOwnedTargetAndOperations(t *testing.T) {
	d, f := newDispatcherFixture(t, nil)
	ctx := context.Background()
	if err := d.OpenOwnedTarget(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := d.Navigate(ctx, "https://example.invalid/start"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.EvaluateScript(ctx, "() => 'ok'"); err != nil {
		t.Fatal(err)
	}
	snap, err := d.Snapshot(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	msg := snap["structuredContent"].(map[string]any)["message"].(string)
	if !strings.Contains(msg, "uid=d1_0") || !strings.Contains(msg, "uid=d1_1") {
		t.Fatal(msg)
	}
	if _, err := d.Click(ctx, "d1_0"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Fill(ctx, "d1_1", "Alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PressKey(ctx, "Enter"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Screenshot(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := d.Navigate(ctx, "https://example.invalid/next"); err != nil {
		t.Fatal(err)
	}
	before := commandCount(f.recorded(), "DOM.resolveNode")
	if _, err := d.Click(ctx, "d1_0"); err == nil {
		t.Fatal("navigation did not invalidate uid")
	}
	if commandCount(f.recorded(), "DOM.resolveNode") != before {
		t.Fatal("stale uid resolved")
	}
	if _, err := d.Snapshot(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Click(ctx, "d1_0"); err == nil {
		t.Fatal("old generation accepted")
	}
	if _, err := d.Click(ctx, "d2_0"); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.CloseOwnedTarget(ctx); !ok || err != nil {
		t.Fatalf("close=%v %v", ok, err)
	}
	cmds := f.recorded()
	if f.connections.Load() != 1 {
		t.Fatal("redial")
	}
	for _, m := range []string{"Target.getTargets", "Target.setDiscoverTargets", "Target.setAutoAttach", "Target.activateTarget", "Browser.close", "Browser.setWindowBounds"} {
		if commandCount(cmds, m) != 0 {
			t.Fatalf("forbidden %s", m)
		}
	}
	create := cmds[0]
	_, hasNewWindow := create.Params["newWindow"]
	if create.Method != "Target.createTarget" || create.Params["url"] != "about:blank" || create.Params["background"] != true || create.Params["hidden"] != true || hasNewWindow || create.Params["forTab"] != false {
		t.Fatalf("create=%+v", create)
	}
	for _, c := range cmds {
		switch c.Method {
		case "Target.createTarget", "Target.attachToTarget", "Target.getTargetInfo", "Target.closeTarget":
			if c.SessionID != "" {
				t.Fatalf("browser-level session %+v", c)
			}
			if c.Method != "Target.createTarget" && c.Params["targetId"] != "owned-target" {
				t.Fatalf("wrong target %+v", c)
			}
		default:
			if c.SessionID != "owned-session" {
				t.Fatalf("escaped session %+v", c)
			}
		}
	}
	if commandCount(cmds, "Target.closeTarget") != 1 {
		t.Fatal("close count")
	}
}

func TestDirectCDPDispatcherUIDBinding(t *testing.T) {
	d, f := newDispatcherFixture(t, nil)
	if err := d.OpenOwnedTarget(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Snapshot(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	before := len(f.recorded())
	if _, err := d.Click(context.Background(), "unknown"); err == nil {
		t.Fatal("unknown uid")
	}
	for _, c := range f.recorded()[before:] {
		if c.Method == "DOM.resolveNode" {
			t.Fatal("unknown uid resolved")
		}
	}
	if _, err := d.Click(context.Background(), "d1_0"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Fill(context.Background(), "d1_1", "x"); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, c := range f.recorded() {
		if c.Method == "DOM.resolveNode" {
			ids = append(ids, int64(c.Params["backendNodeId"].(float64)))
		}
	}
	if fmt.Sprint(ids) != "[101 102]" {
		t.Fatal(ids)
	}
}

func TestDirectCDPDispatcherSecondClaimDoesNotDisruptFirst(t *testing.T) {
	d, f := newDispatcherFixture(t, nil)
	if err := d.OpenOwnedTarget(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	second, err := newDirectCDPDispatcher(context.Background(), d.relay, d.originScope)
	if err == nil || second != nil {
		t.Fatal("second claimed")
	}
	if d.relay.Lost() || !d.live() {
		t.Fatal("second destroyed first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	second, err = newDirectCDPDispatcher(ctx, d.relay, d.originScope)
	if err == nil || second != nil {
		t.Fatal("canceled second claimed")
	}
	if d.relay.Lost() || !d.live() {
		t.Fatal("canceled second destroyed first")
	}
	if _, err := d.EvaluateScript(context.Background(), "() => 1"); err != nil {
		t.Fatal(err)
	}
	if f.connections.Load() != 1 {
		t.Fatal("upstream redial")
	}
}

func TestDirectCDPDispatcherCancellationAfterWriteRetires(t *testing.T) {
	block := make(chan struct{})
	d, f := newDispatcherFixture(t, func(_ *dispatcherFixture, _ *websocket.Conn, c dispatcherCommand) bool {
		if c.Method == "Runtime.evaluate" {
			<-block
			return true
		}
		return false
	})
	if err := d.OpenOwnedTarget(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err := d.EvaluateScript(ctx, "() => 1")
	close(block)
	if err == nil || d.live() || !d.relay.Lost() {
		t.Fatal("not retired")
	}
	before := len(f.recorded())
	if _, err := d.EvaluateScript(context.Background(), "() => 2"); err == nil {
		t.Fatal("reused")
	}
	if len(f.recorded()) != before {
		t.Fatal("sent after retire")
	}
}

func TestDirectCDPDispatcherTargetMismatchFailsClosed(t *testing.T) {
	d, f := newDispatcherFixture(t, func(_ *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
		if cmd.Method == "Target.getTargetInfo" {
			_ = dispatcherReply(c, cmd, map[string]any{"targetInfo": map[string]any{"targetId": "user-target", "type": "page", "url": "about:blank"}})
			return true
		}
		return false
	})
	if d.OpenOwnedTarget(context.Background(), "") == nil {
		t.Fatal("mismatch accepted")
	}
	if d.live() {
		t.Fatal("not retired")
	}
	if commandCount(f.recorded(), "Target.closeTarget") != 1 {
		t.Fatal("cleanup missing")
	}
}

func TestDirectCDPDispatcherOriginScope(t *testing.T) {
	d, f := newDispatcherFixture(t, nil)
	ctx := context.Background()
	if err := d.OpenOwnedTarget(ctx, "https://example.invalid/"); err == nil {
		t.Fatal("non-blank bootstrap target accepted")
	}
	if commandCount(f.recorded(), "Target.createTarget") != 0 {
		t.Fatal("rejected bootstrap sent createTarget")
	}
	if err := d.OpenOwnedTarget(ctx, ""); err != nil {
		t.Fatal(err)
	}
	before := commandCount(f.recorded(), "Page.navigate")
	for _, raw := range []string{"about:blank", "http://example.invalid/", "https://evil.invalid/", "https://example.invalid.evil/"} {
		if err := d.Navigate(ctx, raw); err == nil {
			t.Fatalf("out-of-scope navigation accepted: %s", raw)
		}
	}
	if commandCount(f.recorded(), "Page.navigate") != before {
		t.Fatal("out-of-scope navigation reached CDP")
	}
	if err := d.Navigate(ctx, "https://example.invalid:443/path?q=1"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.EvaluateScript(ctx, "() => 1"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.currentURL = "https://evil.invalid/redirected"
	f.mu.Unlock()
	before = commandCount(f.recorded(), "Runtime.evaluate")
	if _, err := d.EvaluateScript(ctx, "() => 2"); err == nil {
		t.Fatal("out-of-scope current target remained usable")
	}
	if commandCount(f.recorded(), "Runtime.evaluate") != before {
		t.Fatal("out-of-scope target executed operation")
	}
	if d.live() {
		t.Fatal("out-of-scope target did not retire dispatcher")
	}
}

func TestDirectCDPDispatcherInputValidation(t *testing.T) {
	d, f := newDispatcherFixture(t, nil)
	if err := d.OpenOwnedTarget(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	before := len(f.recorded())
	for _, k := range []string{"", "Control+A", "A\n"} {
		if _, err := d.PressKey(context.Background(), k); err == nil {
			t.Fatalf("accepted %q", k)
		}
	}
	if len(f.recorded()) != before {
		t.Fatal("invalid key sent")
	}
	if _, err := d.PressKey(context.Background(), "Enter"); err != nil {
		t.Fatal(err)
	}
}

func TestDirectCDPDispatcherProtocolErrorDoesNotMintSuccess(t *testing.T) {
	failed := false
	d, _ := newDispatcherFixture(t, func(_ *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
		if !failed && cmd.Method == "Runtime.evaluate" {
			failed = true
			dispatcherProtocolError(c, cmd)
			return true
		}
		return false
	})
	if err := d.OpenOwnedTarget(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := d.EvaluateScript(context.Background(), "() => 1"); err == nil {
		t.Fatal("protocol success")
	}
	if !d.live() {
		t.Fatal("transport retired")
	}
}
