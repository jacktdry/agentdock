//go:build darwin

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/uvwt/agentdock/internal/browserpolicy"
	"golang.org/x/sys/unix"
)

// nextBrowserRouteFile is a *read-only* config admission path, not proof of
// CDP health, login or user consent. Stable never invokes it. Config must be
// created explicitly by the owner; the launcher must not synthesize a route.
func nextBrowserRouteFile() (browserpolicy.CatalogDefinition, []browserpolicy.WorkspaceRootPolicy, bool, error) {
	var empty browserpolicy.CatalogDefinition
	home, err := os.UserHomeDir()
	if err != nil {
		return empty, nil, false, fmt.Errorf("Next Browser routes: resolve user home: %w", err)
	}
	root := filepath.Join(home, ".agentdock-next")
	if configured := strings.TrimSpace(os.Getenv("AGENTDOCK_HOME")); configured != "" && configured != root {
		// Custom test/service homes do not implicitly inherit the live owner's
		// browser authority. No attempt is made to inspect that home.
		return empty, nil, false, nil
	}
	// Directory fd + openat prevent a symlink replacing the private config
	// filename between inspection and open. Neither path is created or chmod'd.
	dirfd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return empty, nil, false, nil
	}
	if err != nil {
		return empty, nil, false, fmt.Errorf("Next Browser routes: unsafe private home: %w", err)
	}
	defer unix.Close(dirfd)
	var directory unix.Stat_t
	if err := unix.Fstat(dirfd, &directory); err != nil {
		return empty, nil, false, fmt.Errorf("Next Browser routes: stat private home: %w", err)
	}
	if directory.Uid != uint32(os.Geteuid()) || directory.Mode&0o777 != 0o700 {
		return empty, nil, false, fmt.Errorf("Next Browser routes: private home must be owner-only (0700)")
	}
	const filename = "browser-routes.json"
	fd, err := unix.Openat(dirfd, filename, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return empty, nil, false, nil
	}
	if err != nil {
		return empty, nil, false, fmt.Errorf("Next Browser routes: unsafe route config: %w", err)
	}
	file := os.NewFile(uintptr(fd), filename)
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return empty, nil, false, fmt.Errorf("Next Browser routes: stat route config: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o777 != 0o600 {
		return empty, nil, false, fmt.Errorf("Next Browser routes: config must be an owner-only regular file (0600)")
	}
	const maxBytes = 128 * 1024
	if stat.Size > maxBytes {
		return empty, nil, false, fmt.Errorf("Next Browser routes: config exceeds size limit")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || len(body) > maxBytes {
		return empty, nil, false, fmt.Errorf("Next Browser routes: read bounded config: %v", err)
	}
	var saved struct {
		Version           int                                 `json:"version"`
		Catalog           browserpolicy.CatalogDefinition     `json:"catalog"`
		WorkspacePolicies []browserpolicy.WorkspaceRootPolicy `json:"workspace_policies"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&saved); err != nil {
		return empty, nil, false, fmt.Errorf("Next Browser routes: invalid config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return empty, nil, false, fmt.Errorf("Next Browser routes: trailing data")
	}
	if saved.Version != 1 || len(saved.Catalog.Connectors) == 0 || len(saved.WorkspacePolicies) == 0 {
		return empty, nil, false, fmt.Errorf("Next Browser routes: require version 1, connector and explicit workspace policy")
	}
	return saved.Catalog, saved.WorkspacePolicies, true, nil
}
