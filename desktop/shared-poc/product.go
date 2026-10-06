package main

import (
	"fmt"
	"os"
)

// Set by the product packager with -ldflags -X. Empty variant preserves the
// developer shell's explicit environment selection.
var productName = "AgentDock Desktop"
var desktopVariant = ""

func configureProduct() error {
	variant := desktopVariant
	if variant == "" {
		variant = os.Getenv("AGENTDOCK_DESKTOP_VARIANT")
	}
	if variant != "" && variant != "stable" && variant != "next" {
		return fmt.Errorf("unknown desktop variant")
	}
	if desktopVariant == "next" && productName != "AgentDock Next" {
		return fmt.Errorf("invalid Next product name")
	}
	if productName == "AgentDock Next" && variant != "next" {
		return fmt.Errorf("Next product requires Next desktop variant")
	}
	return os.Setenv("AGENTDOCK_DESKTOP_VARIANT", variant)
}
