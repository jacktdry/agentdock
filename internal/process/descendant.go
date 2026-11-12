package process

import (
	"context"
	"errors"
	"sort"
)

var ErrDescendantObservationUnavailable = errors.New("descendant birth observation unavailable")

// MaxObservedDescendants bounds evidence persisted in Next's private marker.
// No result proves a complete tree or confers process cleanup authority.
const MaxObservedDescendants = 24
const maxUIDProcessSample = 8192

// ObserveDescendantBirths is a read-only, best-effort snapshot of currently
// attached descendants, including children in DIFFERENT process groups.
// Reparented / already exited children cannot be rediscovered from PPID alone.
func ObserveDescendantBirths(ctx context.Context, expected BirthIdentity) ([]BirthIdentity, error) {
	if ctx == nil || ctx.Err() != nil || !validLineageBirth(expected) {
		return nil, ErrDescendantObservationUnavailable
	}
	first, err := ObserveBirthIdentity(ctx, expected.PID)
	if err != nil || first != expected {
		return nil, ErrDescendantObservationUnavailable
	}
	table, err := uidProcessBirths(expected.EffectiveUID)
	if err != nil || ctx.Err() != nil || len(table) > maxUIDProcessSample {
		return nil, ErrDescendantObservationUnavailable
	}
	linked, err := boundedDescendantLineage(expected, table)
	if err != nil {
		return nil, ErrDescendantObservationUnavailable
	}
	// Validate the selected PID+birth+PPID identities again against the live
	// kernel. A race in sampling fails closed instead of claiming ownership.
	for _, d := range linked {
		now, err := ObserveBirthIdentity(ctx, d.PID)
		if err != nil || now != d {
			return nil, ErrDescendantObservationUnavailable
		}
	}
	second, err := ObserveBirthIdentity(ctx, expected.PID)
	if err != nil || second != expected {
		return nil, ErrDescendantObservationUnavailable
	}
	return linked, nil
}

func validLineageBirth(i BirthIdentity) bool {
	return i.PID > 0 && i.StartSec > 0 && i.StartUsec >= 0 && i.StartUsec < 1000000 &&
		i.ProcessGroupID > 0
}

func bornNoEarlier(child, parent BirthIdentity) bool {
	return child.StartSec > parent.StartSec ||
		(child.StartSec == parent.StartSec && child.StartUsec >= parent.StartUsec)
}

func boundedDescendantLineage(root BirthIdentity, sample []BirthIdentity) ([]BirthIdentity, error) {
	if !validLineageBirth(root) || len(sample) > maxUIDProcessSample {
		return nil, ErrDescendantObservationUnavailable
	}
	byPID := make(map[int]BirthIdentity, len(sample))
	for _, row := range sample {
		if row.PID <= 0 || row.EffectiveUID != root.EffectiveUID {
			return nil, ErrDescendantObservationUnavailable
		}
		if _, exists := byPID[row.PID]; exists {
			return nil, ErrDescendantObservationUnavailable
		}
		byPID[row.PID] = row
	}
	if got, ok := byPID[root.PID]; !ok || got != root {
		return nil, ErrDescendantObservationUnavailable
	}
	descendants := make([]BirthIdentity, 0)
	for _, candidate := range sample {
		if candidate.PID == root.PID || !validLineageBirth(candidate) {
			continue
		}
		visited := map[int]bool{candidate.PID: true}
		current := candidate
		for steps := 0; steps < len(sample); steps++ {
			parent, ok := byPID[current.ParentPID]
			if !ok || !validLineageBirth(parent) || visited[parent.PID] ||
				!bornNoEarlier(current, parent) {
				break
			}
			visited[parent.PID] = true
			if parent.PID == root.PID {
				descendants = append(descendants, candidate)
				if len(descendants) > MaxObservedDescendants {
					return nil, ErrDescendantObservationUnavailable
				}
				break
			}
			current = parent
		}
	}
	sort.Slice(descendants, func(i, j int) bool { return descendants[i].PID < descendants[j].PID })
	return descendants, nil
}
