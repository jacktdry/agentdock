//go:build !darwin || !cgo

package mcpowneraudit

import "context"

// No fake kernel evidence on platforms without Darwin libproc support.
type darwinKernelPipeObserver struct{}

func (darwinKernelPipeObserver) pipeSnapshot(context.Context, diagnosticRequest, []transportChild) (pipeEvidence, error) {
	return pipeEvidence{}, errUnavailable
}
