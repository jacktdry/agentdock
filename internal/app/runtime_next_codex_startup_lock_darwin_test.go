//go:build darwin

package app

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNextCodexStartupLockRejectsUnsafePaths(t *testing.T) {
	for _, name := range []string{"root-symlink", "root-mode", "lock-symlink", "lock-mode", "directory", "hardlink"} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "next")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, nextCodexStartupLockName)
			check := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "root-symlink":
				alias := filepath.Join(parent, "alias")
				check(os.Symlink(root, alias))
				root = alias
			case "root-mode":
				check(os.Chmod(root, 0755))
			case "lock-symlink":
				target := filepath.Join(parent, "foreign")
				check(os.WriteFile(target, []byte("keep"), 0600))
				check(os.Symlink(target, path))
			case "lock-mode":
				check(os.WriteFile(path, []byte("keep"), 0644))
				check(os.Chmod(path, 0644))
			case "directory":
				check(os.Mkdir(path, 0700))
			case "hardlink":
				target := filepath.Join(parent, "foreign")
				check(os.WriteFile(target, []byte("keep"), 0600))
				check(os.Link(target, path))
			}
			lock, err := acquireNextCodexStartupLock(root)
			if lock != nil {
				_ = lock.close()
			}
			if err == nil {
				t.Fatal("unsafe startup path accepted")
			}
			if name == "lock-mode" {
				data, _ := os.ReadFile(path)
				if string(data) != "keep" {
					t.Fatal("foreign lock modified")
				}
			}
		})
	}
}

func TestNextCodexStartupLockConcurrentReservationAndReplacement(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	winners := make(chan *nextCodexStartupLock, 16)
	start := make(chan struct{})
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			lock, err := acquireNextCodexStartupLock(root)
			if err == nil {
				winners <- lock
			}
		}()
	}
	close(start)
	wg.Wait()
	close(winners)
	var owner *nextCodexStartupLock
	count := 0
	for lock := range winners {
		count++
		if owner == nil {
			owner = lock
		} else {
			_ = lock.close()
		}
	}
	if owner != nil {
		defer owner.close()
	}
	if count != 1 {
		t.Fatalf("concurrent owners = %d", count)
	}
	path := filepath.Join(root, nextCodexStartupLockName)
	moved := filepath.Join(root, "held-inode")
	if err := os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("foreign replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if owner.validate() == nil {
		t.Fatal("replaced lock remained authoritative")
	}
	if err := owner.close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "foreign replacement" {
		t.Fatal("Close removed/replaced foreign lock")
	}
	if err := owner.close(); err != nil {
		t.Fatal(err)
	}
	if owner.validate() == nil {
		t.Fatal("closed descriptor remained authoritative")
	}
}

func TestNextCodexConfigDigestBindsAllLaunchInputs(t *testing.T) {
	digest := nextCodexMCPConfigDigest("a", "/wrapper", "/backend", []string{"--token=one"})
	for _, other := range []string{
		nextCodexMCPConfigDigest("b", "/wrapper", "/backend", []string{"--token=one"}),
		nextCodexMCPConfigDigest("a", "/wrapper2", "/backend", []string{"--token=one"}),
		nextCodexMCPConfigDigest("a", "/wrapper", "/backend2", []string{"--token=one"}),
		nextCodexMCPConfigDigest("a", "/wrapper", "/backend", []string{"--token=two"}),
	} {
		if digest == other {
			t.Fatal("launch input omitted from digest")
		}
	}
}

func TestNextCodexStartupCrashFenceNeverAutoReclaimsOrphan(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	lease, err := acquireNextCodexStartupLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.markBootstrapAttempt(); err != nil {
		t.Fatal(err)
	}
	if err := lease.close(); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, nextCodexStartupLockName)
	original, err := os.ReadFile(marker)
	if err != nil || !strings.Contains(string(original), "unresolved-v1") {
		t.Fatalf("unresolved marker missing: %q %v", original, err)
	}
	if other, err := acquireNextCodexStartupLock(root); err == nil {
		_ = other.close()
		t.Fatal("crashed generation was silently reclaimed")
	}
	truncated := original[:3]
	if err := os.WriteFile(marker, truncated, 0600); err != nil {
		t.Fatal(err)
	}
	if other, err := acquireNextCodexStartupLock(root); err == nil {
		_ = other.close()
		t.Fatal("partial crash marker was silently reclaimed")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("foreign crash record was unlinked: ", err)
	}
}
