package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDesktopPluginRegistrySnapshotDoesNotCreateRemovalState(t *testing.T) {
	home := t.TempDir()
	manager, err := NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	removals := filepath.Join(home, "state", "plugins", "removals")
	if _, err := os.Stat(removals); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected removal state before snapshot: %v", err)
	}
	if _, err := manager.Store().DesktopRegistrySnapshot(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(removals); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("passive snapshot created removal state: %v", err)
	}
	if _, err := manager.Store().ListRemovalStatuses(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(removals); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("passive removal listing created state: %v", err)
	}
}

func TestDesktopPluginRegistryIncludesPendingActivationWithoutRecovering(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	writeTestPlugin(t, source, "desktop-core", "1.0.0", false)
	_, err = installLocalPluginForTest(manager, context.Background(), source, false)
	if err != nil {
		t.Fatal(err)
	}
	before, err := manager.Store().DesktopRegistrySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	writeTestPlugin(t, source, "desktop-core", "2.0.0", false)
	review := manager.Validate(source)
	_, err = manager.UpdateReviewedSource(context.Background(), source, review.ReviewToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.AbortActivation(context.Background(), "desktop-core") })
	snapshot, err := manager.Store().DesktopRegistrySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision == before.Revision || len(snapshot.Recoveries) != 1 || snapshot.Recoveries[0].State != "activation_pending" || snapshot.States[0].Version != "1.0.0" {
		t.Fatalf("pending state not projected: %#v", snapshot)
	}
	repeated, err := manager.Store().DesktopRegistrySnapshot()
	if err != nil || !reflect.DeepEqual(snapshot, repeated) {
		t.Fatal("passive snapshot changed recovery")
	}
	if !hasPendingPluginActivationForTest(manager, "desktop-core") {
		t.Fatal("snapshot resolved activation")
	}
}

func TestDesktopPluginCheckedLifecycleRejectsBeforeSideEffects(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	writeTestPlugin(t, source, "desktop-core", "1.0.0", false)
	_, err = installLocalPluginForTest(manager, context.Background(), source, false)
	if err != nil {
		t.Fatal(err)
	}
	reject := errors.New("test conflict")
	check := func() error { return reject }
	called := false
	_, err = manager.SetEnabledWithLifecycleChecked(context.Background(), "desktop-core", true, func(State) error { called = true; return nil }, check)
	if !errors.Is(err, reject) || called {
		t.Fatal("checked enable activated before fence")
	}
	_, err = manager.RemoveWithLifecycleChecked(context.Background(), "desktop-core", "keep", RemoveLifecycle{BeforeDelete: func(State) error { called = true; return nil }}, check)
	if !errors.Is(err, reject) || called {
		t.Fatal("checked remove deactivated before fence")
	}
	state, err := manager.Store().Load("desktop-core")
	if err != nil || state.Enabled {
		t.Fatal("fence rejection mutated state")
	}
}
