//go:build darwin || linux

package nexusbridge

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const maxIdentityFileBytes = 1 << 20

// preparePairHome is called only for an explicit pairing mutation, while the
// cross-process pairing lock is held. A fresh Next install may have no state
// directory yet; create exactly that final directory before sending the
// one-time code. Never recursively create or follow a substituted parent.
func preparePairHome(agentDockHome string) error {
	fd, err := openPrivateHome(agentDockHome)
	if err == nil {
		return unix.Close(fd)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	home := filepath.Clean(strings.TrimSpace(agentDockHome))
	if !filepath.IsAbs(home) || home == string(filepath.Separator) {
		return errors.New("AgentDock home is invalid")
	}
	parentFD, err := unix.Open(filepath.Dir(home), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return errors.New("AgentDock home parent is unsafe")
	}
	defer unix.Close(parentFD)
	if !secureDirectoryFD(parentFD) {
		return errors.New("AgentDock home parent is unsafe")
	}
	if err := unix.Mkdirat(parentFD, filepath.Base(home), 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
		return errors.New("create AgentDock state directory failed")
	}
	fd, err = unix.Openat(parentFD, filepath.Base(home), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return errors.New("AgentDock home is unsafe")
	}
	if !secureDirectoryFD(fd) {
		unix.Close(fd)
		return errors.New("AgentDock home is unsafe")
	}
	return unix.Close(fd)
}

func readIdentityData(agentDockHome string) ([]byte, error) {
	homeFD, err := openPrivateHome(agentDockHome)
	if err != nil {
		return nil, err
	}
	defer unix.Close(homeFD)
	nexusFD, err := unix.Openat(homeFD, "nexus", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, os.ErrNotExist
		}
		return nil, errors.New("NexusDock identity directory is unsafe")
	}
	defer unix.Close(nexusFD)
	if !secureDirectoryFD(nexusFD) {
		return nil, errors.New("NexusDock identity directory is unsafe")
	}
	fd, err := unix.Openat(nexusFD, "device.json", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, os.ErrNotExist
		}
		return nil, errors.New("NexusDock identity file is unsafe")
	}
	file := os.NewFile(uintptr(fd), "nexus-device")
	if file == nil {
		unix.Close(fd)
		return nil, errors.New("NexusDock identity file is unsafe")
	}
	defer file.Close()
	if !secureIdentityFD(fd) {
		return nil, errors.New("NexusDock identity file is unsafe")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxIdentityFileBytes+1))
	if err != nil || len(data) > maxIdentityFileBytes {
		return nil, errors.New("NexusDock identity file is unsafe")
	}
	return data, nil
}

func writeIdentityData(agentDockHome string, data []byte) error {
	return writeIdentityDataWithSync(agentDockHome, data, unix.Fsync)
}

// writeIdentityDataWithSync permits an isolated failure-injection test for the
// durability boundary after rename, without overriding process-global state.
func writeIdentityDataWithSync(agentDockHome string, data []byte, syncDirectory func(int) error) error {
	if len(data) > maxIdentityFileBytes {
		return errors.New("NexusDock identity file is too large")
	}
	homeFD, err := openPrivateHome(agentDockHome)
	if err != nil {
		return err
	}
	defer unix.Close(homeFD)
	nexusFD, err := openOrCreateNexusDir(homeFD)
	if err != nil {
		return err
	}
	defer unix.Close(nexusFD)

	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return errors.New("create NexusDock identity transaction")
	}
	tmpName := ".device-" + hex.EncodeToString(raw[:]) + ".tmp"
	fd, err := unix.Openat(nexusFD, tmpName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return errors.New("create NexusDock identity transaction")
	}
	tmp := os.NewFile(uintptr(fd), tmpName)
	if tmp == nil {
		unix.Close(fd)
		return errors.New("create NexusDock identity transaction")
	}
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = unix.Unlinkat(nexusFD, tmpName, 0)
		}
	}()
	if !secureIdentityFD(fd) {
		return errors.New("NexusDock identity transaction is unsafe")
	}
	if _, err := tmp.Write(data); err != nil {
		return errors.New("write NexusDock identity transaction")
	}
	if err := tmp.Sync(); err != nil {
		return errors.New("sync NexusDock identity transaction")
	}
	if err := tmp.Close(); err != nil {
		return errors.New("close NexusDock identity transaction")
	}
	if err := unix.Renameat(nexusFD, tmpName, nexusFD, "device.json"); err != nil {
		return errors.New("commit NexusDock identity transaction")
	}
	committed = true
	if err := syncDirectory(nexusFD); err != nil {
		return ErrIdentityCommitUncertain
	}
	return nil
}

func openPrivateHome(agentDockHome string) (int, error) {
	home := filepath.Clean(strings.TrimSpace(agentDockHome))
	if home == "" || !filepath.IsAbs(home) {
		return -1, errors.New("AgentDock home is invalid")
	}
	fd, err := unix.Open(home, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return -1, os.ErrNotExist
		}
		return -1, errors.New("AgentDock home is unsafe")
	}
	if !secureDirectoryFD(fd) {
		unix.Close(fd)
		return -1, errors.New("AgentDock home is unsafe")
	}
	return fd, nil
}

func openOrCreateNexusDir(homeFD int) (int, error) {
	fd, err := unix.Openat(homeFD, "nexus", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		if mkdirErr := unix.Mkdirat(homeFD, "nexus", 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
			return -1, errors.New("create NexusDock identity directory")
		}
		fd, err = unix.Openat(homeFD, "nexus", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	}
	if err != nil || !secureDirectoryFD(fd) {
		if fd >= 0 {
			unix.Close(fd)
		}
		return -1, errors.New("NexusDock identity directory is unsafe")
	}
	return fd, nil
}

func secureDirectoryFD(fd int) bool {
	var stat unix.Stat_t
	return unix.Fstat(fd, &stat) == nil && stat.Uid == uint32(os.Getuid()) && stat.Mode&unix.S_IFMT == unix.S_IFDIR && stat.Mode&0022 == 0
}

func secureIdentityFD(fd int) bool {
	var stat unix.Stat_t
	return unix.Fstat(fd, &stat) == nil && stat.Uid == uint32(os.Getuid()) && stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Mode&0077 == 0 && stat.Nlink == 1 && stat.Size <= maxIdentityFileBytes
}
