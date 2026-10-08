//go:build !darwin

package browser

import "context"

// No process-evidence qualification is implemented on this platform.
// Do not silently fall back to tool CDP discovery or raw health status.
func inspectNativeEdgeProcess(context.Context, string, string) (edgeProcessPreflight, error) {
	return edgeProcessPreflight{}, errEdgeProcessUnqualified
}
