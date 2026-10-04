// Package updateidentity defines the closed macOS updater target contract.
package updateidentity

import (
	"fmt"
	"path/filepath"
)

type Identity struct {
	Variant  string
	Name     string
	BundleID string
	Artifact string
}

// Resolve permits the historical empty marker only for stable callers. Bundle
// validation must still prove that an unmarked caller is operating on stable.
func Resolve(variant string) (Identity, error) {
	switch variant {
	case "", "stable":
		return Identity{"stable", "AgentDock", "com.uvwt.agentdock", "AgentDock-macos-universal.zip"}, nil
	case "next":
		return Identity{"next", "AgentDock Next", "dev.dropabit.agentdock.next", "AgentDock-Next-macos-universal.zip"}, nil
	default:
		return Identity{}, fmt.Errorf("unknown update identity %q", variant)
	}
}

func (id Identity) AppName() string { return id.Name + ".app" }
func (id Identity) Root(home string) string {
	return filepath.Join(home, "Library", "Application Support", id.Name)
}
func (id Identity) Label(service string) string { return id.BundleID + "." + service }

func (id Identity) ValidateMetadata(variant, bundleID, name, displayName string) error {
	if bundleID != id.BundleID || (variant != id.Variant && !(id.Variant == "stable" && variant == "")) {
		return fmt.Errorf("bundle does not match explicit %s identity", id.Variant)
	}
	if id.Variant == "next" && (name != id.Name || displayName != id.Name) {
		return fmt.Errorf("Next bundle names do not match identity")
	}
	return nil
}

// ValidateDestination is lexical and runs before any filesystem inspection.
func (id Identity) ValidateDestination(path, home string) error {
	if !filepath.IsAbs(path) || path != filepath.Clean(path) || filepath.Base(path) != id.AppName() {
		return fmt.Errorf("invalid %s App destination %q", id.Variant, path)
	}
	if id.Variant == "next" && path != filepath.Join("/Applications", id.AppName()) && path != filepath.Join(home, "Applications", id.AppName()) {
		return fmt.Errorf("Next destination is outside its installation namespace")
	}
	return nil
}
