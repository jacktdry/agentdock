package process

import (
	"context"
	"errors"
)

var ErrIdentityUnavailable = errors.New("process birth identity unavailable")

// BirthIdentity is a read-only PID + OS observed start timestamp/credentials.
// Snapshot identity does not pin a process handle, attest child ownership,
// authorize signals or establish that detached browser descendants have exited.
type BirthIdentity struct {
	PID int
	// ParentPID is a kernel-observed relation, not a durable ownership right.
	ParentPID      int
	StartSec       int64
	StartUsec      int64
	EffectiveUID   uint32
	ProcessGroupID int
}

// ObserveBirthIdentity is intentionally observation-only, never process control.
func ObserveBirthIdentity(ctx context.Context, pid int) (BirthIdentity, error) {
	if ctx == nil || ctx.Err() != nil || pid <= 0 {
		return BirthIdentity{}, ErrIdentityUnavailable
	}
	id, err := observeBirthIdentity(pid)
	if err != nil || ctx.Err() != nil || id.PID != pid || id.StartSec <= 0 ||
		id.StartUsec < 0 || id.StartUsec >= 1000000 || id.ProcessGroupID <= 0 {
		return BirthIdentity{}, ErrIdentityUnavailable
	}
	return id, nil
}
