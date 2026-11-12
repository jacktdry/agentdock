//go:build windows

package permission

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uvwt/agentdock/internal/fs/securepath"
	"golang.org/x/sys/windows"
)

func TestPermissionWindowsPrivateDACLAndTamperFailClosed(t *testing.T) {
	store := newPermissionStore(t, t.TempDir(), "acl-epoch")
	policy := store.Policy()
	policy.GlobalMode = ReadOnly
	if _, err := store.ReplacePolicy(policy.Revision, policy); err != nil {
		t.Fatalf("replace state before private ACL check: %v", err)
	}
	for _, path := range []string{filepath.Dir(store.StatePath()), store.StatePath()} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := verifyPrivateStatePath(path, info, info.IsDir()); err != nil {
			t.Fatalf("private ACL not installed for %s: %v", path, err)
		}
	}
	unsafeSD, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	unsafeACL, _, err := unsafeSD.DACL()
	if err != nil {
		t.Fatal(err)
	}
	path := store.StatePath()
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, unsafeACL, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = securepath.EnsurePrivate(path) })
	if err := store.VerifyDiskState(); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("unsafe ACL was trusted: %v", err)
	}
	if err := securepath.EnsurePrivate(path); err != nil {
		t.Fatal(err)
	}
	if err := store.VerifyDiskState(); err != nil {
		t.Fatalf("restored ACL rejected: %v", err)
	}
}
