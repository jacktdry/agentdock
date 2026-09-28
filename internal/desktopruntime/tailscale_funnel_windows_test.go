//go:build windows

package desktopruntime

import (
	"path/filepath"
	"testing"
)

func TestWindowsTunnelModeAcceptsTailscale(t *testing.T) {
	if mode, err := readTunnelMode(filepath.Join(t.TempDir(), "absent"), "tailscale"); err != nil || mode != "tailscale" {
		t.Fatalf("mode = %q, %v", mode, err)
	}
}
