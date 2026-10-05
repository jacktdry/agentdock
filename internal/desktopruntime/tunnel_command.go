package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strings"

	"github.com/uvwt/agentdock/internal/desktopcontrol"
)

// TunnelStatus 是桌面端和 CLI 共享的结构化 Tunnel 状态。
type TunnelStatus struct {
	Mode           string             `json:"mode"`
	Running        bool               `json:"running"`
	Ready          bool               `json:"ready"`
	StartupEnabled bool               `json:"startup_enabled"`
	PublicURL      string             `json:"public_url,omitempty"`
	Observation    *TunnelObservation `json:"observation,omitempty"`
}

type TunnelConfigureRequest struct {
	RuntimeRoot string
	Mode        string
	ServerURL   string
	TokenFile   string
}

// RunTunnelCommand serializes public CLI mutations. Read/launch dispatch never
// holds the lock for the lifetime of a supervisor process.
func RunTunnelCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return runTunnelCommand(ctx, args, stdout, stderr, false)
}

// RunTunnelCommandLocked is only for a caller already holding Desktop mutation.
func RunTunnelCommandLocked(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return runTunnelCommand(ctx, args, stdout, stderr, true)
}

func runTunnelCommand(ctx context.Context, args []string, stdout, stderr io.Writer, locked bool) error {
	acquire := func(root string) (func(), error) {
		if locked {
			return func() {}, nil
		}
		next := NextManagedRoot(root)
		var before ConnectionConfig
		if next {
			if err := ValidateNextSettingsIdentity(ctx, root); err != nil {
				return nil, err
			}
			var err error
			before, err = ReadConnectionConfig(ctx, root)
			if err != nil {
				return nil, err
			}
		}
		release, err := AcquireDesktopMutation(ctx, root)
		if err != nil {
			return nil, err
		}
		if next {
			selected, err := SelectNextConnectionRuntime(ctx, root)
			current, configErr := ReadConnectionConfig(ctx, root)
			if err != nil || configErr != nil || current != before {
				release()
				return nil, ErrNextIdentityUnavailable
			}
			if args[0] == "start" || args[0] == "restart" || args[0] == "regenerate" || args[0] == "configure" {
				if _, err := PreflightPortMutation(ctx, PortObservationRequest{Runtime: selected.PortRuntime, Host: selected.BindHost, ConfiguredPort: current.Port, CandidatePort: current.Port}); err != nil {
					release()
					return nil, err
				}
			}
		}
		return release, nil
	}

	if len(args) == 0 {
		return tunnelCommandUsageError()
	}

	switch args[0] {
	case "launch":
		runtimeRoot, err := parseLaunchRuntimeRoot("agentdock tunnel launch", args[1:], stderr)
		if err != nil {
			return err
		}
		return platformLaunchTunnel(ctx, runtimeRoot)
	case "status":
		runtimeRoot, err := parseRuntimeRoot("agentdock tunnel status", args[1:], stderr)
		if err != nil {
			return err
		}
		var status TunnelStatus
		err = desktopcontrol.Call(ctx, runtimeRoot, "tunnel.status", controlActionParams{RuntimeRoot: runtimeRoot}, &status)
		if err != nil {
			status, err = platformTunnelStatus(ctx, runtimeRoot)
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(status)
	case "start", "stop", "restart", "regenerate":
		action := args[0]
		runtimeRoot, err := parseRuntimeRoot("agentdock tunnel "+action, args[1:], stderr)
		if err != nil {
			return err
		}
		release, err := acquire(runtimeRoot)
		if err != nil {
			return err
		}
		defer release()
		// 写操作可能重启当前核心，必须由独立控制进程直接调用系统适配器；
		// IPC 仅用于不会改变服务生命周期的状态读取。
		if err := platformTunnelAction(ctx, runtimeRoot, action); err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(serviceCommandResult{Action: action, Completed: true})
	case "configure":
		flags := flag.NewFlagSet("agentdock tunnel configure", flag.ContinueOnError)
		flags.SetOutput(stderr)
		runtimeRoot := flags.String("runtime-root", "", "AgentDock 桌面运行目录")
		mode := flags.String("mode", "", "Tunnel 模式：none、quick 或 named")
		serverURL := flags.String("server-url", "", "Named Tunnel HTTPS Origin")
		tokenFile := flags.String("token-file", "", "临时 Tunnel Token 文件")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || strings.TrimSpace(*runtimeRoot) == "" {
			return errors.New("用法：agentdock tunnel configure --runtime-root <目录> --mode <none|quick|named> [--server-url <HTTPS Origin>] [--token-file <文件>]")
		}
		normalizedMode := strings.ToLower(strings.TrimSpace(*mode))
		if normalizedMode != "none" && normalizedMode != "quick" && normalizedMode != "named" {
			return errors.New("tunnel configure 的 mode 必须是 none、quick 或 named")
		}
		request := TunnelConfigureRequest{
			RuntimeRoot: *runtimeRoot,
			Mode:        normalizedMode,
			ServerURL:   strings.TrimSpace(*serverURL),
			TokenFile:   strings.TrimSpace(*tokenFile),
		}
		release, err := acquire(request.RuntimeRoot)
		if err != nil {
			return err
		}
		defer release()
		if err := platformConfigureTunnel(ctx, request); err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(serviceCommandResult{Action: "configure", Completed: true})
	case "autostart":
		flags := flag.NewFlagSet("agentdock tunnel autostart", flag.ContinueOnError)
		flags.SetOutput(stderr)
		runtimeRoot := flags.String("runtime-root", "", "AgentDock 桌面运行目录")
		enabled := flags.String("enabled", "", "是否启用：true 或 false")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || strings.TrimSpace(*runtimeRoot) == "" {
			return errors.New("用法：agentdock tunnel autostart --runtime-root <目录> --enabled <true|false>")
		}
		shouldEnable, err := parseCommandBoolean("tunnel autostart", *enabled)
		if err != nil {
			return err
		}
		release, err := acquire(*runtimeRoot)
		if err != nil {
			return err
		}
		defer release()
		if err := platformSetTunnelAutostart(ctx, *runtimeRoot, shouldEnable); err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(serviceCommandResult{Action: "autostart", Completed: true})
	default:
		return tunnelCommandUsageError()
	}
}

func parseLaunchRuntimeRoot(name string, args []string, stderr io.Writer) (string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	runtimeRoot := flags.String("runtime-root", DefaultRuntimeRoot(), "AgentDock 桌面运行目录")
	if err := flags.Parse(args); err != nil {
		return "", err
	}
	if flags.NArg() != 0 || strings.TrimSpace(*runtimeRoot) == "" {
		return "", errors.New("用法：" + name + " --runtime-root <目录>")
	}
	return strings.TrimSpace(*runtimeRoot), nil
}

func parseCommandBoolean(command, value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, errors.New(command + " 的 enabled 必须是 true 或 false")
	}
}

func tunnelCommandUsageError() error {
	return errors.New("用法：agentdock tunnel <launch|status|start|stop|restart|regenerate|configure|autostart> --runtime-root <目录>")
}
