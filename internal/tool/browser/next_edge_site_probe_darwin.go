//go:build darwin

package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

var nextEdgeProbeOrigins = [...]string{"https://oprueba.com", "https://www.oprueba.com", "https://homiplay.io", "https://www.homiplay.io"}

// Only the exact company worktrees approved for this task. This is a source
// smoke allowlist, not an installed Browser policy or a wildcard parent grant.
func nextEdgeProbeScopeForWorkspace(workspace string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errDirectCDPAction
	}
	canonical, err := browserpolicy.CanonicalWorkspaceRoot(workspace)
	if err != nil || workspace != canonical {
		return nil, errDirectCDPAction
	}
	if _, err := os.Lstat(filepath.Join(canonical, ".git")); err != nil {
		return nil, errDirectCDPAction
	}
	switch canonical {
	case filepath.Join(home, "opruebaCode", "official_site", "oprueba"):
		return append([]string(nil), nextEdgeProbeOrigins[:2]...), nil
	case filepath.Join(home, "opruebaCode", "official_site", "homiplay"), filepath.Join(home, "opruebaCode", "homiplay"):
		return append([]string(nil), nextEdgeProbeOrigins[2:]...), nil
	case filepath.Join(home, "opruebaCode", "official_site", "content-cms"):
		return append([]string(nil), nextEdgeProbeOrigins[:]...), nil
	default:
		return nil, errDirectCDPAction
	}
}

type NextEdgeSiteProbe struct {
	Origin          string `json:"origin"`
	FinalOrigin     string `json:"final_origin"`
	SnapshotBytes   int    `json:"snapshot_bytes"`
	ScreenshotBytes int    `json:"screenshot_bytes"`
	TargetClosed    bool   `json:"target_closed"`
}

// Each public-site probe has its own owned target and exact origin pair, on
// the one previously verified Profile connection. The internal version-page
// target is already closed; it is never repurposed for a company operation.
func (d *directCDPDispatcher) probeApprovedSites(ctx context.Context) ([]NextEdgeSiteProbe, error) {
	var checks []NextEdgeSiteProbe
	for i, origin := range nextEdgeProbeOrigins {
		if !d.originScope.Allows(origin) {
			continue
		}
		scope, _ := browserpolicy.NewOriginScope(nextEdgeProbeOrigins[(i/2)*2 : (i/2)*2+2])
		check, err := d.probeOneApprovedSite(ctx, origin, scope)
		checks = append(checks, check)
		if err != nil {
			return checks, err
		}
	}
	return checks, nil
}

func (d *directCDPDispatcher) probeOneApprovedSite(ctx context.Context, origin string, scope browserpolicy.OriginScope) (check NextEdgeSiteProbe, err error) {
	check.Origin = origin
	target, err := d.forkTarget(scope)
	if err != nil {
		return check, err
	}
	if err = target.OpenOwnedTarget(ctx, ""); err != nil {
		return check, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		closed, closeErr := target.CloseOwnedTarget(cleanup)
		check.TargetClosed = closed
		if closeErr != nil || !closed {
			err = errDirectCDPAction
		}
	}()
	if err = target.Navigate(ctx, origin); err != nil {
		return check, err
	}
	pageCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	check.FinalOrigin, err = target.waitProbeDocument(pageCtx, origin)
	cancel()
	if err != nil {
		return check, err
	}
	snapshot, err := target.Snapshot(ctx, false)
	if err != nil {
		return check, err
	}
	structured, _ := snapshot["structuredContent"].(map[string]any)
	text, _ := structured["message"].(string)
	if text == "" {
		return check, errDirectCDPAction
	}
	check.SnapshotBytes = len(text)
	shot, err := target.Screenshot(ctx, false)
	if err != nil {
		return check, err
	}
	content, _ := shot["content"].([]any)
	if len(content) != 1 {
		return check, errDirectCDPAction
	}
	block, _ := content[0].(map[string]any)
	encoded, _ := block["data"].(string)
	png, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(png) < 8 || string(png[:8]) != "\x89PNG\r\n\x1a\n" {
		return check, errDirectCDPAction
	}
	check.ScreenshotBytes = len(png)
	return check, nil
}

func (d *directCDPDispatcher) waitProbeDocument(ctx context.Context, requested string) (string, error) {
	for {
		raw, err := d.Evaluate(ctx, `({ready:document.readyState,origin:location.origin})`)
		if err != nil {
			return "", err
		}
		var state struct {
			Ready  string `json:"ready"`
			Origin string `json:"origin"`
		}
		if json.Unmarshal(raw, &state) != nil {
			return "", errDirectCDPAction
		}
		u, err := url.Parse(state.Origin)
		if err == nil && (state.Ready == "interactive" || state.Ready == "complete") && d.originScope.Allows(state.Origin) {
			want, _ := url.Parse(requested)
			if strings.TrimPrefix(u.Hostname(), "www.") == strings.TrimPrefix(want.Hostname(), "www.") {
				return state.Origin, nil
			}
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}
