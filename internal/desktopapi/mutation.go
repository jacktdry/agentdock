package desktopapi

import (
	"context"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

const desktopMutationTimeout = 5 * time.Minute

// acquireRuntimeMutation serializes shared desktop mutations across service
// instances and processes that use this contract. The lock is kept inside the
// selected runtime root so all domains coordinate on the same state boundary.
func acquireRuntimeMutation(ctx context.Context, runtimeRoot string) (func(), error) {
	return desktopruntime.AcquireDesktopMutation(ctx, runtimeRoot)
}

func beginRuntimeMutation(ctx context.Context, runtimeRoot string) (context.Context, func(), error) {
	waitCtx, cancelWait := context.WithTimeout(ctx, desktopMutationTimeout)
	release, err := acquireRuntimeMutation(waitCtx, runtimeRoot)
	cancelWait()
	if err != nil {
		return nil, nil, err
	}
	operationCtx, cancelOperation := context.WithTimeout(ctx, desktopMutationTimeout)
	return operationCtx, func() {
		cancelOperation()
		release()
	}, nil
}
