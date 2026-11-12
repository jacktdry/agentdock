package process

import (
	"context"
	"errors"
	"math"
)

var ErrGroupObservationUnavailable = errors.New("process group observation unavailable")

const MaxObservedGroupMembers = 256

// GroupObservation is an advisory, non-atomic kernel sample. Membership can
// change immediately afterward and a process group ID may be recycled.
// Neither an empty group nor a matching member grants cleanup authority.
type GroupObservation struct {
	GroupID int
	Members []BirthIdentity
}

func ObserveGroupBirths(ctx context.Context, groupID int) (GroupObservation, error) {
	if ctx == nil || ctx.Err() != nil || groupID <= 0 || groupID > math.MaxInt32 {
		return GroupObservation{}, ErrGroupObservationUnavailable
	}
	members, err := observeGroupBirths(groupID)
	if err != nil || ctx.Err() != nil {
		return GroupObservation{}, ErrGroupObservationUnavailable
	}
	return validatedGroupBirths(groupID, members)
}

func validatedGroupBirths(groupID int, members []BirthIdentity) (GroupObservation, error) {
	if groupID <= 0 || len(members) > MaxObservedGroupMembers {
		return GroupObservation{}, ErrGroupObservationUnavailable
	}
	seen := make(map[int]bool, len(members))
	out := GroupObservation{GroupID: groupID, Members: make([]BirthIdentity, 0, len(members))}
	for _, m := range members {
		if m.PID <= 0 || m.StartSec <= 0 || m.StartUsec < 0 || m.StartUsec >= 1000000 ||
			m.ProcessGroupID != groupID || seen[m.PID] {
			return GroupObservation{}, ErrGroupObservationUnavailable
		}
		seen[m.PID] = true
		out.Members = append(out.Members, m)
	}
	return out, nil
}
