package browser

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
)

const edgeProfileMaxComponents = 32

// Metadata only: no directory enumeration, profile file opens, or contents.
// Keep every ancestor identity to detect observed path replacement as well as
// leaf replacement. This is not an atomic fence or profile authentication.
type edgeProfileComponent struct {
	device, inode uint64
	uid           uint32
	mode          fs.FileMode
}

type edgeProfileMetadata struct {
	count      int
	owner      uint32
	components [edgeProfileMaxComponents + 1]edgeProfileComponent
}

func (m edgeProfileMetadata) valid() bool {
	if m.count < 2 || m.count > len(m.components) {
		return false
	}
	for i, c := range m.components[:m.count] {
		if c.device == 0 || c.inode == 0 || !c.mode.IsDir() || c.mode&fs.ModeSymlink != 0 || c.mode.Perm()&0022 != 0 ||
			(c.uid != 0 && c.uid != m.owner) || (i == m.count-1 && c.uid != m.owner) {
			return false
		}
	}
	return true
}

// lstat is injected so tests never inspect real user profiles. Two bounded
// sweeps detect changes observed within this check; the caller repeats the
// check around all process/signing evidence. No claim of defeating ABA races.
func readEdgeProfileMetadata(ctx context.Context, dir string, owner uint32, lstat func(string) (edgeProfileComponent, error)) (edgeProfileMetadata, error) {
	fail := func() (edgeProfileMetadata, error) { return edgeProfileMetadata{}, errEdgeProcessUnqualified }
	if ctx == nil || ctx.Err() != nil || lstat == nil || len(dir) > 4096 ||
		!filepath.IsAbs(dir) || filepath.Clean(dir) != dir || dir == "/" || strings.ContainsRune(dir, 0) {
		return fail()
	}
	parts := strings.Split(strings.TrimPrefix(dir, "/"), "/")
	if len(parts) > edgeProfileMaxComponents {
		return fail()
	}
	paths := []string{"/"}
	for _, part := range parts {
		paths = append(paths, filepath.Join(paths[len(paths)-1], part))
	}
	m := edgeProfileMetadata{count: len(paths), owner: owner}
	for i, path := range paths {
		if ctx.Err() != nil {
			return fail()
		}
		c, err := lstat(path)
		if err != nil || ctx.Err() != nil {
			return fail()
		}
		m.components[i] = c
		// Reject each unsafe component before following it to a child.
		if c.device == 0 || c.inode == 0 || !c.mode.IsDir() || c.mode&fs.ModeSymlink != 0 || c.mode.Perm()&0022 != 0 ||
			(c.uid != 0 && c.uid != owner) {
			return fail()
		}
	}
	if !m.valid() {
		return fail()
	}
	for i := len(paths) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			return fail()
		}
		c, err := lstat(paths[i])
		if err != nil || c != m.components[i] || ctx.Err() != nil {
			return fail()
		}
	}
	return m, nil
}
