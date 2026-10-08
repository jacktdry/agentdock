//go:build darwin

package desktopruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"os/user"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// ResolveNextAgentDockHome is backend authority, never a renderer path selector.
// AppPaths owns two distinct roots. Neither is selected from process defaults.
func ResolveNextAgentDockHome(ctx context.Context, runtimeRoot string) (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", ErrNextIdentityUnavailable
	}
	return resolveNextNexusHome(ctx, runtimeRoot, u.HomeDir, explicitNextDarwinRuntime)
}

func resolveNextNexusHome(ctx context.Context, root, home string, validate func(string) (unixRuntimeManifest, error)) (string, error) {
	if ctx.Err() != nil || os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" || !filepath.IsAbs(home) || root != filepath.Join(home, "Library", "Application Support", "AgentDock Next") {
		return "", ErrNextIdentityUnavailable
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil || canonical != root {
		return "", ErrNextIdentityUnavailable
	}
	if _, err := validate(root); err != nil {
		return "", ErrNextIdentityUnavailable
	}
	// AppPaths(.next).stateDirectory is fixed by the verified signed product
	// identity. An ambient AGENTDOCK_HOME may belong to the stable launcher;
	// it must neither select a directory nor make the Next identity ambiguous.
	state := filepath.Join(home, ".agentdock-next")
	values, err := nextCoreEnvironment(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", ErrNextIdentityUnavailable
	}
	if value := values["AGENTDOCK_HOME"]; value != "" && value != state {
		return "", ErrNextIdentityUnavailable
	}
	if info, err := os.Lstat(state); err == nil {
		if !info.IsDir() {
			return "", ErrNextIdentityUnavailable
		}
		canonical, err := filepath.EvalSymlinks(state)
		if err != nil || canonical != state {
			return "", ErrNextIdentityUnavailable
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", ErrNextIdentityUnavailable
	}
	return state, nil
}

// ReadNextNexusIdentity reads bounded private bytes without creating directories.
// Descriptor-relative no-follow opens prevent a directory swap from reaching stable.
func ReadNextNexusIdentity(ctx context.Context, runtimeRoot string) ([]byte, error) {
	state, err := ResolveNextAgentDockHome(ctx, runtimeRoot)
	if err != nil {
		return nil, err
	}
	return readNextNexusIdentity(state)
}

func readNextNexusIdentity(state string) ([]byte, error) {
	fd, err := unix.Open(filepath.Dir(state), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrNextIdentityUnavailable
	}
	defer func() {
		if fd >= 0 {
			unix.Close(fd)
		}
	}()
	for _, name := range []string{filepath.Base(state), "nexus", "device.json"} {
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if name != "device.json" {
			flags |= unix.O_DIRECTORY
		}
		next, openErr := unix.Openat(fd, name, flags, 0)
		if openErr != nil {
			if errors.Is(openErr, unix.ENOENT) {
				return nil, os.ErrNotExist
			}
			return nil, errors.New("nexus_identity_invalid")
		}
		unix.Close(fd)
		fd = next
		var stat unix.Stat_t
		if unix.Fstat(fd, &stat) != nil || stat.Uid != uint32(os.Getuid()) || stat.Mode&0022 != 0 || (name == "device.json" && (stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0077 != 0 || stat.Nlink != 1 || stat.Size > 1<<20)) {
			return nil, errors.New("nexus_identity_invalid")
		}
	}
	file := os.NewFile(uintptr(fd), "nexus-identity")
	// Transfer ownership to File before reading; no descriptor survives the call.
	fd = -1
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("nexus_identity_invalid")
	}
	return data, nil
}
