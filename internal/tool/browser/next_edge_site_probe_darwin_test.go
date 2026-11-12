//go:build darwin

package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/uvwt/agentdock/internal/browserpolicy"
)

func TestNextEdgeSiteScopeRequiresExactApprovedGitWorktree(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	for _, test := range []struct {
		path  string
		count int
	}{
		{"opruebaCode/official_site/oprueba", 2}, {"opruebaCode/official_site/homiplay", 2},
		{"opruebaCode/homiplay", 2}, {"opruebaCode/official_site/content-cms", 4},
	} {
		root := filepath.Join(home, test.path)
		if err := os.MkdirAll(filepath.Join(root, ".git"), 0700); err != nil {
			t.Fatal(err)
		}
		origins, err := nextEdgeProbeScopeForWorkspace(root)
		if err != nil || len(origins) != test.count {
			t.Fatal("approved exact workspace scope rejected", err)
		}
	}
	for _, root := range []string{home, filepath.Join(home, "opruebaCode"), filepath.Join(home, "other"), ""} {
		result, err := ProbeNextEdgeSites(context.Background(), root)
		if err == nil || result.Stage != "workspace_scope" || result.BrowserVersion != "" {
			t.Fatal("unapproved workspace reached browser probe")
		}
	}
}

func TestNextEdgeSiteSmokeReusesOneSocketWithSeparateOwnedTargets(t *testing.T) {
	const profile = "/owned/edge/Default"
	responder := contextResponder(t, "company", profile)
	d, f := newDispatcherFixture(t, func(f *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
		switch cmd.Method {
		case "Runtime.evaluate":
			if cmd.Params["expression"] == `({ready:document.readyState,origin:location.origin})` {
				f.mu.Lock()
				origin := f.currentURL
				f.mu.Unlock()
				_ = dispatcherReply(c, cmd, map[string]any{"result": map[string]any{"value": map[string]any{"ready": "complete", "origin": origin}}})
				return true
			}
		case "Page.captureScreenshot":
			_ = dispatcherReply(c, cmd, map[string]any{"data": "iVBORw0KGgo="})
			return true
		}
		return responder(f, c, cmd)
	})
	d.originScope, _ = browserpolicy.NewOriginScope(nextEdgeProbeOrigins[:])
	ctx := context.Background()
	if err := d.bindBrowserContext(ctx, "company"); err != nil {
		t.Fatal(err)
	}
	probe, err := d.forkTarget(d.originScope)
	if err != nil {
		t.Fatal(err)
	}
	if err := probe.OpenOwnedTarget(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := probe.inspectOwnedEdgeProfile(ctx, "/owned/edge"); err != nil {
		t.Fatal(err)
	}
	if closed, err := probe.CloseOwnedTarget(ctx); err != nil || !closed {
		t.Fatal("Profile probe cleanup unconfirmed")
	}
	checks, err := d.probeApprovedSites(ctx)
	if err != nil || len(checks) != 4 {
		t.Fatalf("checks=%v err=%v", checks, err)
	}
	cmds := f.recorded()
	if f.connections.Load() != 1 || commandCount(cmds, "Target.createTarget") != 5 || commandCount(cmds, "Target.closeTarget") != 5 || !d.live() {
		t.Fatal("site smoke duplicated connections or targets")
	}
	if commandCount(cmds, "Accessibility.getFullAXTree") != 4 || commandCount(cmds, "Page.captureScreenshot") != 4 {
		t.Fatal("missing site observation")
	}
	for _, c := range cmds {
		switch c.Method {
		case "Target.getTargets", "Target.activateTarget", "Browser.close", "Input.dispatchKeyEvent", "Runtime.callFunctionOn":
			t.Fatal("site probe dispatched unapproved operation")
		case "Page.navigate":
			u, _ := c.Params["url"].(string)
			if u != "edge://version/" && !d.originScope.Allows(u) {
				t.Fatal("site probe navigated outside exact allowlist")
			}
		}
	}
}
