//go:build darwin || linux

package desktopruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/uvwt/agentdock/internal/envstore"
)

func TestBasicSettingsQuickURLDoesNotStartStoppedCore(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "agentdock.env")
	if err := os.WriteFile(path, []byte("AGENTDOCK_PORT=8877\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cloudflared := filepath.Join(root, "fake-cloudflared")
	script := "#!/bin/sh\necho 'INF | Your quick Tunnel has been created! Visit it at https://fresh.trycloudflare.com |'\n"
	if err := os.WriteFile(cloudflared, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTDOCK_LAUNCHCTL_BIN", "/usr/bin/false")
	manifest := unixRuntimeManifest{CloudflaredBinary: cloudflared, EnvironmentFile: path, ServiceManager: "none"}
	// Any attempt to restart Core fails with this deliberately inactive adapter.
	if err := runQuickTunnel(context.Background(), manifest, root, root, "http://127.0.0.1:8877", io.Discard); err != nil {
		t.Fatal(err)
	}
	values, err := envstore.ParseFile(path)
	if err != nil || values["AGENTDOCK_SERVER_URL"] != "https://fresh.trycloudflare.com" {
		t.Fatal("URL was not applied")
	}
}

func TestBasicEnvironmentPreservesCredentialsAndAdvancedValues(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "agentdock.env")
	original := envstore.Marshal(map[string]string{"AGENTDOCK_PORT": "8765", "AGENTDOCK_LOG_LEVEL": "info", "AGENTDOCK_AUTH_TOKEN": "secret", "AGENTDOCK_ACP_PROFILES_JSON": `[{"env":{"KEY":"value"}}]`, "CUSTOM": "first\nsecond"})
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, values, err := readBasicEnvironment(root)
	if err != nil {
		t.Fatal(err)
	}
	values["AGENTDOCK_PORT"] = "8877"
	values["AGENTDOCK_LOG_LEVEL"] = "debug"
	if err := replaceBasicEnvironments(context.Background(), []basicEnvironmentChange{{path, original, envstore.Marshal(values)}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	actual, err := envstore.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range values {
		if actual[key] != value {
			t.Fatalf("value lost: %s", key)
		}
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatal(info.Mode())
	}
}

func TestBasicQuickTunnelTargetPreservesValuesAndNamedBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cloudflared.env")
	for _, mode := range []string{"quick", "named", "none"} {
		original := []byte("# retain on rollback\nAGENTDOCK_TUNNEL_MODE=" + mode + "\nAGENTDOCK_TUNNEL_TARGET='http://127.0.0.1:8765'\nTUNNEL_TOKEN='secret'\nCUSTOM='first\nsecond'\n")
		if err := os.WriteFile(path, original, 0o600); err != nil {
			t.Fatal(err)
		}
		change, err := basicQuickTunnelChange(path, "http://[::1]:8877")
		if err != nil {
			t.Fatal(err)
		}
		if mode != "quick" {
			if change != nil {
				t.Fatal("non-quick configuration changed")
			}
			continue
		}
		values, err := envstore.Parse(change.updated)
		if err != nil || values["AGENTDOCK_TUNNEL_TARGET"] != "http://[::1]:8877" || values["TUNNEL_TOKEN"] != "secret" || values["CUSTOM"] != "first\nsecond" {
			t.Fatalf("values lost: %v %v", values, err)
		}
		if string(change.original) != string(original) {
			t.Fatal("snapshot changed")
		}
	}
}

func TestBasicEnvironmentTransactionRunStates(t *testing.T) {
	for _, coreRunning := range []bool{false, true} {
		for _, tunnelRunning := range []bool{false, true} {
			t.Run(fmt.Sprintf("core=%t/tunnel=%t", coreRunning, tunnelRunning), func(t *testing.T) {
				root := t.TempDir()
				changes := []basicEnvironmentChange{{filepath.Join(root, "core"), []byte("old core"), []byte("new core")}, {filepath.Join(root, "tunnel"), []byte("old tunnel"), []byte("new tunnel")}}
				var core, tunnel func(context.Context) error
				calls := 0
				check := func(context.Context) error {
					calls++
					for _, change := range changes {
						data, err := os.ReadFile(change.path)
						if err != nil || string(data) != string(change.updated) {
							t.Fatal("restart before both writes")
						}
					}
					return nil
				}
				want := 0
				if coreRunning {
					core = check
					want++
				}
				if tunnelRunning {
					tunnel = check
					want++
				}
				if err := replaceBasicEnvironments(context.Background(), changes, core, tunnel); err != nil {
					t.Fatal(err)
				}
				if calls != want {
					t.Fatalf("restarted inactive component: %d", calls)
				}
			})
		}
	}
}

func TestBasicEnvironmentTransactionRollsBackBothFiles(t *testing.T) {
	for _, failure := range []string{"write", "core", "tunnel"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			changes := []basicEnvironmentChange{{filepath.Join(root, "core"), []byte("# core\nSECRET='exact'\n"), []byte("changed core")}, {filepath.Join(root, "tunnel"), []byte("# tunnel\nCUSTOM='exact'\n"), []byte("changed tunnel")}}
			for _, change := range changes {
				if err := os.WriteFile(change.path, change.original, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "write" {
				blocked := filepath.Join(root, "blocked")
				if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
				changes[1].path = filepath.Join(blocked, "tunnel")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failed := false
			calls := 0
			restart := func(component string) func(context.Context) error {
				return func(ctx context.Context) error {
					calls++
					if !failed && component == failure {
						failed = true
						cancel()
						return context.Canceled
					}
					if failed {
						if ctx.Err() != nil {
							t.Fatal("canceled recovery context")
						}
						for _, change := range changes {
							data, err := os.ReadFile(change.path)
							if err != nil || string(data) != string(change.original) {
								t.Fatal("recovery restart before exact restoration")
							}
						}
					}
					return nil
				}
			}
			err := replaceBasicEnvironments(ctx, changes, restart("core"), restart("tunnel"))
			if err == nil {
				t.Fatal("failure hidden")
			}
			data, _ := os.ReadFile(changes[0].path)
			if string(data) != string(changes[0].original) {
				t.Fatal("core not restored")
			}
			if failure == "write" && calls != 0 {
				t.Fatal("write failure restarted services")
			}
			if failure != "write" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestBasicEnvironmentRollbackAfterCanceledRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentdock.env")
	original := []byte("# original\nCUSTOM='secret'\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := replaceBasicEnvironments(ctx, []basicEnvironmentChange{{path, original, []byte("CUSTOM='changed'\n")}}, func(ctx context.Context) error {
		calls++
		if calls == 1 {
			cancel()
			return context.Canceled
		}
		if ctx.Err() != nil {
			t.Fatal("recovery reused canceled context")
		}
		data, _ := os.ReadFile(path)
		if string(data) != string(original) {
			t.Fatal("restart before restoring exact bytes")
		}
		return nil
	}, nil)
	data, _ := os.ReadFile(path)
	if !errors.Is(err, context.Canceled) || calls != 2 || string(data) != string(original) {
		t.Fatalf("rollback: %v, calls %d", err, calls)
	}
}

func TestBasicEnvironmentFailsClosed(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "agentdock.env")
	if err := os.WriteFile(path, []byte("AGENTDOCK_PORT=8765\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := readBasicEnvironment(root); err == nil {
		t.Fatal("broad permissions accepted")
	}
	target := filepath.Join(root, "target")
	if err := os.Rename(path, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := readBasicEnvironment(root); err == nil {
		t.Fatal("symlink accepted")
	}
	if runtime.GOOS == "darwin" && !errors.Is(platformBasicAutostartAvailable(), ErrBasicSettingsUnavailable) {
		t.Fatal("macOS autostart boundary lost")
	}
}
