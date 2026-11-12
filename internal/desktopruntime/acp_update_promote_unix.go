//go:build darwin || linux

package desktopruntime

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func platformTrustedAntigravityTarget(runtimeRoot, target string) bool {
	if !NextManagedRoot(runtimeRoot) {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	expectedDir := filepath.Clean(filepath.Join(home, ".agentdock-next", "bin"))
	target = filepath.Clean(target)
	if filepath.Dir(target) != expectedDir || filepath.Base(target) != "antigravity-acp" {
		return false
	}
	uid := uint32(unix.Geteuid())
	for _, path := range []string{home, filepath.Join(home, ".agentdock-next"), expectedDir} {
		var stat unix.Stat_t
		if err := unix.Lstat(path, &stat); err != nil ||
			stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
			stat.Uid != uid ||
			stat.Mode&0o022 != 0 {
			return false
		}
	}
	var targetStat unix.Stat_t
	return unix.Lstat(target, &targetStat) == nil &&
		targetStat.Mode&unix.S_IFMT == unix.S_IFREG &&
		targetStat.Uid == uid &&
		targetStat.Mode&0o022 == 0 &&
		targetStat.Mode&0o111 != 0
}

func platformPromoteACPUpdate(runtimeRoot, target string, data []byte) error {
	if !platformTrustedAntigravityTarget(runtimeRoot, target) {
		return errors.New("ACP adapter target ownership changed")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	homeFD, err := unix.Open(home, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open trusted home directory: %w", err)
	}
	defer unix.Close(homeFD)
	nextFD, err := unix.Openat(homeFD, ".agentdock-next", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open trusted Next directory: %w", err)
	}
	defer unix.Close(nextFD)
	binFD, err := unix.Openat(nextFD, "bin", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open trusted ACP bin directory: %w", err)
	}
	defer unix.Close(binFD)

	var targetStat unix.Stat_t
	if err := unix.Fstatat(binFD, "antigravity-acp", &targetStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("stat trusted ACP target: %w", err)
	}
	if targetStat.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("ACP adapter target is no longer a regular file")
	}

	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Errorf("generate ACP update staging name: %w", err)
	}
	tempName := ".agentdock-acp-update-" + hex.EncodeToString(random[:])
	fd, err := unix.Openat(binFD, tempName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o700)
	if err != nil {
		return fmt.Errorf("create ACP update staging file: %w", err)
	}
	temp := os.NewFile(uintptr(fd), tempName)
	cleanup := true
	defer func() {
		if temp != nil {
			_ = temp.Close()
		}
		if cleanup {
			_ = unix.Unlinkat(binFD, tempName, 0)
		}
	}()
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write ACP update staging file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync ACP update staging file: %w", err)
	}
	if err := temp.Close(); err != nil {
		temp = nil
		return fmt.Errorf("close ACP update staging file: %w", err)
	}
	temp = nil
	if err := unix.Renameat(binFD, tempName, binFD, "antigravity-acp"); err != nil {
		return fmt.Errorf("promote ACP adapter update: %w", err)
	}
	cleanup = false
	if err := unix.Fsync(binFD); err != nil {
		return fmt.Errorf("%w: sync ACP adapter directory: %v", ErrACPUpdateOutcomeUnknown, err)
	}
	return nil
}
