package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

func TestNextProductOverridesAmbientStableBeforeRuntimeResolution(t *testing.T) {
	oldName, oldVariant := productName, desktopVariant
	t.Cleanup(func() { productName, desktopVariant = oldName, oldVariant })
	productName, desktopVariant = "AgentDock Next", "next"
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "stable")
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := configureProduct(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" {
		t.Fatal("ambient stable won")
	}
	if runtime.GOOS == "darwin" {
		root := filepath.Join(home, "Library", "Application Support", "AgentDock Next")
		if desktopruntime.DefaultRuntimeRoot() != root {
			t.Fatal("wrong runtime root")
		}
		if defaultSettingsPath() != filepath.Join(root, "shared-desktop-poc", "preferences.json") {
			t.Fatal("preferences escaped Next")
		}
	}
}

func TestInvalidProductFailsClosed(t *testing.T) {
	oldName, oldVariant := productName, desktopVariant
	t.Cleanup(func() { productName, desktopVariant = oldName, oldVariant })
	for _, pair := range [][2]string{{"AgentDock Next", "stable"}, {"AgentDock Desktop", "next"}, {"AgentDock Desktop", "unknown"}} {
		productName, desktopVariant = pair[0], pair[1]
		if configureProduct() == nil {
			t.Fatalf("accepted %v", pair)
		}
	}
}
