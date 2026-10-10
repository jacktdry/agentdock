package process

import (
	"context"
	"errors"
)

var ErrExitWitnessUnavailable = errors.New("kernel exit witness unavailable")

// ExitWitness is an advisory kernel registration bound to the expected
// identity at setup; it observes only this process's exit. It cannot survive
// Core failure, control the PID, or attest detached browser descendants.
type ExitWitness struct{ impl exitWitnessPlatform }

func WatchVerifiedExit(ctx context.Context, expected BirthIdentity) (*ExitWitness, error) {
	if ctx == nil || ctx.Err() != nil || expected.PID <= 0 ||
		expected.StartSec <= 0 || expected.StartUsec < 0 || expected.StartUsec >= 1000000 ||
		expected.ProcessGroupID <= 0 {
		return nil, ErrExitWitnessUnavailable
	}
	witness, err := openExitWitness(ctx, expected)
	if err != nil {
		return nil, ErrExitWitnessUnavailable
	}
	return &ExitWitness{impl: witness}, nil
}

func (w *ExitWitness) Wait(ctx context.Context) error {
	if w == nil || w.impl == nil || ctx == nil {
		return ErrExitWitnessUnavailable
	}
	return w.impl.wait(ctx)
}
func (w *ExitWitness) Close() error {
	if w == nil || w.impl == nil {
		return nil
	}
	return w.impl.close()
}

type exitWitnessPlatform interface {
	wait(context.Context) error
	close() error
}
