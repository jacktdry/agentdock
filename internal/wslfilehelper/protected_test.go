package wslfilehelper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestControlProtectionRejectsDrvFSCaseAliasesAndSymlinks(t *testing.T) {
	if !controlPathBlocked([]string{"/mnt/c/Users/Wei/.agentdock-next/permission"}, "/mnt/c/users/wei/.AGENTDOCK-NEXT/permission/state.json", false) {
		t.Fatal("DrvFS alias escaped protection")
	}
	root := t.TempDir()
	protected := filepath.Join(root, "permission")
	if err := os.Mkdir(protected, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(protected, alias); err != nil {
		t.Fatal(err)
	}
	if !controlPathBlocked([]string{protected}, filepath.Join(alias, "state.json"), false) {
		t.Fatal("symlink alias escaped protection")
	}
	if !controlPathBlocked([]string{protected}, root, true) {
		t.Fatal("ancestor deletion escaped protection")
	}
	if controlPathBlocked([]string{protected}, root, false) {
		t.Fatal("ancestor scans must be allowed and skip protected descendants")
	}
	if controlPathWithin("/home/Wei/permission", "/home/wei/permission/state.json") {
		t.Fatal("Linux casing changed")
	}
}
