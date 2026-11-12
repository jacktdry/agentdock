//go:build darwin && !cgo

package browser

import "context"

func macEdgeExecutablePath(context.Context, int) (string, error) {
	return "", errEdgeProcessUnqualified
}

// No substitute for dynamic Security.framework verification is permitted.
func (macEdgeProcessProbe) liveSignature(context.Context, int) error {
	return errEdgeProcessUnqualified
}
