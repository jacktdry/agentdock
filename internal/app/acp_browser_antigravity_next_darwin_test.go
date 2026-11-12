//go:build darwin

package app

import (
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/config"
)

type fakeOwnerFileInfo struct {
	uid uint32
}

func (f fakeOwnerFileInfo) Name() string       { return "fixture" }
func (f fakeOwnerFileInfo) Size() int64        { return 0 }
func (f fakeOwnerFileInfo) Mode() os.FileMode  { return os.ModeDir | 0o700 }
func (f fakeOwnerFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeOwnerFileInfo) IsDir() bool        { return true }
func (f fakeOwnerFileInfo) Sys() any           { return &syscall.Stat_t{Uid: f.uid} }

func TestNativeStateOwnedByCurrentUserRejectsDifferentOwner(t *testing.T) {
	current := uint32(os.Geteuid())
	if !nativeStateOwnedByCurrentUser(fakeOwnerFileInfo{uid: current}) {
		t.Fatal("current owner rejected")
	}
	foreign := current + 1
	if foreign == current {
		foreign++
	}
	if nativeStateOwnedByCurrentUser(fakeOwnerFileInfo{uid: foreign}) {
		t.Fatal("foreign owner accepted")
	}
}

func TestPrepareACPBrowserEnvironmentNextAntigravity(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	next := filepath.Join(root, ".agentdock-next")
	state := filepath.Join(next, "acp", "antigravity")
	profile := config.ACPProfile{ID: "agy", Command: "antigravity-acp"}
	for _, broker := range []bool{true, false} {
		for _, preconfigured := range []bool{false, true} {
			env := map[string]string{"HOME": root, "KEEP": "value"}
			if preconfigured {
				env["AGY_ACP_STATE_DIR"] = state
				env["AGY_ACP_CHILD_ISOLATION"] = "required"
			}
			got, err := prepareACPBrowserEnvironment(next, profile, env, broker)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"HOME": root, "KEEP": "value", "AGY_ACP_STATE_DIR": state, "AGY_ACP_CHILD_ISOLATION": "required"}
			if broker {
				want["AGENTDOCK_BROWSER_BROKER_REQUIRED"] = "1"
			} else {
				want["AGENTDOCK_BROWSER_BACKENDS_DISABLED"] = "1"
			}
			// Exact equality also forbids auto-approval, direct/stream-json,
			// conversation overrides, hook fixtures and credential injection.
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("env=%v want=%v", got, want)
			}
		}
	}
	for _, dir := range []string{next, filepath.Join(next, "acp"), state} {
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 || !nativeStateOwnedByCurrentUser(info) {
			t.Fatalf("private directory %s: %v %v", dir, info, err)
		}
	}
	for _, key := range []string{"AGY_ACP_STATE_DIR", "AGY_ACP_CHILD_ISOLATION"} {
		for _, value := range []string{"", "unexpected"} {
			if got, err := prepareACPBrowserEnvironment(next, profile, map[string]string{key: value}, true); err == nil || got != nil {
				t.Fatalf("conflict accepted: %s=%q", key, value)
			}
		}
	}
}

func TestPrepareACPBrowserEnvironmentAntigravityOutsideNextUnchanged(t *testing.T) {
	for _, tc := range []struct{ name, marker, dir, profile string }{
		{"stable", "next", ".agentdock", "agy"},
		{"arbitrary", "next", "custom", "agy"},
		{"unmarked", "", ".agentdock-next", "agy"},
		{"stable-marker", "stable", ".agentdock-next", "agy"},
		{"other-profile", "next", ".agentdock-next", "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("AGENTDOCK_DESKTOP_VARIANT", tc.marker)
			dir := filepath.Join(root, tc.dir)
			got, err := prepareACPBrowserEnvironment(dir, config.ACPProfile{ID: tc.profile}, nil, true)
			want := map[string]string{}
			if tc.profile == "agy" {
				want["AGENTDOCK_BROWSER_BROKER_REQUIRED"] = "1"
			}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("env=%v err=%v", got, err)
			}
			if _, err := os.Lstat(dir); !os.IsNotExist(err) {
				t.Fatalf("unqualified directory touched: %v", err)
			}
		})
	}
}

func TestPrepareACPBrowserEnvironmentNextAntigravityRejectsUnsafeDirectories(t *testing.T) {
	for _, component := range []string{".", "acp", "acp/antigravity"} {
		for _, unsafe := range []string{"symlink", "dangling-symlink", "mode", "file"} {
			t.Run(component+"/"+unsafe, func(t *testing.T) {
				root := t.TempDir()
				t.Setenv("HOME", root)
				t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
				next := filepath.Join(root, ".agentdock-next")
				bad := filepath.Join(next, component)
				if err := os.MkdirAll(filepath.Dir(bad), 0o700); err != nil {
					t.Fatal(err)
				}
				var err error
				switch unsafe {
				case "symlink":
					err = os.Symlink(t.TempDir(), bad)
				case "dangling-symlink":
					err = os.Symlink(filepath.Join(root, "missing"), bad)
				case "mode":
					err = os.Mkdir(bad, 0o700)
					if err == nil {
						err = os.Chmod(bad, 0o750)
					}
				case "file":
					err = os.WriteFile(bad, nil, 0o600)
				}
				if err != nil {
					t.Fatal(err)
				}
				before, _ := os.Lstat(bad)
				got, err := prepareACPBrowserEnvironment(next, config.ACPProfile{ID: "antigravity"}, nil, false)
				if err == nil || got != nil {
					t.Fatal("unsafe state allowed a global fallback")
				}
				after, err := os.Lstat(bad)
				if err != nil || before.Mode() != after.Mode() || !os.SameFile(before, after) {
					t.Fatal("unsafe directory repaired or replaced")
				}
			})
		}
	}
}
