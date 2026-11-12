//go:build windows

package permission

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows os.FileMode does not represent NTFS ACL privacy. Verify the
// protected, explicit allow-list installed by securepath.EnsurePrivate.
// This is read-only: disk drift must fail closed rather than silently repair.
func verifyPrivateStatePath(path string, _ os.FileInfo, directory bool) error {
	// SDDL FA = FILE_ALL_ACCESS (0x001f01ff); x/sys/windows does not expose this constant.
	const fileAllAccess windows.ACCESS_MASK = 0x001f01ff
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PRESENT == 0 || control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("permission state Windows DACL is absent or not protected: %v", err)
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return fmt.Errorf("permission state Windows DACL is missing: %v", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	expected := map[string]bool{user.User.Sid.String(): false, "S-1-5-18": false, "S-1-5-32-544": false}
	flags := uint8(0)
	if directory {
		flags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil || ace == nil {
			return fmt.Errorf("read permission state Windows ACE: %v", err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != flags || ace.Mask != fileAllAccess {
			return fmt.Errorf("unexpected permission state Windows ACE type, flags or mask")
		}
		// Validate SID bounds inside this ACE before asking Windows to inspect it.
		const sidHeader = 8
		offset := int(unsafe.Offsetof(ace.SidStart))
		length := int(ace.Header.AceSize) - offset
		if length < sidHeader {
			return fmt.Errorf("short permission state Windows ACE")
		}
		data := unsafe.Slice((*byte)(unsafe.Pointer(&ace.SidStart)), length)
		sidLength := sidHeader + int(data[1])*4
		if sidLength > len(data) {
			return fmt.Errorf("invalid permission state Windows ACE SID size")
		}
		sidBytes := append([]byte(nil), data[:sidLength]...)
		sid := (*windows.SID)(unsafe.Pointer(&sidBytes[0]))
		if !sid.IsValid() || sid.Len() != sidLength {
			return fmt.Errorf("invalid permission state Windows SID")
		}
		id := sid.String()
		if _, ok := expected[id]; !ok {
			return fmt.Errorf("unexpected permission state Windows ACE SID")
		}
		expected[id] = true
	}
	for _, found := range expected {
		if !found {
			return fmt.Errorf("permission state Windows ACL missing required principal")
		}
	}
	return nil
}
