//go:build !darwin

package process

import "context"

func openExitWitness(context.Context, BirthIdentity) (exitWitnessPlatform, error) {
	return nil, ErrExitWitnessUnavailable
}
