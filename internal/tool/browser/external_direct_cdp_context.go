package browser

import (
	"context"
	"strings"
)

// bindBrowserContext must run before creating a target. Context IDs are opaque,
// process-incarnation-specific observations; an owner-selected ID is never a
// substitute for signed process, socket, Profile or consent verification.
// A missing/replaced context cannot fall back to Edge's default Profile.
func (d *directCDPDispatcher) bindBrowserContext(ctx context.Context, expected string) error {
	if expected == "" || len(expected) > 128 || strings.TrimSpace(expected) != expected || strings.ContainsAny(expected, "\x00\r\n\t") {
		return errDirectCDPAction
	}
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if d.opened || d.browserContextID != "" {
		return errDirectCDPAction
	}
	var result struct {
		BrowserContextIDs       []string `json:"browserContextIds"`
		DefaultBrowserContextID string   `json:"defaultBrowserContextId"`
	}
	if err := d.callLocked(ctx, "Target.getBrowserContexts", "", struct{}{}, &result); err != nil {
		d.Close()
		return err
	}
	seen := make(map[string]bool)
	for _, id := range result.BrowserContextIDs {
		if id == "" || seen[id] {
			d.Close()
			return errDirectCDPRetired
		}
		seen[id] = true
	}
	if !seen[expected] && result.DefaultBrowserContextID != expected {
		d.Close()
		return errDirectCDPRetired
	}
	d.browserContextID = expected
	d.defaultBrowserContext = result.DefaultBrowserContextID == expected
	return nil
}

// Chromium can omit browserContextId for a default-context target. Such an
// omission is accepted only for an explicitly selected default context whose
// process-local ID is independently re-observed on this same transport. A
// non-default context never gets this compatibility path.
func (d *directCDPDispatcher) targetContextMatchesLocked(ctx context.Context, info directTargetInfo) bool {
	if d.browserContextID == "" {
		return true
	}
	if info.BrowserContextID != "" {
		return info.BrowserContextID == d.browserContextID
	}
	if !d.defaultBrowserContext {
		return false
	}
	var current struct {
		Default string `json:"defaultBrowserContextId"`
	}
	return d.callLocked(ctx, "Target.getBrowserContexts", "", struct{}{}, &current) == nil && current.Default == d.browserContextID
}
