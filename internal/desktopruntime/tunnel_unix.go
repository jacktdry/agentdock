//go:build darwin || linux

package desktopruntime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/uvwt/agentdock/internal/envstore"
	"github.com/uvwt/agentdock/internal/fs/atomicfile"
)

func loadTunnelEnvironment(runtimeRoot string) (unixRuntimeManifest, string, map[string]string, error) {
	manifest, root, err := loadUnixRuntime(runtimeRoot)
	if err != nil {
		return unixRuntimeManifest{}, "", nil, err
	}
	values, err := envstore.ParseFile(manifest.TunnelEnvironment)
	if err != nil {
		return unixRuntimeManifest{}, "", nil, err
	}
	return manifest, root, values, nil
}

func tunnelMode(values map[string]string) string {
	mode := strings.ToLower(strings.TrimSpace(values["AGENTDOCK_TUNNEL_MODE"]))
	if mode != "quick" && mode != "named" {
		return "none"
	}
	return mode
}

func platformTunnelStatus(ctx context.Context, runtimeRoot string) (TunnelStatus, error) {
	manifest, root, values, err := loadTunnelEnvironment(runtimeRoot)
	if errors.Is(err, os.ErrNotExist) {
		status := completeTunnelStatus(TunnelStatus{Mode: "none"}, runtimeRoot, "unavailable")
		status.Observation.Running = nil
		status.Observation.Connected = nil
		status.Observation.Autostart = "unavailable"
		return status, nil
	}
	if err != nil {
		return TunnelStatus{}, err
	}
	mode := tunnelMode(values)
	running := basicUnixServiceRunning(ctx, manifest, true)
	publicURL := ""
	if mode == "quick" {
		data, _ := os.ReadFile(filepath.Join(root, "quick-tunnel-url.txt"))
		publicURL = strings.TrimSpace(string(data))
	} else if mode == "named" {
		_, _, core, coreErr := loadCoreEnvironment(runtimeRoot)
		if coreErr == nil {
			publicURL = strings.TrimSpace(core["AGENTDOCK_SERVER_URL"])
		}
	}
	status := TunnelStatus{
		Mode:           mode,
		Running:        running,
		Ready:          running && (mode == "named" || publicURL != ""),
		StartupEnabled: tunnelServiceEnabled(ctx, manifest),
		PublicURL:      publicURL,
	}
	if mode == "quick" && !running {
		status.PublicURL = ""
		status.Ready = false
	}
	status = completeTunnelStatus(status, root, tunnelTokenStateUnix(root))
	if manifest.ServiceManager == "launchd" || manifest.ServiceManager == "smappservice" {
		registered := tunnelServiceActive(ctx, manifest)
		if registered {
			status.Observation.Registered = &registered
		} else {
			status.Observation.Running = nil
		}
		status.Observation.Autostart = "unavailable"
	} else {
		if !running {
			status.Observation.Running = nil
		}
		status.Observation.Autostart = "unknown"
		if status.StartupEnabled {
			status.Observation.Autostart = "enabled"
		}
	}
	if status.Observation.Running == nil {
		status.Observation.Connected = nil
	}
	return status, nil
}

// Transaction helpers require an outer Desktop mutation owner. They never
// acquire that lock themselves and never wait for public readiness.
func platformTunnelAction(ctx context.Context, runtimeRoot, action string) error {
	manifest, root, values, err := loadTunnelEnvironment(runtimeRoot)
	if err != nil {
		return err
	}
	mode := tunnelMode(values)
	if action != "start" && action != "restart" && action != "stop" && action != "regenerate" {
		return errors.New("unsupported_tunnel_action")
	}
	if mode == "none" && action != "stop" {
		return errors.New("tunnel_mode_none")
	}
	if action == "regenerate" && mode != "quick" {
		return errors.New("quick_only")
	}
	if mode == "named" && action != "stop" {
		_, _, core, err := loadCoreEnvironment(root)
		if err != nil {
			return err
		}
		if err := validateNamedPortUnix(core); err != nil {
			return err
		}
	}
	snapshots, err := captureTunnelFiles(manifest.EnvironmentFile, manifest.TunnelEnvironment)
	if err != nil {
		return err
	}
	wasRunning := basicUnixServiceRunning(ctx, manifest, true)
	if tunnelRecoveryPending(root) {
		return ErrTunnelRecoveryRequired
	}
	if action == "start" && wasRunning {
		return nil
	}
	return runTunnelTransactionLocked(ctx, root, snapshots, func(ctx context.Context) error {
		if _, err := AdvanceTunnelGenerationLocked(root); err != nil {
			return err
		}
		if mode == "quick" {
			if err := invalidateQuickTunnelUnixLocked(root); err != nil {
				return err
			}
		}
		if action == "regenerate" {
			action = "restart"
		}
		return tunnelServiceAction(ctx, manifest, action)
	}, func(ctx context.Context) error {
		if _, err := AdvanceTunnelGenerationLocked(root); err != nil {
			return err
		}
		if mode == "quick" {
			if err := invalidateQuickTunnelUnixLocked(root); err != nil {
				return err
			}
		}
		if wasRunning {
			return tunnelServiceAction(ctx, manifest, "restart")
		}
		if basicUnixServiceRunning(ctx, manifest, true) {
			return tunnelServiceAction(ctx, manifest, "stop")
		}
		return nil
	})
}

func validateNamedPortUnix(core map[string]string) error {
	port, err := strconv.Atoi(core["AGENTDOCK_PORT"])
	if core["AGENTDOCK_PORT"] == "" {
		port = unixDefaultPort()
		err = nil
	}
	// Preserve the legacy default; Next's dedicated route uses its own default.
	expected := unixDefaultPort()
	if expected == 0 || err != nil || port != expected {
		return ErrNamedManualRouteRequired
	}
	return nil
}

func platformConfigureTunnel(ctx context.Context, request TunnelConfigureRequest) error {
	manifest, root, core, err := loadCoreEnvironment(request.RuntimeRoot)
	if err != nil {
		return err
	}
	if request.Mode != "none" && request.Mode != "quick" && request.Mode != "named" {
		return errors.New("invalid_tunnel_mode")
	}
	if request.Mode != "named" && (request.ServerURL != "" || request.TokenFile != "") {
		return errors.New("named_fields_in_other_mode")
	}
	origin, token := "", ""
	if request.Mode == "named" {
		if err := validateNamedPortUnix(core); err != nil {
			return err
		}
		origin, err = normalizeHTTPSOrigin(request.ServerURL)
		if err != nil {
			return err
		}
		token, err = configuredTunnelToken(root, request.TokenFile)
		if err != nil {
			return errors.New("tunnel_token_unavailable")
		}
	}
	oldMode := "none"
	if _, _, values, err := loadTunnelEnvironment(root); err == nil {
		oldMode = tunnelMode(values)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	snapshots, err := captureTunnelFiles(manifest.EnvironmentFile, manifest.TunnelEnvironment, filepath.Join(root, "cloudflare-tunnel-token"))
	if err != nil {
		return err
	}
	coreRunning := basicUnixServiceRunning(ctx, manifest, false)
	tunnelRunning := basicUnixServiceRunning(ctx, manifest, true)
	return runTunnelTransactionLocked(ctx, root, snapshots, func(ctx context.Context) error {
		if _, err := AdvanceTunnelGenerationLocked(root); err != nil {
			return err
		}
		if tunnelRunning && manifest.ServiceManager != "launchd" && manifest.ServiceManager != "smappservice" {
			if err := tunnelServiceAction(ctx, manifest, "stop"); err != nil {
				return err
			}
		}
		if err := removeQuickURL(root); err != nil {
			return err
		}
		values := map[string]string{"AGENTDOCK_TUNNEL_MODE": request.Mode}
		delete(core, "AGENTDOCK_SERVER_URL")
		core["AGENTDOCK_OAUTH_ENABLED"] = tunnelBool(request.Mode == "named")
		if request.Mode == "quick" {
			values["AGENTDOCK_TUNNEL_TARGET"] = strings.TrimSuffix(healthURL(core), "/healthz")
		}
		if request.Mode == "named" {
			if err := atomicfile.Write(filepath.Join(root, "cloudflare-tunnel-token"), []byte(token+"\n"), 0o600); err != nil {
				return errors.New("tunnel_token_write_failed")
			}
			core["AGENTDOCK_SERVER_URL"] = origin
		}
		if err := writeEnvironment(manifest.EnvironmentFile, core); err != nil {
			return err
		}
		if err := writeEnvironment(manifest.TunnelEnvironment, values); err != nil {
			return err
		}
		if coreRunning {
			if err := platformServiceAction(ctx, root, "restart"); err != nil {
				return err
			}
		}
		if tunnelRunning {
			action := "restart"
			if request.Mode == "none" {
				action = "stop"
			}
			return tunnelServiceAction(ctx, manifest, action)
		}
		return nil
	}, func(ctx context.Context) error {
		if _, err := AdvanceTunnelGenerationLocked(root); err != nil {
			return err
		}
		if oldMode == "quick" {
			if err := invalidateQuickTunnelUnixLocked(root); err != nil {
				return err
			}
		}
		if coreRunning {
			if err := platformServiceAction(ctx, root, "restart"); err != nil {
				return err
			}
		}
		if tunnelRunning {
			return tunnelServiceAction(ctx, manifest, "start")
		}
		if basicUnixServiceRunning(ctx, manifest, true) {
			return tunnelServiceAction(ctx, manifest, "stop")
		}
		return nil
	})
}

func removeQuickURL(root string) error {
	err := os.Remove(filepath.Join(root, "quick-tunnel-url.txt"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// InvalidateQuickTunnelLocked is also the port worker's integration hook.
func InvalidateQuickTunnelLocked(root string) error {
	if _, err := AdvanceTunnelGenerationLocked(root); err != nil {
		return err
	}
	return invalidateQuickTunnelUnixLocked(root)
}
func invalidateQuickTunnelUnixLocked(root string) error {
	manifest, _, core, err := loadCoreEnvironment(root)
	if err != nil {
		return err
	}
	if err := removeQuickURL(root); err != nil {
		return err
	}
	delete(core, "AGENTDOCK_SERVER_URL")
	core["AGENTDOCK_OAUTH_ENABLED"] = "false"
	return writeEnvironment(manifest.EnvironmentFile, core)
}

func normalizeHTTPSOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Named Tunnel 公网地址必须是有效的 HTTPS Origin")
	}
	if parsed.Path != "" && parsed.Path != "/" && parsed.Path != "/mcp" {
		return "", errors.New("invalid_named_origin")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	return parsed.String(), nil
}

func configuredTunnelToken(root, tokenFile string) (string, error) {
	path := strings.TrimSpace(tokenFile)
	if path == "" {
		path = filepath.Join(root, "cloudflare-tunnel-token")
	}
	// Explicit replacement never silently falls back to the stored credential.
	return readTunnelTokenUnix(path)
}

func readTunnelTokenUnix(path string) (string, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return "", os.ErrNotExist
		}
		return "", errors.New("tunnel_token_unreadable")
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() > 16*1024+2 {
		return "", errors.New("tunnel_token_unreadable")
	}
	data, err := io.ReadAll(io.LimitReader(file, 16*1024+3))
	if err != nil {
		return "", errors.New("tunnel_token_unreadable")
	}
	token := strings.TrimSpace(string(data))
	if token == "" || len(token) > 16*1024 || strings.ContainsAny(token, "\r\n\x00") {
		return "", errors.New("tunnel_token_unreadable")
	}
	return token, nil
}

func tunnelTokenStateUnix(root string) string {
	_, err := configuredTunnelToken(root, "")
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	if err != nil {
		return "unreadable"
	}
	return "stored"
}

func platformSetTunnelAutostart(ctx context.Context, runtimeRoot string, enabled bool) error {
	manifest, root, err := loadUnixRuntime(runtimeRoot)
	if err != nil {
		return err
	}
	if manifest.ServiceManager == "launchd" || manifest.ServiceManager == "smappservice" {
		return errors.New("tunnel_autostart_native_required")
	}
	old := tunnelServiceEnabled(ctx, manifest)
	return runTunnelTransactionLocked(ctx, root, nil, func(ctx context.Context) error { return tunnelServiceSetEnabled(ctx, manifest, enabled) }, func(ctx context.Context) error { return tunnelServiceSetEnabled(ctx, manifest, old) })
}

func platformLaunchTunnel(ctx context.Context, runtimeRoot string) error {
	if err := platformPrepareLaunchEnvironment("tunnel"); err != nil {
		return err
	}
	generation, err := TunnelGeneration(runtimeRoot)
	if err != nil {
		return err
	}
	manifest, root, values, err := loadTunnelEnvironment(runtimeRoot)
	if err != nil {
		return err
	}
	stdout, stderr := io.Writer(os.Stdout), io.Writer(os.Stderr)
	logs, err := platformOpenTunnelLogs(manifest)
	if err != nil {
		return err
	}
	if logs != nil {
		defer logs.Close()
		stdout, stderr = logs.stdout, logs.stderr
	}
	mode := tunnelMode(values)
	switch mode {
	case "quick":
		target := strings.TrimSpace(values["AGENTDOCK_TUNNEL_TARGET"])
		if target == "" {
			return errors.New("Quick Tunnel 缺少目标地址")
		}
		return runQuickTunnel(ctx, manifest, root, runtimeRoot, target, stdout, generation)
	case "named":
		token, err := configuredTunnelToken(root, "")
		if err != nil {
			return errors.New("tunnel_token_unreadable")
		}
		arguments, err := prepareCloudflaredTunnelArgs(root, "run")
		if err != nil {
			return err
		}
		command := exec.CommandContext(ctx, manifest.CloudflaredBinary, arguments...)
		command.Env = append(os.Environ(), "TUNNEL_TOKEN="+token)
		command.Stdout = &tunnelSafeLogWriter{output: stdout, token: token}
		command.Stderr = &tunnelSafeLogWriter{output: stderr, token: token}
		return command.Run()
	default:
		return errors.New("Tunnel 模式为 none")
	}
}

func runQuickTunnel(ctx context.Context, manifest unixRuntimeManifest, root, runtimeRoot, target string, logOutput io.Writer, capturedGeneration ...string) error {
	generation, err := TunnelGeneration(root)
	if len(capturedGeneration) > 0 {
		generation = capturedGeneration[0]
		err = nil
	}
	if err != nil {
		return err
	}
	arguments, err := prepareCloudflaredTunnelArgs(root, "--url", target)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, manifest.CloudflaredBinary, arguments...)
	reader, writer := io.Pipe()
	command.Stdout = writer
	command.Stderr = writer
	if err := command.Start(); err != nil {
		return err
	}
	wait := make(chan error, 1)
	go func() {
		wait <- command.Wait()
		_ = writer.Close()
	}()

	addressApplied := false
	quickURLParser := quickTunnelLogParser{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintln(logOutput, line)
		if addressApplied {
			continue
		}
		publicURL := quickURLParser.URL(line)
		if publicURL == "" {
			continue
		}
		if err := applyQuickTunnelURLUnixGeneration(ctx, manifest, root, runtimeRoot, publicURL, generation, target); err != nil {
			_ = command.Process.Kill()
			return err
		}
		addressApplied = true
	}
	if err := scanner.Err(); err != nil {
		_ = command.Process.Kill()
		return err
	}
	return <-wait
}

func applyQuickTunnelURLUnix(ctx context.Context, manifest unixRuntimeManifest, root, runtimeRoot, publicURL string) error {
	generation, err := TunnelGeneration(root)
	if err != nil {
		return err
	}
	return applyQuickTunnelURLUnixGeneration(ctx, manifest, root, runtimeRoot, publicURL, generation, "")
}

func applyQuickTunnelURLUnixGeneration(ctx context.Context, manifest unixRuntimeManifest, root, runtimeRoot, publicURL, generation, target string) error {
	release, err := AcquireDesktopMutation(ctx, runtimeRoot)
	if err != nil {
		return err
	}
	defer release()
	if err := checkTunnelGeneration(root, generation); err != nil {
		return err
	}
	if generation != "" {
		_, _, values, err := loadTunnelEnvironment(runtimeRoot)
		if err != nil || tunnelMode(values) != "quick" {
			return ErrStaleTunnelGeneration
		}
		if target != "" && values["AGENTDOCK_TUNNEL_TARGET"] != target {
			return ErrStaleTunnelGeneration
		}
	}
	origin, err := normalizeHTTPSOrigin(publicURL)
	if err != nil {
		return errors.New("invalid_quick_origin")
	}
	publicURL = origin

	_, _, core, err := loadCoreEnvironment(runtimeRoot)
	if err != nil {
		return err
	}
	if generation != "" && target != "" && strings.TrimSuffix(healthURL(core), "/healthz") != target {
		return ErrStaleTunnelGeneration
	}
	core["AGENTDOCK_SERVER_URL"] = publicURL
	core["AGENTDOCK_OAUTH_ENABLED"] = "true"
	if err := writeEnvironment(manifest.EnvironmentFile, core); err != nil {
		return err
	}
	if basicUnixServiceRunning(ctx, manifest, false) {
		if err := platformServiceAction(ctx, runtimeRoot, "restart"); err != nil {
			return err
		}
	}
	return atomicfile.Write(filepath.Join(root, "quick-tunnel-url.txt"), []byte(publicURL+"\n"), 0o600)
}

func tunnelServiceActive(ctx context.Context, manifest unixRuntimeManifest) bool {
	return platformTunnelServiceActive(ctx, manifest)
}

func tunnelServiceEnabled(ctx context.Context, manifest unixRuntimeManifest) bool {
	return platformTunnelServiceEnabled(ctx, manifest)
}

func tunnelServiceAction(ctx context.Context, manifest unixRuntimeManifest, action string) error {
	return platformTunnelServiceAction(ctx, manifest, action)
}

func tunnelServiceSetEnabled(ctx context.Context, manifest unixRuntimeManifest, enabled bool) error {
	return platformTunnelServiceSetEnabled(ctx, manifest, enabled)
}
