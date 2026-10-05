package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/uvwt/agentdock/internal/fs/atomicfile"
)

// TunnelMutationError contains safe outcome codes only. Service diagnostics and
// protected rollback bytes never become operation metadata or CLI errors.
type TunnelMutationError struct{ Phase string }

func (e *TunnelMutationError) Error() string { return e.Phase }
func (e *TunnelMutationError) Is(target error) bool {
	return target == ErrTunnelRecoveryRequired && e.Phase == "recovery_required"
}

type tunnelFileSnapshot struct {
	path   string
	data   []byte
	exists bool
	mode   os.FileMode
}

func captureTunnelFiles(paths ...string) ([]tunnelFileSnapshot, error) {
	var snapshots []tunnelFileSnapshot
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			snapshots = append(snapshots, tunnelFileSnapshot{path: path})
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return nil, errors.New("tunnel_snapshot_unavailable")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, errors.New("tunnel_snapshot_unavailable")
		}
		snapshots = append(snapshots, tunnelFileSnapshot{path: path, data: data, exists: true, mode: info.Mode().Perm()})
	}
	return snapshots, nil
}

func restoreTunnelFiles(snapshots []tunnelFileSnapshot) error {
	var result error
	for _, file := range snapshots {
		if file.exists {
			result = errors.Join(result, atomicfile.Write(file.path, file.data, file.mode))
		} else {
			err := os.Remove(file.path)
			if !errors.Is(err, os.ErrNotExist) {
				result = errors.Join(result, err)
			}
		}
	}
	return result
}

// runTunnelTransactionLocked never acquires the Desktop lock and never waits for
// public readiness. A bounded non-secret marker makes interrupted transactions
// fail closed for manual recovery; rollback material remains private in memory.
func runTunnelTransactionLocked(ctx context.Context, root string, snapshots []tunnelFileSnapshot, apply, restore func(context.Context) error) error {
	if err := validateTunnelStateRoot(root); err != nil {
		return err
	}
	if tunnelRecoveryPending(root) {
		return ErrTunnelRecoveryRequired
	}
	marker := filepath.Join(root, tunnelRecoveryFile)
	data, err := json.Marshal(struct {
		Phase        string `json:"phase"`
		Interruption string `json:"interruption"`
	}{"applying", "recovery_required"})
	if err != nil {
		return &TunnelMutationError{Phase: "failed"}
	}
	if err := atomicfile.Write(marker, data, 0o600); err != nil {
		return &TunnelMutationError{Phase: "failed"}
	}
	if err := apply(ctx); err != nil {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		restoreErr := restoreTunnelFiles(snapshots)
		if restoreErr == nil {
			restoreErr = restore(recovery)
		}
		if restoreErr == nil {
			restoreErr = os.Remove(marker)
		}
		if restoreErr != nil {
			return &TunnelMutationError{Phase: "recovery_required"}
		}
		return &TunnelMutationError{Phase: "rolled_back"}
	}
	if err := os.Remove(marker); err != nil {
		return &TunnelMutationError{Phase: "recovery_required"}
	}
	return nil
}
