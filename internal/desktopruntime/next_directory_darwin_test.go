//go:build darwin

package desktopruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNextDirectoriesReadOnlyAndExplicitAction(t *testing.T) {
	home, root := nexusHomeFixture(t)
	calls := 0
	opener := func(_ context.Context, path string) error {
		calls++
		if path != root && path != filepath.Join(root, "logs") {
			t.Fatal("foreign destination")
		}
		return nil
	}
	run := func(kind NextDirectoryKind, action bool) error {
		return nextDirectoryForHome(context.Background(), root, home, kind, action, explicitNextDarwinRuntime, opener)
	}
	for _, kind := range []NextDirectoryKind{NextDirectoryLogs, NextDirectoryConfiguration} {
		if err := run(kind, false); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatal("capability opened program")
	}
	if _, err := os.Lstat(filepath.Join(root, "logs")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("capability created logs", err)
	}
	if err := run(NextDirectoryConfiguration, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "logs")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("config created logs")
	}
	if err := run(NextDirectoryLogs, true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "logs"))
	if err != nil || info.Mode().Perm() != 0700 || calls != 2 {
		t.Fatal(info, err, calls)
	}
	if _, err := os.Stat(filepath.Join(home, ".agentdock-next")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Nexus state touched")
	}
}

func TestNextDirectoriesFailClosed(t *testing.T) {
	for _, attack := range []string{"stable", "foreign", "missing", "unknown", "absolute", "unsigned", "logs_symlink", "root_symlink", "parent_symlink", "logs_writable", "parent_writable", "acl", "swap"} {
		t.Run(attack, func(t *testing.T) {
			home, root := nexusHomeFixture(t)
			kind := NextDirectoryLogs
			validate := explicitNextDarwinRuntime
			switch attack {
			case "stable":
				t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "stable")
			case "foreign":
				root = filepath.Join(home, "Library/Application Support/AgentDock")
			case "missing":
				os.Remove(filepath.Join(root, "desktop-runtime.json"))
			case "unknown":
				kind = "../configuration"
			case "absolute":
				kind = NextDirectoryKind(root)
			case "unsigned":
				if err := exec.Command("/usr/bin/codesign", "--remove-signature", filepath.Join(home, "AgentDock Next.app/Contents/Helpers/agentdock")).Run(); err != nil {
					t.Fatal(err)
				}
			case "logs_symlink":
				if err := os.Symlink(home, filepath.Join(root, "logs")); err != nil {
					t.Fatal(err)
				}
			case "root_symlink", "parent_symlink":
				target := root
				if attack == "parent_symlink" {
					target = filepath.Dir(root)
				}
				if err := os.Rename(target, target+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target+"-old", target); err != nil {
					t.Fatal(err)
				}
			case "logs_writable":
				if err := os.Mkdir(filepath.Join(root, "logs"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(filepath.Join(root, "logs"), 0777); err != nil {
					t.Fatal(err)
				}
			case "parent_writable":
				if err := os.Chmod(filepath.Dir(root), 0777); err != nil {
					t.Fatal(err)
				}
			case "acl":
				if err := exec.Command("/bin/chmod", "+a", "everyone allow add_file,add_subdirectory", root).Run(); err != nil {
					t.Fatal(err)
				}
			case "swap":
				swaps := 0
				validate = func(string) (unixRuntimeManifest, error) {
					swaps++
					if err := os.Rename(root, root+fmt.Sprint("-old-", swaps)); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(root, 0700); err != nil {
						t.Fatal(err)
					}
					return unixRuntimeManifest{}, nil
				}
			}
			opened := false
			for _, action := range []bool{false, true} {
				err := nextDirectoryForHome(context.Background(), root, home, kind, action, validate, func(context.Context, string) error { opened = true; return nil })
				if err == nil || opened {
					t.Fatal("attack accepted", attack)
				}
			}
		})
	}
}
func TestNextDirectoryOpenerErrorIsSafe(t *testing.T) {
	home, root := nexusHomeFixture(t)
	err := nextDirectoryForHome(context.Background(), root, home, NextDirectoryConfiguration, true, explicitNextDarwinRuntime, func(context.Context, string) error { return errors.New("secret /private/path") })
	if err != errDirectoryUnavailable {
		t.Fatal("unsafe error", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := nextDirectoryForHome(ctx, root, home, NextDirectoryLogs, true, explicitNextDarwinRuntime, func(context.Context, string) error { t.Fatal("canceled open"); return nil }); err == nil {
		t.Fatal("canceled action")
	}
}
func TestDirectoryOwnershipRejectsForeignUID(t *testing.T) {
	stat := unix.Stat_t{Uid: uint32(os.Getuid()) + 1, Mode: unix.S_IFDIR | 0700}
	if safeDirectoryStat(&stat, true) || safeDirectoryStat(&stat, false) {
		t.Fatal("foreign owner")
	}
	stat.Uid = 0
	if safeDirectoryStat(&stat, true) {
		t.Fatal("root-owned target")
	}
	stat.Uid = uint32(os.Getuid())
	stat.Mode = unix.S_IFDIR | 0777
	if safeDirectoryStat(&stat, true) {
		t.Fatal("writable target")
	}
}
