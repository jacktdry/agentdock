//go:build !darwin

package desktopruntime

import (
	"context"
	"testing"
)

func TestNextDirectoryRequiresNative(t *testing.T) {
	for _, kind := range []NextDirectoryKind{NextDirectoryLogs, NextDirectoryConfiguration} {
		if InspectNextDirectory(context.Background(), "", kind) != ErrDirectoryRequiresNative || OpenNextDirectory(context.Background(), "", kind) != ErrDirectoryRequiresNative {
			t.Fatal("native capability overstated")
		}
	}
}
