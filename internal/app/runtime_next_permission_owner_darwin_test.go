//go:build darwin

package app

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNextPermissionOwnerExcludesOtherCoreBeforePolicyRecovery(t *testing.T) {
	rt, _, _, _ := nextCodexAuthorizationFixture(t)
	statePath := rt.permissions.StatePath()
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewRuntime(rt.cfg)
	if err == nil {
		_ = other.Close()
		t.Fatal("second Next Core overwrote policy authority")
	}
	after, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected Core changed policy: %v", err)
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	other, err = NewRuntime(rt.cfg)
	if err != nil {
		t.Fatalf("authority leaked after graceful release: %v", err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNextPermissionOwnerLockUnsafeFilesAndParallelContenders(t *testing.T) {
	for _, typ := range []string{"symlink", "permissive", "hardlink", "root-permissive", "root-symlink"} {
		t.Run(typ, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "next")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, nextPermissionOwnerLockName)
			switch typ {
			case "symlink":
				if err := os.Symlink(filepath.Join(parent, "out"), path); err != nil {
					t.Fatal(err)
				}
			case "permissive":
				if err := os.WriteFile(path, []byte("preserve"), 0644); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				original := filepath.Join(parent, "foreign")
				if err := os.WriteFile(original, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(original, path); err != nil {
					t.Fatal(err)
				}
			case "root-permissive":
				if err := os.Chmod(root, 0755); err != nil {
					t.Fatal(err)
				}
			case "root-symlink":
				alias := filepath.Join(parent, "link")
				if err := os.Symlink(root, alias); err != nil {
					t.Fatal(err)
				}
				root = alias
			}
			owner, err := acquireNextPermissionOwner(root)
			if err == nil {
				_ = owner.close()
				t.Fatalf("unsafe %s accepted", typ)
			}
		})
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	winners := make(chan *nextPermissionOwner, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			lease, err := acquireNextPermissionOwner(root)
			if err == nil {
				winners <- lease
			}
		}()
	}
	close(start)
	wg.Wait()
	close(winners)
	count := 0
	for l := range winners {
		count++
		defer l.close()
	}
	if count != 1 {
		t.Fatalf("concurrent permission writers=%d want 1", count)
	}
}
