//go:build windows

package desktopruntime

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
)

func platformReadCoreConnection(runtimeRoot string) (CoreConnection, error) {
	manifest, root, err := loadDesktopManifest(runtimeRoot)
	if err != nil {
		return CoreConnection{}, err
	}
	settings, err := loadControlPanelSettings(root, manifest.Port)
	if err != nil {
		return CoreConnection{}, err
	}
	token, err := readProtectedTextWithRetry(filepath.Join(root, "auth-token.dpapi"), "agentdock.startup.v1")
	if err != nil {
		return CoreConnection{}, fmt.Errorf("read protected Core token: %w", err)
	}
	return CoreConnection{
		endpoint:  "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(settings.Port)),
		authToken: strings.TrimSpace(token),
	}, nil
}
