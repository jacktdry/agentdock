package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func contextResponder(t *testing.T, contextID, profilePath string) dispatcherResponder {
	t.Helper()
	return func(f *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
		var result any
		switch cmd.Method {
		case "Target.getBrowserContexts":
			result = map[string]any{"browserContextIds": []string{"unselected", "company"}, "defaultBrowserContextId": "unselected"}
		case "Target.getTargetInfo":
			f.mu.Lock()
			u := f.currentURL
			f.mu.Unlock()
			result = map[string]any{"targetInfo": map[string]any{"targetId": "owned-target", "type": "page", "url": u, "browserContextId": contextID}}
		case "Runtime.evaluate":
			result = map[string]any{"result": map[string]any{"value": profilePath}}
		default:
			return false
		}
		if err := dispatcherReply(c, cmd, result); err != nil {
			t.Error(err)
		}
		return true
	}
}

func TestDirectCDPContextNeverFallsBackToDefault(t *testing.T) {
	for _, selected := range []string{"company", "missing"} {
		t.Run(selected, func(t *testing.T) {
			d, f := newDispatcherFixture(t, contextResponder(t, "company", ""))
			err := d.bindBrowserContext(context.Background(), selected)
			if selected == "missing" {
				if err == nil || d.live() || d.OpenOwnedTarget(context.Background(), "") == nil {
					t.Fatal("missing company context fell back")
				}
				if commandCount(f.recorded(), "Target.createTarget") != 0 {
					t.Fatal("unselected Profile was touched")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := d.OpenOwnedTarget(context.Background(), ""); err != nil {
				t.Fatal(err)
			}
			for _, c := range f.recorded() {
				if c.Method == "Target.createTarget" && c.Params["browserContextId"] != "company" {
					t.Fatal("default context used")
				}
			}
			if err := d.bindBrowserContext(context.Background(), "unselected"); err == nil {
				t.Fatal("live target rebound")
			}
		})
	}
}

func TestDirectCDPContextDriftRejectsBeforeCompanyNavigation(t *testing.T) {
	d, f := newDispatcherFixture(t, contextResponder(t, "wrong-context", ""))
	ctx := context.Background()
	if err := d.bindBrowserContext(ctx, "company"); err != nil {
		t.Fatal(err)
	}
	if d.OpenOwnedTarget(ctx, "") == nil || d.live() {
		t.Fatal("wrong Profile target accepted")
	}
	if commandCount(f.recorded(), "Target.closeTarget") != 1 || commandCount(f.recorded(), "Page.navigate") != 0 {
		t.Fatal("wrong target cleanup/navigation")
	}
}

func TestDirectCDPExplicitDefaultContextStillRequiresSameIncarnation(t *testing.T) {
	for _, drift := range []bool{false, true} {
		d, f := newDispatcherFixture(t, contextResponder(t, "", ""))
		ctx := context.Background()
		if err := d.bindBrowserContext(ctx, "unselected"); err != nil {
			t.Fatal(err)
		}
		if err := d.OpenOwnedTarget(ctx, ""); err != nil {
			t.Fatal(err)
		}
		for _, cmd := range f.recorded() {
			if cmd.Method == "Target.createTarget" {
				if _, present := cmd.Params["browserContextId"]; present {
					t.Fatal("default context encoded as an incognito context")
				}
			}
		}
		if drift {
			d.browserContextID = "replacement"
		}
		err := d.Navigate(ctx, "https://example.invalid/")
		if drift {
			if err == nil || commandCount(f.recorded(), "Page.navigate") != 0 {
				t.Fatal("replaced default context navigated company website")
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
}

// Use OS-native absolute paths: inspecting a Windows profile must not be
// rejected merely because a test fixture supplied a Unix-only root.
func testOwnedEdgeDataDir() string {
	return filepath.Join(os.TempDir(), "agentdock-owned-edge")
}

func TestDirectCDPOwnedProfileProbeCannotBecomeCompanyTarget(t *testing.T) {
	for _, profile := range []string{
		filepath.Join(testOwnedEdgeDataDir(), "Default"),
		filepath.Join(os.TempDir(), "agentdock-other-edge", "Default"),
		testOwnedEdgeDataDir() + string(filepath.Separator) + ".." + string(filepath.Separator) + "other",
	} {
		t.Run(profile, func(t *testing.T) {
			d, f := newDispatcherFixture(t, contextResponder(t, "company", profile))
			ctx := context.Background()
			if err := d.bindBrowserContext(ctx, "company"); err != nil {
				t.Fatal(err)
			}
			if err := d.OpenOwnedTarget(ctx, ""); err != nil {
				t.Fatal(err)
			}
			got, err := d.inspectOwnedEdgeProfile(ctx, testOwnedEdgeDataDir())
			if profile == filepath.Join(testOwnedEdgeDataDir(), "Default") {
				if err != nil || got != "Default" {
					t.Fatalf("profile=%q err=%v", got, err)
				}
				// A metadata probe grants no company origin or authentication.
				if d.Navigate(ctx, "https://example.invalid/") == nil {
					t.Fatal("metadata target reused for company operation")
				}
			} else if err == nil || got != "" || d.live() {
				t.Fatal("wrong Profile root accepted")
			}
			for _, c := range f.recorded() {
				if c.Method == "Page.navigate" && c.Params["url"] != "edge://version/" {
					t.Fatal("probe navigated company website")
				}
				if c.Method == "Runtime.evaluate" && c.Params["expression"] != `document.getElementById('profile_path')?.textContent ?? ''` {
					t.Fatal("arbitrary probe evaluation")
				}
			}
		})
	}
}

func TestEdgeProfileDirectoryRejectsTraversalAndGuest(t *testing.T) {
	for _, name := range []string{"Default", "Profile 1", "Profile 123"} {
		if !edgeProfileDirectoryValid(name) {
			t.Fatal(name)
		}
	}
	for _, name := range []string{"", ".", "..", "Guest Profile", "System Profile", "Profile 01", "Profile -1", "Profile 1/../Default", "Profile 1\x00"} {
		if edgeProfileDirectoryValid(name) {
			t.Fatal(name)
		}
	}
	// Do not accidentally admit a path containing a valid-looking suffix.
	if edgeProfileDirectoryValid(filepath.Join("/tmp", "Default")) {
		t.Fatal("absolute Profile directory admitted")
	}
}

func TestDirectCDPOtherTypeRequiresOwnedHiddenTargetAndSameContext(t *testing.T) {
	for _, kind := range []string{"page", "other", "tab", "iframe", "service_worker"} {
		for _, contextID := range []string{"company", "wrong-context"} {
			t.Run(kind+"/"+contextID, func(t *testing.T) {
				base := contextResponder(t, contextID, filepath.Join(testOwnedEdgeDataDir(), "Default"))
				d, f := newDispatcherFixture(t, func(f *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
					if cmd.Method != "Target.getTargetInfo" {
						return base(f, c, cmd)
					}
					f.mu.Lock()
					u := f.currentURL
					f.mu.Unlock()
					if err := dispatcherReply(c, cmd, map[string]any{"targetInfo": map[string]any{
						"targetId": "owned-target", "type": kind, "url": u, "browserContextId": contextID,
					}}); err != nil {
						t.Error(err)
					}
					return true
				})
				ctx := context.Background()
				if err := d.bindBrowserContext(ctx, "company"); err != nil {
					t.Fatal(err)
				}
				err := d.OpenOwnedTarget(ctx, "")
				allowed := (kind == "page" || kind == "other") && contextID == "company"
				if !allowed {
					if err == nil || d.live() || commandCount(f.recorded(), "Page.navigate") != 0 {
						t.Fatal("unowned type/context reached navigation")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if profile, err := d.inspectOwnedEdgeProfile(ctx, testOwnedEdgeDataDir()); err != nil || profile != "Default" {
					t.Fatalf("owned hidden profile: %q %v", profile, err)
				}
				if closed, err := d.CloseOwnedTarget(ctx); err != nil || !closed {
					t.Fatal("owned hidden target not released")
				}
			})
		}
	}
	if (&directCDPDispatcher{}).ownedTargetTypeMatches("other") {
		t.Fatal("uncreated other target accepted")
	}
}

func TestDirectCDPProfileWaitsForOwnedVersionDocument(t *testing.T) {
	for _, transition := range []string{"", "https://unapproved.example/"} {
		t.Run(transition, func(t *testing.T) {
			base := contextResponder(t, "company", filepath.Join(testOwnedEdgeDataDir(), "Default"))
			transitionSeen := false
			d, f := newDispatcherFixture(t, func(f *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
				if cmd.Method == "Target.getTargetInfo" && !transitionSeen {
					f.mu.Lock()
					navigated := f.currentURL == "edge://version/"
					f.mu.Unlock()
					if navigated {
						transitionSeen = true
						if err := dispatcherReply(c, cmd, map[string]any{"targetInfo": map[string]any{
							"targetId": "owned-target", "type": "other", "url": transition, "browserContextId": "company",
						}}); err != nil {
							t.Error(err)
						}
						return true
					}
				}
				return base(f, c, cmd)
			})
			ctx := context.Background()
			if err := d.bindBrowserContext(ctx, "company"); err != nil {
				t.Fatal(err)
			}
			if err := d.OpenOwnedTarget(ctx, ""); err != nil {
				t.Fatal(err)
			}
			profile, err := d.inspectOwnedEdgeProfile(ctx, testOwnedEdgeDataDir())
			if !transitionSeen {
				t.Fatal("navigation transition not exercised")
			}
			if transition == "" {
				if err != nil || profile != "Default" || commandCount(f.recorded(), "Runtime.evaluate") != 1 {
					t.Fatal("empty URL did not wait for the owned version document", err)
				}
			} else if err == nil || profile != "" || d.live() || commandCount(f.recorded(), "Runtime.evaluate") != 0 {
				t.Fatal("unapproved document evaluated as Profile metadata")
			}
		})
	}
}

func TestDirectCDPNavigationWaitsForCommittedLoader(t *testing.T) {
	navigating, targetPolls, framePolls := false, 0, 0
	d, _ := newDispatcherFixture(t, func(_ *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
		var result any
		switch cmd.Method {
		case "Page.navigate":
			navigating = true
			result = map[string]any{"loaderId": "new-document"}
		case "Target.getTargetInfo":
			u := "about:blank"
			if navigating {
				targetPolls++
				u = "https://example.invalid/final"
				if targetPolls == 1 {
					u = ""
				}
			}
			result = map[string]any{"targetInfo": map[string]any{"targetId": "owned-target", "type": "other", "url": u}}
		case "Page.getFrameTree":
			framePolls++
			loader := "old-document"
			if framePolls > 1 {
				loader = "new-document"
			}
			result = map[string]any{"frameTree": map[string]any{"frame": map[string]any{"loaderId": loader, "url": "https://example.invalid/final"}}}
		default:
			return false
		}
		if err := dispatcherReply(c, cmd, result); err != nil {
			t.Error(err)
		}
		return true
	})
	ctx := context.Background()
	if err := d.OpenOwnedTarget(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := d.Navigate(ctx, "https://example.invalid/start"); err != nil || d.bootstrap || framePolls != 2 || targetPolls != 3 {
		t.Fatal("navigation returned before its committed document", err, targetPolls, framePolls)
	}
}

func TestDirectCDPNavigationReadinessCancellationRetiresTransport(t *testing.T) {
	d, _ := newDispatcherFixture(t, func(_ *dispatcherFixture, c *websocket.Conn, cmd dispatcherCommand) bool {
		if cmd.Method != "Page.navigate" {
			return false
		}
		if err := dispatcherReply(c, cmd, map[string]any{"loaderId": "not-committed"}); err != nil {
			t.Error(err)
		}
		return true
	})
	if err := d.OpenOwnedTarget(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if d.Navigate(ctx, "https://example.invalid/") == nil || d.live() {
		t.Fatal("canceled readiness left an ambiguous transport usable")
	}
}

func TestDirectCDPViewportRequiresOwnedTarget(t *testing.T) {
	d, f := newDispatcherFixture(t, nil)
	if d.setOwnedViewport(context.Background()) == nil || commandCount(f.recorded(), "Emulation.setDeviceMetricsOverride") != 0 {
		t.Fatal("viewport changed without an owned hidden target")
	}
}
