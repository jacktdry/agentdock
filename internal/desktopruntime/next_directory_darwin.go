//go:build darwin

package desktopruntime

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func InspectNextDirectory(ctx context.Context, root string, kind NextDirectoryKind) error {
	return nextDirectory(ctx, root, kind, false)
}
func OpenNextDirectory(ctx context.Context, root string, kind NextDirectoryKind) error {
	return nextDirectory(ctx, root, kind, true)
}
func nextDirectory(ctx context.Context, root string, kind NextDirectoryKind, action bool) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	u, err := user.Current()
	if err != nil {
		return errDirectoryUnavailable
	}
	return nextDirectoryForHome(ctx, root, u.HomeDir, kind, action, explicitNextDarwinRuntime, func(ctx context.Context, path string) error {
		return exec.CommandContext(ctx, "/usr/bin/open", path).Run()
	})
}

// No Nexus state, environment file, or device identity is read here. The
// existing signed Next runtime authority validates the fixed support root.
func nextDirectoryForHome(ctx context.Context, root, home string, kind NextDirectoryKind, action bool, validate func(string) (unixRuntimeManifest, error), opener func(context.Context, string) error) error {
	if ctx.Err() != nil || (kind != NextDirectoryLogs && kind != NextDirectoryConfiguration) || os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" || !filepath.IsAbs(home) || root != filepath.Join(home, "Library", "Application Support", "AgentDock Next") {
		return errDirectoryUnavailable
	}
	// Pin the entire chain before identity validation or any mutation.
	chain, err := holdDirectoryChain(root)
	if err != nil {
		return errDirectoryUnavailable
	}
	defer func() { closeDirectoryChain(chain) }()
	if _, err := validate(root); err != nil {
		return errDirectoryUnavailable
	}
	if !sameDirectoryChain(root, chain) {
		return errDirectoryUnavailable
	}
	target := root
	if kind == NextDirectoryLogs {
		target = filepath.Join(root, "logs")
		fd, err := unix.Openat(chain[len(chain)-1], "logs", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.ENOENT) {
			if !action {
				return nil
			}
			if ctx.Err() != nil || unix.Mkdirat(chain[len(chain)-1], "logs", 0700) != nil {
				return errDirectoryUnavailable
			}
			fd, err = unix.Openat(chain[len(chain)-1], "logs", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		}
		if err != nil {
			return errDirectoryUnavailable
		}
		chain = append(chain, fd)
		if !safeDirectoryFD(fd, true) {
			return errDirectoryUnavailable
		}
	}
	if !action {
		return nil
	}
	if ctx.Err() != nil || !sameDirectoryChain(target, chain) {
		return errDirectoryUnavailable
	}
	if err := opener(ctx, target); err != nil {
		return errDirectoryUnavailable
	}
	return nil
}

func closeDirectoryChain(chain []int) {
	for _, fd := range chain {
		unix.Close(fd)
	}
}
func holdDirectoryChain(path string) ([]int, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errDirectoryUnavailable
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	chain := []int{fd}
	if !safeDirectoryFD(fd, false) {
		closeDirectoryChain(chain)
		return nil, errDirectoryUnavailable
	}
	for _, name := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		next, err := unix.Openat(chain[len(chain)-1], name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			closeDirectoryChain(chain)
			return nil, errDirectoryUnavailable
		}
		chain = append(chain, next)
		if !safeDirectoryFD(next, false) {
			closeDirectoryChain(chain)
			return nil, errDirectoryUnavailable
		}
	}
	if !safeDirectoryFD(chain[len(chain)-1], true) {
		closeDirectoryChain(chain)
		return nil, errDirectoryUnavailable
	}
	return chain, nil
}
func safeDirectoryFD(fd int, userOwned bool) bool {
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return false
	}
	return directoryACLReadOnly(fd) && safeDirectoryStat(&stat, userOwned)
}
func safeDirectoryStat(stat *unix.Stat_t, userOwned bool) bool {
	uid := uint32(os.Getuid())
	if userOwned && stat.Uid != uid {
		return false
	}
	if stat.Uid != uid && stat.Uid != 0 {
		return false
	}
	// Only root-owned sticky ancestors can have group/world write permission.
	return stat.Mode&0022 == 0 || (!userOwned && stat.Uid == 0 && stat.Mode&unix.S_ISVTX != 0)
}
func sameDirectoryChain(path string, held []int) bool {
	current, err := holdDirectoryChain(path)
	if err != nil {
		return false
	}
	defer closeDirectoryChain(current)
	if len(current) != len(held) {
		return false
	}
	for i, fd := range current {
		var a, b unix.Stat_t
		if unix.Fstat(fd, &a) != nil || unix.Fstat(held[i], &b) != nil || a.Dev != b.Dev || a.Ino != b.Ino {
			return false
		}
	}
	return true
}

// Inspect extended security on the pinned descriptor, not by pathname. Any
// allow ACE is conservatively unavailable; deny-only ACLs (e.g. home delete
// protection) are safe. Unknown/truncated ACL data fails closed.
func directoryACLReadOnly(fd int) bool {
	attrs := unix.Attrlist{Bitmapcount: 5, Commonattr: unix.ATTR_CMN_EXTENDED_SECURITY}
	buf := make([]byte, 8192)
	_, _, errno := unix.Syscall6(unix.SYS_FGETATTRLIST, uintptr(fd), uintptr(unsafe.Pointer(&attrs)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0)
	if errno != 0 {
		return false
	}
	size := int(binary.LittleEndian.Uint32(buf[:4]))
	if size < 12 || size > len(buf) {
		return false
	}
	offset := int(int32(binary.LittleEndian.Uint32(buf[4:8]))) + 4
	length := int(binary.LittleEndian.Uint32(buf[8:12]))
	if length == 0 {
		return true
	}
	if offset < 12 || length < 44 || offset > size-length {
		return false
	}
	security := buf[offset : offset+length]
	if binary.LittleEndian.Uint32(security[:4]) != 0x012cc16d {
		return false
	}
	count := binary.LittleEndian.Uint32(security[36:40])
	if count == 0xffffffff {
		return true
	}
	if count > uint32((length-44)/24) {
		return false
	}
	for i := 0; i < int(count); i++ {
		flags := binary.LittleEndian.Uint32(security[44+i*24+16 : 44+i*24+20])
		if flags&0xf != 2 {
			return false
		}
	}
	return true
}
