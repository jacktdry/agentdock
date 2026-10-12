//go:build windows

package app

import "os"

// The source-only native isolation path is Darwin-only; fail closed here.
func nativeStateOwnedByCurrentUser(os.FileInfo) bool { return false }
func nativeStateSingleLink(os.FileInfo) bool         { return false }
