package desktopruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var ErrNextIdentityUnavailable = errors.New("next_identity_unavailable")

// NextConnectionRuntime is backend lifecycle selection, never port-owner discovery.
// Nil Running and unknown Health denote missing observational evidence.
type NextConnectionRuntime struct {
	PortRuntime NextPortRuntime
	BindHost    string
	Running     *bool
	Health      string
}

func SelectNextConnectionRuntime(ctx context.Context, root string) (NextConnectionRuntime, error) {
	return platformSelectNextConnectionRuntime(ctx, root)
}

// NextManagedRoot detects attempted Next selection even when its authority is
// incomplete. Detection never grants authority to perform a service operation.
func NextManagedRoot(root string) bool {
	marker := os.Getenv("AGENTDOCK_DESKTOP_VARIANT")
	if marker != "" && marker != "stable" {
		return true
	}
	if filepath.Base(filepath.Clean(root)) == "AgentDock Next" || filepath.Base(filepath.Clean(root)) == ".agentdock-next" {
		return true
	}
	for _, name := range []string{"agentdock.env", "desktop-runtime.json", "runtime.json"} {
		data, _ := os.ReadFile(filepath.Join(root, name))
		if strings.Contains(string(data), "dev.dropabit.agentdock.next") || strings.Contains(string(data), "AGENTDOCK_DESKTOP_VARIANT=next") || strings.Contains(string(data), "AGENTDOCK_DESKTOP_VARIANT='next'") {
			return true
		}
	}
	return false
}

// ValidateNextSettingsIdentity is read-only and must precede service observations.
func ValidateNextSettingsIdentity(ctx context.Context, root string) error {
	if !NextManagedRoot(root) {
		return nil
	}
	return platformValidateNextSettingsIdentity(ctx, root)
}
