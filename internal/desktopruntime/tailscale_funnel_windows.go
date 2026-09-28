//go:build windows

package desktopruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	processcontrol "github.com/uvwt/agentdock/internal/process"
)

const tailscalePIDFile = "tailscale-funnel.pid"

func tailscaleExecutable(configured string) (string, error) {
	pathResult, _ := exec.LookPath("tailscale.exe")
	return firstRegularFile(tailscaleWindowsCandidates(configured, pathResult,
		os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")))
}

func configuredTailscaleExecutable(runtimeRoot string) (string, error) {
	configured, err := configuredTailscalePath(runtimeRoot)
	if err != nil {
		return "", err
	}
	return tailscaleExecutable(configured)
}

func configuredTailscalePath(runtimeRoot string) (string, error) {
	configured := strings.TrimSpace(os.Getenv("AGENTDOCK_TAILSCALE_BIN"))
	if configured == "" {
		var err error
		configured, err = readTrimmedText(filepath.Join(runtimeRoot, "tailscale-bin.txt"))
		if err != nil {
			return "", err
		}
	}
	if configured == "" {
		return tailscaleExecutable("")
	}
	return filepath.Abs(configured)
}

func resolveTailscaleFunnelInfo(ctx context.Context, configured string) (string, string, error) {
	binary, err := tailscaleExecutable(configured)
	if err != nil {
		return "", "", err
	}
	output, err := exec.CommandContext(ctx, binary, "status", "--json").CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("读取 Tailscale 状态失败: %w: %s", err, strings.TrimSpace(string(output)))
	}
	publicURL, err := parseTailscaleFunnelStatus(output)
	return binary, publicURL, err
}

func tailscaleRunning(runtime tunnelRuntime) (bool, error) {
	pid, err := ownedTailscalePID(filepath.Join(runtime.root, tailscalePIDFile), runtime.tailscaleBinary, inspectTailscaleProcess)
	return pid != 0, err
}

func inspectTailscaleProcess(pid uint32) (string, error) {
	path, err := queryProcessPath(pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return "", os.ErrNotExist
	}
	return path, err
}

func stopOwnedTailscaleProcess(ctx context.Context, runtime tunnelRuntime) error {
	return stopOwnedTailscalePID(filepath.Join(runtime.root, tailscalePIDFile), runtime.tailscaleBinary,
		inspectTailscaleProcess, func(pid uint32, binaryPath string) error {
			process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
			if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
				return os.ErrNotExist
			}
			if err != nil {
				return err
			}
			defer windows.CloseHandle(process)
			buffer := make([]uint16, 32768)
			size := uint32(len(buffer))
			if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
				return err
			}
			if !samePath(windows.UTF16ToString(buffer[:size]), binaryPath) {
				return os.ErrNotExist // PID was reused; do not terminate it.
			}
			if err := windows.TerminateProcess(process, 0); err != nil {
				return err
			}
			deadline := time.Now().Add(15 * time.Second)
			for time.Now().Before(deadline) {
				if err := ctx.Err(); err != nil {
					return err
				}
				result, err := windows.WaitForSingleObject(process, 250)
				if err != nil {
					return err
				}
				if result == uint32(windows.WAIT_OBJECT_0) {
					return nil
				}
				if result != uint32(windows.WAIT_TIMEOUT) {
					return fmt.Errorf("等待 Tailscale PID %d 停止返回未知状态: 0x%x", pid, result)
				}
			}
			return fmt.Errorf("Tailscale PID %d 未在 15s 内停止", pid)
		})
}

func runTailscaleOnce(ctx context.Context, runtime tunnelRuntime, guard *tunnelSupervisorGuard, logs *processLogs) error {
	target := fmt.Sprintf("http://127.0.0.1:%d", runtime.settings.Port)
	command := exec.CommandContext(ctx, runtime.tailscaleBinary, tailscaleFunnelArgs(target)...)
	command.Dir = runtime.root
	command.Stdout, command.Stderr = logs.stdout, logs.stderr
	processcontrol.Configure(command)
	if err := command.Start(); err != nil {
		return err
	}
	controller, err := processcontrol.Attach(command)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return err
	}
	defer controller.Close()
	pidPath := filepath.Join(runtime.root, tailscalePIDFile)
	if err := writeRuntimeText(pidPath, strconv.Itoa(command.Process.Pid)); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return err
	}
	defer os.Remove(pidPath)
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	for {
		select {
		case err := <-wait:
			return err
		case <-ctx.Done():
			_ = command.Process.Kill()
			<-wait
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
			stopped, err := guard.stopRequested()
			if err != nil || stopped {
				_ = command.Process.Kill()
				<-wait
				return err
			}
		}
	}
}

func waitTailscaleReady(ctx context.Context, runtime tunnelRuntime, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pid, err := activeTunnelSupervisorPIDForRuntime(runtime.root, runtime.manifest)
		if err != nil {
			return err
		}
		running, err := tailscaleRunning(runtime)
		if err != nil {
			return err
		}
		if pid != 0 && running {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return fmt.Errorf("Tailscale Funnel 未在 %s 内启动: %s", timeout, tunnelLogSummary(runtime.files))
}

func startTailscaleTunnel(ctx context.Context, runtime tunnelRuntime) error {
	binary, err := configuredTailscaleExecutable(runtime.root)
	if err != nil {
		return err
	}
	runtime.tailscaleBinary = binary
	supervisorPID, err := activeTunnelSupervisorPIDForRuntime(runtime.root, runtime.manifest)
	if err != nil {
		return err
	}
	running, err := tailscaleRunning(runtime)
	if err != nil {
		return err
	}
	if supervisorPID != 0 && running {
		return nil
	}
	if supervisorPID != 0 {
		if err := signalTunnelSupervisorStop(runtime.root); err != nil {
			return err
		}
		if err := waitTunnelSupervisorStopped(ctx, runtime.root, 10*time.Second); err != nil {
			return err
		}
	}
	// A foreground child can outlive a crashed supervisor. Reclaim only the
	// recorded PID after validating its executable, before launching another.
	if err := stopOwnedTailscaleProcess(ctx, runtime); err != nil {
		return err
	}
	if err := launchCloudflared(runtime); err != nil {
		return err
	}
	return waitTailscaleReady(ctx, runtime, 20*time.Second)
}
