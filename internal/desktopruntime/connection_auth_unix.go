//go:build darwin || linux

package desktopruntime

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/uvwt/agentdock/internal/envstore"
)

var errConnectionUnavailable = errors.New("Next connection configuration unavailable")

// Require launch identity before reading any runtime files. Never consult a
// default root or a manifest-provided external environment path here.
func connectionEnvironment(root, name string) (map[string]string, error) {
	if os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" || !filepath.IsAbs(root) {
		return nil, errConnectionUnavailable
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil || canonical != filepath.Clean(root) {
		return nil, errConnectionUnavailable
	}
	manifest, _, err := loadUnixRuntime(root)
	if err != nil || filepath.Clean(manifest.EnvironmentFile) != filepath.Join(root, "agentdock.env") ||
		filepath.Clean(manifest.TunnelEnvironment) != filepath.Join(root, "cloudflared.env") {
		return nil, errConnectionUnavailable
	}
	path := filepath.Join(root, name)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 1<<20 {
		return nil, errConnectionUnavailable
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Getuid()) {
		return nil, errConnectionUnavailable
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errConnectionUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errConnectionUnavailable
	}
	values, err := envstore.Parse(data)
	if err != nil {
		return nil, errConnectionUnavailable
	}
	return values, nil
}

func nextCoreEnvironment(root string) (map[string]string, error) {
	values, err := connectionEnvironment(root, "agentdock.env")
	if err != nil {
		return nil, err
	}
	if values["AGENTDOCK_DESKTOP_VARIANT"] != "next" {
		return nil, errConnectionUnavailable
	}
	return values, nil
}

func platformReadConnectionConfig(ctx context.Context, root string) (ConnectionConfig, error) {
	if ctx.Err() != nil {
		return ConnectionConfig{}, ctx.Err()
	}
	values, err := nextCoreEnvironment(root)
	if err != nil {
		return ConnectionConfig{}, errConnectionUnavailable
	}
	port := 8767
	if raw := values["AGENTDOCK_PORT"]; raw != "" {
		port, err = strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return ConnectionConfig{}, errConnectionUnavailable
		}
	}
	host := strings.TrimSpace(values["AGENTDOCK_HOST"])
	switch host {
	case "", "0.0.0.0", "127.0.0.1", "localhost":
		host = "127.0.0.1"
	case "::", "[::]", "::1", "[::1]":
		host = "::1"
	default:
		return ConnectionConfig{}, errConnectionUnavailable
	}
	tunnel, err := connectionEnvironment(root, "cloudflared.env")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ConnectionConfig{}, errConnectionUnavailable
	}
	mode := strings.ToLower(strings.TrimSpace(tunnel["AGENTDOCK_TUNNEL_MODE"]))
	if mode == "" {
		mode = "none"
	}
	switch mode {
	case "none", "quick", "named":
	default:
		return ConnectionConfig{}, errConnectionUnavailable
	}
	return ConnectionConfig{
		CoreEndpoint: "http://" + net.JoinHostPort(host, strconv.Itoa(port)),
		Port:         port, PublicOrigin: values["AGENTDOCK_SERVER_URL"], Mode: mode,
		OAuthEnabled: strings.EqualFold(values["AGENTDOCK_OAUTH_ENABLED"], "true"),
	}, nil
}

func platformReadOAuthPassword(root string) (string, OAuthPasswordState) {
	values, err := nextCoreEnvironment(root)
	state := passwordAvailability(values, err)
	if state != OAuthPasswordStored {
		return "", state
	}
	return values["AGENTDOCK_OAUTH_PASSWORD"], state
}

func platformReadOAuthPasswordState(root string) OAuthPasswordState {
	values, err := nextCoreEnvironment(root)
	return passwordAvailability(values, err)
}

func passwordAvailability(values map[string]string, err error) OAuthPasswordState {
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return OAuthPasswordMissing
		}
		if os.Getenv("AGENTDOCK_DESKTOP_VARIANT") != "next" {
			return OAuthPasswordUnavailable
		}
		return OAuthPasswordUnreadable
	}
	password := values["AGENTDOCK_OAUTH_PASSWORD"]
	if strings.TrimSpace(password) == "" {
		return OAuthPasswordMissing
	}
	return OAuthPasswordStored
}
