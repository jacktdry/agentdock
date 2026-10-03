//go:build windows

package desktopruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func basicSettingsTestRuntime(t *testing.T) tunnelRuntime {
	t.Helper()
	root := t.TempDir()
	manifest := Manifest{SchemaVersion: SchemaVersion, AgentDockBinary: filepath.Join(root, "agentdock.exe"), CloudflaredBinary: filepath.Join(root, "cloudflared.exe"), Host: "127.0.0.1", Port: 8765, LocalMCPURL: "http://127.0.0.1:8765/mcp", TunnelMode: "quick"}
	if err := Save(filepath.Join(root, "runtime.json"), manifest); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{"control-panel-settings.json": "{\"port\":8765,\"log_level\":\"info\"}\n", "server-url.txt": "https://old.trycloudflare.com", "quick-tunnel-url.txt": "https://old.trycloudflare.com"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runtime, err := loadTunnelRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func TestManagedTunnelRunningIncludesSupervisorBackoff(t *testing.T) {
	if managedTunnelRunning(false, 0) {
		t.Fatal("inactive tunnel reported running")
	}
	if !managedTunnelRunning(true, 0) {
		t.Fatal("cloudflared process not counted")
	}
	if !managedTunnelRunning(false, 42) {
		t.Fatal("live supervisor backoff not counted")
	}
}

func TestBasicSettingsWindowsRunStatePolicy(t *testing.T) {
	for _, mode := range []string{"none", "quick", "named"} {
		if got := controlPanelState(mode, nil); got != (controlPanelRunState{true, mode != "none"}) {
			t.Fatal("legacy policy changed")
		}
	}
	for _, core := range []bool{false, true} {
		for _, tunnel := range []bool{false, true} {
			t.Run(fmt.Sprintf("core=%t/tunnel=%t", core, tunnel), func(t *testing.T) {
				runtime := basicSettingsTestRuntime(t)
				settings := runtime.settings
				settings.Port, settings.LogLevel = 8877, "debug"
				var calls []string
				actions := controlPanelActions{
					stopTunnel: func(context.Context, tunnelRuntime) error { calls = append(calls, "stop tunnel"); return nil },
					coreAction: func(context.Context, string, string) error { calls = append(calls, "restart core"); return nil },
					startTunnel: func(_ context.Context, runtime tunnelRuntime) error {
						calls = append(calls, "start tunnel")
						manifest, err := Load(runtime.files.manifest)
						if err != nil || manifest.Port != 8877 || runtime.settings.Port != 8877 || !runtime.preserveStoppedCore {
							t.Fatal("tunnel started with old port or without state policy")
						}
						return nil
					},
				}
				if err := applyControlPanelSettingsWithState(context.Background(), runtime, settings, []byte(`{"port":8877,"log_level":"debug"}`), &controlPanelRunState{core, tunnel}, actions); err != nil {
					t.Fatal(err)
				}
				var want []string
				if tunnel {
					want = append(want, "stop tunnel")
				}
				if core {
					want = append(want, "restart core")
				}
				if tunnel {
					want = append(want, "start tunnel")
				}
				if !slices.Equal(calls, want) {
					t.Fatalf("calls %v want %v", calls, want)
				}
			})
		}
	}
}

func TestBasicSettingsWindowsRollbackRestoresFilesAndRunStates(t *testing.T) {
	for _, core := range []bool{false, true} {
		t.Run(fmt.Sprintf("core=%t", core), func(t *testing.T) {
			runtime := basicSettingsTestRuntime(t)
			paths := []string{filepath.Join(runtime.root, "control-panel-settings.json"), runtime.files.manifest, runtime.files.serverURL, runtime.files.quickURL}
			original := map[string][]byte{}
			for _, path := range paths {
				original[path], _ = os.ReadFile(path)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failed := false
			var calls []string
			checkRecovery := func(ctx context.Context) {
				if ctx.Err() != nil {
					t.Fatal("recovery context canceled")
				}
				for path, data := range original {
					got, err := os.ReadFile(path)
					if err != nil || !slices.Equal(got, data) {
						t.Fatalf("file not restored before restart: %s", path)
					}
				}
			}
			actions := controlPanelActions{
				stopTunnel: func(ctx context.Context, _ tunnelRuntime) error {
					if failed && ctx.Err() != nil {
						t.Fatal("stop recovery context canceled")
					}
					calls = append(calls, "stop tunnel")
					return nil
				},
				coreAction: func(ctx context.Context, _, _ string) error {
					if failed {
						checkRecovery(ctx)
					}
					calls = append(calls, "restart core")
					return nil
				},
				startTunnel: func(ctx context.Context, runtime tunnelRuntime) error {
					calls = append(calls, "start tunnel")
					if !failed {
						failed = true
						cancel()
						return context.Canceled
					}
					checkRecovery(ctx)
					if runtime.settings.Port != 8765 || !runtime.preserveStoppedCore {
						t.Fatal("recovery runtime changed")
					}
					return nil
				},
			}
			settings := runtime.settings
			settings.Port = 8877
			err := applyControlPanelSettingsWithState(ctx, runtime, settings, []byte(`{"port":8877,"log_level":"info"}`), &controlPanelRunState{core, true}, actions)
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			want := []string{"stop tunnel"}
			if core {
				want = append(want, "restart core")
			}
			want = append(want, "start tunnel", "stop tunnel")
			if core {
				want = append(want, "restart core")
			}
			want = append(want, "start tunnel")
			if !slices.Equal(calls, want) {
				t.Fatalf("calls %v want %v", calls, want)
			}
		})
	}
}

func TestBasicSettingsWindowsSupervisorPolicyDoesNotLeakToLegacy(t *testing.T) {
	environment := []string{"CUSTOM=keep", "AGENTDOCK_TUNNEL_PRESERVE_STOPPED_CORE=1"}
	if got := tunnelSupervisorEnvironment(environment, false); !slices.Equal(got, []string{"CUSTOM=keep"}) {
		t.Fatal("basic policy leaked into legacy launch")
	}
	if got := tunnelSupervisorEnvironment(environment, true); !slices.Equal(got, environment) {
		t.Fatal("supervisor lost basic state policy")
	}
}

func TestBasicSettingsWindowsRollbackKeepsInactiveComponentsStopped(t *testing.T) {
	for _, core := range []bool{false, true} {
		t.Run(fmt.Sprintf("core=%t", core), func(t *testing.T) {
			runtime := basicSettingsTestRuntime(t)
			original := map[string][]byte{}
			for _, path := range []string{filepath.Join(runtime.root, "control-panel-settings.json"), runtime.files.manifest, runtime.files.serverURL, runtime.files.quickURL} {
				original[path], _ = os.ReadFile(path)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			coreCalls := 0
			actions := controlPanelActions{
				stopTunnel:  func(context.Context, tunnelRuntime) error { t.Fatal("stopped tunnel touched"); return nil },
				startTunnel: func(context.Context, tunnelRuntime) error { t.Fatal("stopped tunnel started"); return nil },
				coreAction: func(ctx context.Context, _, _ string) error {
					coreCalls++
					if ctx.Err() != nil {
						t.Fatal("recovery reused canceled caller")
					}
					for path, before := range original {
						after, err := os.ReadFile(path)
						if err != nil || !slices.Equal(before, after) {
							t.Fatal("recovery before restoration")
						}
					}
					return nil
				},
			}
			settings := runtime.settings
			settings.Port = 0 // Manifest validation fails after the settings/URL writes.
			if err := applyControlPanelSettingsWithState(ctx, runtime, settings, []byte(`{"port":0}`), &controlPanelRunState{core, false}, actions); err == nil {
				t.Fatal("apply failure hidden")
			}
			want := 0
			if core {
				want = 1
			}
			if coreCalls != want {
				t.Fatal("prior core state lost")
			}
			for path, before := range original {
				after, err := os.ReadFile(path)
				if err != nil || !slices.Equal(before, after) {
					t.Fatal("exact bytes not restored")
				}
			}
		})
	}
}

func TestBasicSettingsPatchPreservesAllAdvancedFields(t *testing.T) {
	original := []byte(`{"port":8765,"log_level":"info","browser_enabled":true,"acp_profiles":[{"env":{"SECRET":"value"}}],"mcp_apps_mode":"off","oauth_access_token_ttl":"1h","future":{"secret":"keep"}}`)
	updated, err := patchBasicSettingsJSON(original, BasicSettings{Port: 8877, LogLevel: "debug"})
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]any
	if err := json.Unmarshal(original, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(updated, &after); err != nil {
		t.Fatal(err)
	}
	before["port"], before["log_level"] = float64(8877), "debug"
	if !reflect.DeepEqual(before, after) {
		t.Fatal("advanced configuration lost")
	}
	for _, invalid := range []string{"null", "[]", "broken"} {
		if _, err := patchBasicSettingsJSON([]byte(invalid), BasicSettings{}); err == nil {
			t.Fatal(invalid)
		}
	}
}
