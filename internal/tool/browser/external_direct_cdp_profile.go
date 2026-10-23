package browser

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/browserpolicy"
)

// inspectOwnedEdgeProfile reads only Edge's supported version page in this
// dispatcher's newly created hidden target. It never reads Local State,
// cookies, account details, existing targets or Profile files. It returns the
// Profile directory privately, not a login/authentication qualification.
// The caller must close/join the probe before admitting any company action.
func (d *directCDPDispatcher) inspectOwnedEdgeProfile(ctx context.Context, dataRoot string) (string, error) {
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()
	if d.browserContextID == "" || d.sessionID == "" || !d.bootstrap || !d.live() {
		return "", errDirectCDPAction
	}
	if err := d.verifyTargetLocked(ctx); err != nil {
		return "", err
	}
	d.bootstrap = false // An internal probe must never become a company target.
	var result struct {
		ErrorText  string `json:"errorText"`
		IsDownload bool   `json:"isDownload"`
	}
	if err := d.callLocked(ctx, "Page.navigate", d.sessionID, struct {
		URL string `json:"url"`
	}{"edge://version/"}, &result); err != nil || result.ErrorText != "" || result.IsDownload {
		return "", errDirectCDPAction
	}
	for {
		info, err := d.targetInfoLocked(ctx)
		if err != nil || info.TargetID != d.targetID || !d.ownedTargetTypeMatches(info.Type) || !d.targetContextMatchesLocked(ctx, info) ||
			(info.URL != "" && info.URL != "about:blank" && info.URL != "edge://version/") {
			d.Close()
			return "", errDirectCDPRetired
		}
		// A version navigation can briefly have no committed URL. Wait only
		// on our exact owned target/context; evaluate only the final version URL.
		// A version navigation can be between documents. Only this fixed,
		// read-only expression is retried; no company operation is retried.
		if info.URL == "edge://version/" {
			raw, evalErr := d.evaluateLocked(ctx, `document.getElementById('profile_path')?.textContent ?? ''`)
			var path string
			if evalErr == nil && json.Unmarshal(raw, &path) == nil {
				path = strings.TrimSpace(path)
				if path != "" {
					if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Dir(path) != dataRoot || !edgeProfileDirectoryValid(filepath.Base(path)) {
						d.Close()
						return "", errDirectCDPRetired
					}
					return filepath.Base(path), nil
				}
			}
			if !d.live() {
				return "", errDirectCDPRetired
			}
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			d.Close()
			return "", errDirectCDPRetired
		case <-timer.C:
		}
	}
}

func edgeProfileDirectoryValid(name string) bool {
	return browserpolicy.ValidExternalProfileDirectory(name)
}
