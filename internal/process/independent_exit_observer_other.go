//go:build !darwin

package process

import "context"

// No independent process witness prototype is supported on Linux/Windows.
// A failed admission cannot register or observe any PID.
func WatchIndependentWorkerExit(context.Context, IndependentExitRequest, func() error) error {
	return ErrIndependentExitObserverUnavailable
}
