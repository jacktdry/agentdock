//go:build !darwin

package app

import (
	"context"
	"errors"
	processcontrol "github.com/uvwt/agentdock/internal/process"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
)

var errSharedBrowserEvidence = errors.New("Next shared browser worker evidence unavailable")

type sharedBrowserOrphanInspection struct {
	Status             string
	WorkerObservations int
}

func (*sharedBrowserRestartGuard) recordManagedWorker(toolbrowser.WorkerInfo) error {
	return errSharedBrowserEvidence
}
func inspectSharedBrowserOrphanEvidence(context.Context, string, func(context.Context, int) processcontrol.Snapshot) sharedBrowserOrphanInspection {
	return sharedBrowserOrphanInspection{Status: "observation_unavailable"}
}
