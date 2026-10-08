//go:build darwin

package desktopruntime

import (
	"context"
	"testing"
)

// An arbitrary directory must never be adopted as a Next Core or used as a
// fallback to the stable Core; only a selected signed launchd process qualifies.
func TestReadVerifiedNextBrowserSnapshotRejectsUntrustedRoot(t *testing.T) {
	for _, marker := range []string{"stable", "next"} {
		t.Setenv("AGENTDOCK_DESKTOP_VARIANT", marker)
		got, err := ReadVerifiedNextBrowserSnapshot(context.Background(), t.TempDir())
		if err == nil || got.ObservedAt != "" || got.Leases != 0 {
			t.Fatalf("untrusted root returned %+v, err=%v", got, err)
		}
	}
}

func TestReadVerifiedNextBrowserSnapshotHonorsCancellation(t *testing.T) {
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadVerifiedNextBrowserSnapshot(ctx, t.TempDir()); err != context.Canceled {
		t.Fatalf("canceled request returned %v", err)
	}
}
