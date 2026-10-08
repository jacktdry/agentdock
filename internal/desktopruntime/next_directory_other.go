//go:build !darwin

package desktopruntime

import "context"

func InspectNextDirectory(context.Context, string, NextDirectoryKind) error {
	return ErrDirectoryRequiresNative
}
func OpenNextDirectory(context.Context, string, NextDirectoryKind) error {
	return ErrDirectoryRequiresNative
}
