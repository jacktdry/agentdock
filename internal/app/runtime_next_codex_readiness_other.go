//go:build !darwin

package app

// A non-macOS runtime has no supported native Codex operator authority.
func (r *Runtime) NextCodexMCPReadiness() Result {
	return Result{"ok": true, "state": "disabled"}
}
