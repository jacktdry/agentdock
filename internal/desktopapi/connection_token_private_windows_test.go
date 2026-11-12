//go:build windows

package desktopapi

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The Windows token must have a protected DACL containing only explicit
// full-control ACEs for its owner, SYSTEM and local Administrators. A chmod
// approximation would miss inherited broad access to the raw Named token.
func testConnectionTokenPrivate(path string, _ os.FileInfo) bool {
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || descriptor == nil {
		return false
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 || control&windows.SE_DACL_PRESENT == 0 {
		return false
	}
	acl, _, err := descriptor.DACL()
	if err != nil || acl == nil || acl.AceCount == 0 {
		return false
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return false
	}
	allowed := map[string]bool{
		user.User.Sid.String(): true,
		"S-1-5-18":             true, // LocalSystem
		"S-1-5-32-544":         true, // Builtin Administrators
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace == nil ||
			ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE ||
			ace.Mask != 0x001f01ff || // FILE_ALL_ACCESS from the private SDDL "FA" ACE.
			!allowed[(*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()] {
			return false
		}
	}
	return true
}
