package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsTailscaleControlPanelContract(t *testing.T) {
	root := filepath.Join("..", "..", "desktop", "windows", "control-panel")
	checks := map[string][]string{
		"MainWindow.xaml":                                  {`x:Name="TailscaleModeRadio"`, `Tag="tailscale"`, `TailscaleFunnelDescription`},
		"MainWindow.xaml.cs":                               {`TailscaleModeRadio.IsChecked`, `return "tailscale"`, `NamedTunnelGroup.Visibility = TailscaleModeRadio.IsChecked == true ? Visibility.Collapsed`, `mode == "named" ? TunnelTokenPasswordBox.Password : ""`},
		filepath.Join("Services", "RuntimeService.cs"):     {`IsOwnedTailscaleProcessRunning(RuntimeRoot)`, `tailscale-funnel.pid`, `tailscale-bin.txt`},
		filepath.Join("Resources", "UiStrings.resx"):       {`name="TailscaleFunnel"`, `name="TailscaleFunnelRunning"`},
		filepath.Join("Resources", "UiStrings.zh-CN.resx"): {`name="TailscaleFunnel"`, `name="TailscaleFunnelRunning"`},
	}
	for file, wants := range checks {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s missing %q", file, want)
			}
		}
	}
}

func TestWindowsTailscaleUsesAgentDockSupervisorContract(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "desktopruntime")
	checks := map[string][]string{
		"tunnel_windows.go":           {`if runtime.mode == "tailscale" {`, `runTailscaleOnce(ctx, runtime, guard, logs)`, `return startTailscaleTunnel(ctx, runtime)`, `waitTunnelSupervisorStopped(ctx, runtime.root, 15*time.Second)`, `return stopOwnedTailscaleProcess(ctx, runtime)`},
		"tailscale_funnel_windows.go": {`tailscaleFunnelArgs(target)`, `processcontrol.Attach(command)`, `guard.stopRequested()`, `tailscale-funnel.pid`, `"status", "--json"`, `windows.QueryFullProcessImageName(process`, `windows.TerminateProcess(process, 0)`},
		"tunnel_config_windows.go":    {`resolveTailscaleFunnelInfo(ctx, configured)`, `runtime.updateManifest("tailscale", tailscalePublicURL)`, `platformSetTunnelAutostart(ctx, runtime.root, true)`},
	}
	for file, wants := range checks {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s missing %q", file, want)
			}
		}
	}
}
