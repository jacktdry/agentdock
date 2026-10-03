package desktopruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/uvwt/agentdock/internal/fs/filelock"
)

const desktopMutationLockName = ".desktop-mutation.lock"

// AcquireDesktopMutation serializes state-changing desktop operations that may
// touch the same runtime files or service lifecycle across processes.
func AcquireDesktopMutation(ctx context.Context, runtimeRoot string) (func(), error) {
	root, err := filepath.Abs(strings.TrimSpace(runtimeRoot))
	if err != nil || root == "" {
		return nil, errors.New("runtime root is invalid")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("runtime root is not a directory")
	}
	return filelock.Acquire(ctx, filepath.Join(root, desktopMutationLockName))
}
