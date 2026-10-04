//go:build windows

package desktopruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

func platformReadLocalCoreAccess(_ context.Context, runtimeRoot string) (LocalCoreAccess, error) {
	manifest, root, err := loadDesktopManifest(runtimeRoot)
	if err != nil {
		return LocalCoreAccess{}, err
	}
	token, err := readProtectedText(filepath.Join(root, "auth-token.dpapi"), "agentdock.startup.v1")
	if errors.Is(err, os.ErrNotExist) {
		token = ""
	} else if err != nil {
		return LocalCoreAccess{}, err
	}
	return LocalCoreAccess{MCPURL: manifest.LocalMCPURL, AuthToken: token}, nil
}
