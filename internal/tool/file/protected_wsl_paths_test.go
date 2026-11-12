package file

import "testing"

func TestWSLProtectionFoldsDrvFSButNotLinuxPaths(t *testing.T) {
	for _, target := range []string{"/mnt/c/users/wei/.agentdock-next/permission/state.json", "/MNT/C/USERS/WEI/.AGENTDOCK-NEXT/PERMISSION/state.json"} {
		if !wslPathWithin("/mnt/c/Users/Wei/.agentdock-next/permission", target) {
			t.Fatal("alternate-case protected path escaped", target)
		}
	}
	if wslPathWithin("/home/Wei/permission", "/home/wei/permission/state.json") {
		t.Fatal("Linux paths must remain case-sensitive")
	}
	if wslPathWithin("/mnt/c/Users/Wei/permission", "/mnt/c/users/wei/permissions-public/a") {
		t.Fatal("prefix sibling blocked")
	}
	if rel := wslRelativePath("/mnt/c/users/wei", "/mnt/c/Users/Wei/.agentdock-next/permission"); rel != ".agentdock-next/permission" {
		t.Fatal("descendant exclusion lost", rel)
	}
}
