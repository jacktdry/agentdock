package desktopruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/uvwt/agentdock/internal/fs/filelock"
)

const desktopMutationLockPrefix = ".agentdock-desktop-mutation-"

// AcquireDesktopMutation serializes state-changing desktop operations across
// service instances/processes. Next uses an identity-scoped sibling lock so a
// swapped runtime-root symlink cannot create anything inside stable AgentDock.
// Stable retains its original runtime-local lock to preserve compatibility
// with already-running desktop clients and pre-existing lock owners.
func AcquireDesktopMutation(ctx context.Context, runtimeRoot string) (func(), error) {
	root, err := filepath.Abs(strings.TrimSpace(runtimeRoot))
	if err != nil || root == "" {
		return nil, errors.New("runtime root is invalid")
	}
	if !NextManagedRoot(root) {
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, errors.New("runtime root is not a directory")
		}
		return filelock.Acquire(ctx, filepath.Join(root, ".desktop-mutation.lock"))
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("runtime root is not a trusted directory")
	}
	return filelock.Acquire(ctx, desktopMutationLockPath(root))
}

func desktopMutationLockPath(root string) string {
	digest := sha256.Sum256([]byte("agentdock-desktop-mutation-lock-v2\x00" + filepath.Clean(root)))
	return filepath.Join(filepath.Dir(root), desktopMutationLockPrefix+hex.EncodeToString(digest[:8])+".lock")
}
