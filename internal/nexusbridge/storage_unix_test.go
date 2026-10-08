//go:build darwin || linux

package nexusbridge

import (
	"encoding/json"
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

// Rename has committed the replacement even when the directory fsync fails.
// The caller must not interpret this as an ordinary pre-commit save failure.
func TestWriteIdentityPostRenameFsyncFailureIsTruthful(t *testing.T) {
	home := t.TempDir()
	before := Identity{Version: 1, Endpoint: "https://nexus.example", NodeID: "old", DeviceID: "device_old", DeviceToken: "old-secret"}
	after := Identity{Version: 1, Endpoint: "https://nexus.example", NodeID: "new", DeviceID: "device_new", DeviceToken: "new-secret"}
	if err := Save(home, before); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	err = writeIdentityDataWithSync(home, data, func(int) error { return unix.EIO })
	if !errors.Is(err, ErrIdentityCommitUncertain) {
		t.Fatalf("post-rename error did not retain commit-state evidence: %v", err)
	}
	loaded, err := Load(home)
	if err != nil || loaded != after {
		t.Fatalf("rename did not persist the visible replacement: loaded=%+v err=%v", loaded, err)
	}
}
