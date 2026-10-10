package process

import (
	"context"
)

// ObserveDirectChildBirth validates a direct-child relationship using repeated
// kernel birth/parent observations. It grants NO signal, cleanup, or recovery
// rights: a snapshot cannot pin PID handles or prove detached descendants.
func ObserveDirectChildBirth(ctx context.Context, parentPID, childPID int) (BirthIdentity, error) {
	return verifyDirectChildBirth(ctx, parentPID, childPID, ObserveBirthIdentity)
}

func verifyDirectChildBirth(ctx context.Context, parentPID, childPID int,
	observe func(context.Context, int) (BirthIdentity, error),
) (BirthIdentity, error) {
	if ctx == nil || ctx.Err() != nil || observe == nil || parentPID <= 0 ||
		childPID <= 0 || childPID == parentPID {
		return BirthIdentity{}, ErrIdentityUnavailable
	}
	parent1, err := observe(ctx, parentPID)
	if err != nil {
		return BirthIdentity{}, ErrIdentityUnavailable
	}
	child1, err := observe(ctx, childPID)
	if err != nil {
		return BirthIdentity{}, ErrIdentityUnavailable
	}
	// Read again: refuse changed PID/birth/parent while the check ran.
	parent2, err := observe(ctx, parentPID)
	if err != nil {
		return BirthIdentity{}, ErrIdentityUnavailable
	}
	child2, err := observe(ctx, childPID)
	if err != nil || ctx.Err() != nil {
		return BirthIdentity{}, ErrIdentityUnavailable
	}
	if parent1 != parent2 || child1 != child2 ||
		parent1.PID != parentPID || child1.PID != childPID ||
		child1.ParentPID != parentPID || parent1.StartSec <= 0 ||
		child1.StartSec <= 0 || parent1.StartUsec < 0 || child1.StartUsec < 0 ||
		parent1.StartUsec >= 1000000 || child1.StartUsec >= 1000000 ||
		parent1.EffectiveUID != child1.EffectiveUID ||
		child1.ProcessGroupID != childPID ||
		child1.StartSec < parent1.StartSec ||
		(child1.StartSec == parent1.StartSec && child1.StartUsec < parent1.StartUsec) {
		return BirthIdentity{}, ErrIdentityUnavailable
	}
	return child1, nil
}
