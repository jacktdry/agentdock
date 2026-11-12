package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPreparedCandidateSnapshotsSourceAndIsSingleUse(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	writeTestPlugin(t, source, "prepared-demo", "1.0.0", false)
	candidate, err := manager.PrepareCandidate(source)
	if err != nil {
		t.Fatal(err)
	}
	review, err := candidate.Review()
	if err != nil || !review.Valid {
		t.Fatalf("review=%#v err=%v", review, err)
	}

	// Mutate the user source after review. Installation must still consume the
	// immutable staged snapshot, not re-read this path.
	if err := os.WriteFile(filepath.Join(source, "plugin.json"), []byte("{\"name\":\"prepared-demo\",\"version\":\"9.9.9\"}"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := manager.InstallPreparedCandidate(context.Background(), candidate, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.FinalizeActivation(result.Name); err != nil {
		t.Fatal(err)
	}
	installed, err := manager.Inspect("prepared-demo")
	if err != nil {
		t.Fatal(err)
	}
	if installed.Version != "1.0.0" || installed.PackageDigest != review.PackageDigest {
		t.Fatalf("installed changed after source mutation: %#v", installed.State)
	}
	if _, err := manager.InstallPreparedCandidate(context.Background(), candidate, false); err == nil {
		t.Fatal("candidate reused after consumption")
	}
}

func TestPreparedCandidateClosePreventsUse(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	writeTestPlugin(t, source, "prepared-close", "1.0.0", false)
	candidate, err := manager.PrepareCandidate(source)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Close()
	if _, err := candidate.Review(); err == nil {
		t.Fatal("closed candidate still reviewable")
	}
	if _, err := manager.InstallPreparedCandidate(context.Background(), candidate, false); err == nil {
		t.Fatal("closed candidate still installable")
	}
}
