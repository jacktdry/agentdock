package envstore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
)

const revisionMarker = "agentdock-env-revision"

// ErrRevisionConflict means the scope changed since the caller's snapshot.
// Checked mutations return the current protected snapshot on conflict.
var ErrRevisionConflict = errors.New("environment revision conflict")

// Snapshot contains only protected metadata, never saved values.
type Snapshot struct {
	Revision string  `json:"revision"`
	Entries  []Entry `json:"entries"`
}

func protectedSnapshot(revision string, values map[string]string) Snapshot {
	entries := make([]Entry, 0, len(values))
	for key, value := range values {
		entries = append(entries, Entry{Key: key, Configured: value != ""})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return Snapshot{Revision: revision, Entries: entries}
}

// metadataRevision bootstraps passive reads without consulting bytes or size.
// Persisted revisions are independently random; these tokens are never written.
func metadataRevision(state, path string, mtime int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("envstore:%s\x00%s\x00%d", state, path, mtime)))
	return hex.EncodeToString(sum[:])
}

func validRevision(token string) bool {
	if len(token) != 64 {
		return false
	}
	for _, char := range token {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func (s *Store) Snapshot(scope Scope) (Snapshot, error) {
	if err := validateScope(scope); err != nil {
		return Snapshot{}, err
	}
	release, err := s.acquireStoreLock()
	if err != nil {
		return Snapshot{}, err
	}
	defer release()
	_, snapshot, err := s.loadSnapshotLocked(scope)
	return snapshot, err
}

func (s *Store) SetChecked(scope Scope, key, value, expectedRevision string) (Snapshot, error) {
	_, snapshot, err := s.mutate(scope, key, value, "set", &expectedRevision)
	return snapshot, err
}

func (s *Store) UnsetChecked(scope Scope, key, expectedRevision string) (bool, Snapshot, error) {
	return s.mutate(scope, key, "", "unset", &expectedRevision)
}

func (s *Store) PurgeChecked(scope Scope, expectedRevision string) (Snapshot, error) {
	_, snapshot, err := s.mutate(scope, "", "", "purge", &expectedRevision)
	return snapshot, err
}

// All accepted writes (including no-op unsets/purges) advance the revision.
// Read, compare, mutation and atomic replacement share the cross-Store lock.
func (s *Store) mutate(scope Scope, key, value, operation string, expected *string) (bool, Snapshot, error) {
	if err := validateScope(scope); err != nil {
		return false, Snapshot{}, err
	}
	if operation != "purge" {
		if err := ValidateKey(key); err != nil {
			return false, Snapshot{}, err
		}
	}
	release, err := s.acquireStoreLock()
	if err != nil {
		return false, Snapshot{}, err
	}
	defer release()
	values, snapshot, err := s.loadSnapshotLocked(scope)
	if err != nil {
		return false, Snapshot{}, err
	}
	if expected != nil && *expected != snapshot.Revision {
		return false, snapshot, ErrRevisionConflict
	}
	removed := false
	switch operation {
	case "set":
		values[key] = value
	case "unset":
		_, removed = values[key]
		delete(values, key)
	case "purge":
		values = map[string]string{}
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return false, Snapshot{}, errors.New("generate environment revision failed")
	}
	revision := hex.EncodeToString(token[:])
	if err := s.writeLocked(scope, values, revision); err != nil {
		return false, Snapshot{}, err
	}
	return removed, protectedSnapshot(revision, values), nil
}
