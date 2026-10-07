package envstore

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

const secretCanary = "canary-private-value-for-revision-tests"

func revisionFixture(t *testing.T) (*Store, Scope, string) {
	t.Helper()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{Kind: ScopeMCP, Name: "revision-test"}
	path, err := store.Path(scope)
	if err != nil {
		t.Fatal(err)
	}
	return store, scope, path
}

func takeSnapshot(t *testing.T, store *Store, scope Scope) Snapshot {
	t.Helper()
	snapshot, err := store.Snapshot(scope)
	if err != nil {
		t.Fatal("snapshot failed")
	}
	if !validRevision(snapshot.Revision) {
		t.Fatal("invalid opaque revision")
	}
	return snapshot
}

func assertStable(t *testing.T, store *Store, scope Scope, want Snapshot) {
	t.Helper()
	if got := takeSnapshot(t, store, scope); !reflect.DeepEqual(got, want) {
		t.Fatal("passive snapshot changed")
	}
}

func TestProtectedSnapshotAndRevisionLifecycle(t *testing.T) {
	store, scope, path := revisionFixture(t)
	absent := takeSnapshot(t, store, scope)
	assertStable(t, store, scope, absent)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("snapshot created scope file")
	}
	first, err := store.SetChecked(scope, "TOKEN", secretCanary, absent.Revision)
	if err != nil {
		t.Fatal("checked set failed")
	}
	if first.Revision == absent.Revision {
		t.Fatal("set did not advance revision")
	}
	second, err := store.SetChecked(scope, "EMPTY", "", first.Revision)
	if err != nil {
		t.Fatal("checked set failed")
	}
	if !reflect.DeepEqual(second.Entries, []Entry{{Key: "EMPTY"}, {Key: "TOKEN", Configured: true}}) {
		t.Fatal("snapshot is not sorted protected metadata")
	}
	encoded, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["revision"] == nil || fields["entries"] == nil || strings.Contains(string(encoded), secretCanary) {
		t.Fatal("snapshot leaked or added fields")
	}
	assertStable(t, store, scope, second)
	if err := store.Set(scope, "TOKEN", secretCanary); err != nil {
		t.Fatal("legacy set failed")
	}
	third := takeSnapshot(t, store, scope)
	if third.Revision == second.Revision {
		t.Fatal("legacy set bypassed revision")
	}
	removed, err := store.Unset(scope, "EMPTY")
	if err != nil || !removed {
		t.Fatal("legacy unset failed")
	}
	fourth := takeSnapshot(t, store, scope)
	if fourth.Revision == third.Revision {
		t.Fatal("legacy unset bypassed revision")
	}
	removed, empty, err := store.UnsetChecked(scope, "TOKEN", fourth.Revision)
	if err != nil || !removed || len(empty.Entries) != 0 {
		t.Fatal("last-key unset failed")
	}
	if empty.Revision == absent.Revision || empty.Revision == fourth.Revision {
		t.Fatal("empty state collapsed revision")
	}
	assertStable(t, store, scope, empty)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "# "+revisionMarker+": "+empty.Revision+"\n" {
		t.Fatal("empty scope lost atomic revision header")
	}
	assertMode(t, path, 0o600)
	values, err := store.Load(scope)
	if err != nil || len(values) != 0 {
		t.Fatal("metadata exposed as values")
	}
	entries, err := store.List(scope)
	if err != nil || len(entries) != 0 {
		t.Fatal("metadata exposed as entries")
	}
	purged, err := store.PurgeChecked(scope, empty.Revision)
	if err != nil || purged.Revision == empty.Revision || len(purged.Entries) != 0 {
		t.Fatal("empty purge did not advance revision")
	}
}

func TestLegacyRevisionBootstrapAndUpgrade(t *testing.T) {
	for _, operation := range []string{"set", "unset", "missing-unset"} {
		t.Run(operation, func(t *testing.T) {
			store, scope, path := revisionFixture(t)
			if err := os.WriteFile(path, []byte("TOKEN='"+secretCanary+"'\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			legacy := takeSnapshot(t, store, scope)
			assertStable(t, store, scope, legacy)
			// Holding mtime fixed while changing bytes/size proves the bootstrap does
			// not fingerprint secrets. Restore the fixture before upgrading it.
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("TOKEN=x\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, time.Now(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
			if takeSnapshot(t, store, scope).Revision != legacy.Revision {
				t.Fatal("legacy revision depends on content or size")
			}
			if err := os.WriteFile(path, []byte("TOKEN='"+secretCanary+"'\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			legacy = takeSnapshot(t, store, scope)
			switch operation {
			case "set":
				err = store.Set(scope, "TOKEN", secretCanary)
			case "unset":
				_, err = store.Unset(scope, "TOKEN")
			case "missing-unset":
				_, err = store.Unset(scope, "MISSING")
			}
			if err != nil {
				t.Fatal("legacy upgrade failed")
			}
			upgraded := takeSnapshot(t, store, scope)
			if upgraded.Revision == legacy.Revision {
				t.Fatal("legacy file did not upgrade")
			}
			data, err := os.ReadFile(path)
			if err != nil || !strings.HasPrefix(string(data), "# "+revisionMarker+": "+upgraded.Revision+"\n") {
				t.Fatal("upgrade not persisted with content")
			}
			assertStable(t, store, scope, upgraded)
		})
	}
}

func TestCheckedWritesRejectStaleRevisionWithoutMutation(t *testing.T) {
	store, scope, path := revisionFixture(t)
	base := takeSnapshot(t, store, scope)
	if err := store.Set(scope, "TOKEN", secretCanary); err != nil {
		t.Fatal("set failed")
	}
	current := takeSnapshot(t, store, scope)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{base.Revision, "", secretCanary} {
		for _, operation := range []string{"set", "unset", "purge"} {
			var snapshot Snapshot
			switch operation {
			case "set":
				snapshot, err = store.SetChecked(scope, "TOKEN", "replacement", expected)
			case "unset":
				var removed bool
				removed, snapshot, err = store.UnsetChecked(scope, "TOKEN", expected)
				if removed {
					t.Fatal("conflict reported removal")
				}
			case "purge":
				snapshot, err = store.PurgeChecked(scope, expected)
			}
			if !errors.Is(err, ErrRevisionConflict) {
				t.Fatal("missing stable conflict signal")
			}
			if strings.Contains(err.Error(), secretCanary) {
				t.Fatal("conflict leaked secret")
			}
			if !reflect.DeepEqual(snapshot, current) {
				t.Fatal("conflict did not return authoritative snapshot")
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil || string(after) != string(before) {
				t.Fatal("stale mutation changed file")
			}
		}
	}
	purged, err := store.PurgeChecked(scope, current.Revision)
	if err != nil || purged.Revision == current.Revision || len(purged.Entries) != 0 {
		t.Fatal("purge failed")
	}
	assertStable(t, store, scope, purged)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "# "+revisionMarker+": "+purged.Revision+"\n" {
		t.Fatal("purge retained values or lost revision")
	}
}

func TestIndependentCheckedWritersHaveSingleWinner(t *testing.T) {
	first, scope, _ := revisionFixture(t)
	second, err := New(strings.TrimSuffix(first.Root(), string(os.PathSeparator)+"env"))
	if err != nil {
		t.Fatal(err)
	}
	base := takeSnapshot(t, first, scope)
	assertStable(t, second, scope, base)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, store := range []*Store{first, second} {
		go func(store *Store) {
			<-start
			_, err := store.SetChecked(scope, "TOKEN", secretCanary, base.Revision)
			results <- err
		}(store)
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, ErrRevisionConflict) {
			conflicts++
		} else {
			t.Fatal("concurrent checked writer failed unexpectedly")
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("same-base writers did not have exactly one winner")
	}
	final := takeSnapshot(t, first, scope)
	assertStable(t, second, scope, final)
	values, err := first.Load(scope)
	if err != nil || values["TOKEN"] != secretCanary || len(values) != 1 {
		t.Fatal("winning value lost")
	}
}

func TestRevisionMetadataFailsClosed(t *testing.T) {
	token := strings.Repeat("a", 64)
	for _, header := range []string{
		"# " + revisionMarker + "\n",
		"# " + revisionMarker + ": " + secretCanary + "\n",
		"# " + revisionMarker + ": " + strings.Repeat("A", 64) + "\n",
		"# " + revisionMarker + ": " + token + " trailing\n",
		"# " + revisionMarker + ": " + token + "\n# " + revisionMarker + ": " + token + "\n",
	} {
		store, scope, path := revisionFixture(t)
		data := []byte(header + "TOKEN='" + secretCanary + "'\n")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := store.Snapshot(scope)
		if err == nil || strings.Contains(err.Error(), secretCanary) {
			t.Fatal("unsafe revision metadata error")
		}
		if _, err := store.Load(scope); err == nil {
			t.Fatal("Load accepted malformed revision")
		}
		if _, err := store.List(scope); err == nil {
			t.Fatal("List accepted malformed revision")
		}
		if _, err := Parse(data); err == nil {
			t.Fatal("Parse accepted malformed revision")
		}
		if err := store.Set(scope, "TOKEN", "replacement"); err == nil {
			t.Fatal("legacy writer bypassed malformed revision")
		}
		if _, err := store.PurgeChecked(scope, token); err == nil {
			t.Fatal("purge bypassed malformed revision")
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(data) {
			t.Fatal("malformed file mutated")
		}
	}
}

func TestRevisionLikeCommentsInsideValuesAreNotMetadata(t *testing.T) {
	store, scope, _ := revisionFixture(t)
	value := "first line\n# " + revisionMarker + ": invalid\nlast line"
	if err := store.Set(scope, "TOKEN", value); err != nil {
		t.Fatal("set failed")
	}
	values, err := store.Load(scope)
	if err != nil || values["TOKEN"] != value {
		t.Fatal("quoted value misread as metadata")
	}
}
