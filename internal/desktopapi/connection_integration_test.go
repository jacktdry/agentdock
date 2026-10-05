package desktopapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

func integratedConnectionFixture(t *testing.T) (*ConnectionService, *desktopruntime.ConnectionConfig) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := fixtureConnectionConfig("https://next.example.test")
	c.Mode = "quick"
	service := NewConnectionServiceWithDependencies(root, ConnectionDependencies{
		ReadConfig: func(context.Context, string) (desktopruntime.ConnectionConfig, error) { return c, nil },
		ReadPasswordState: func(context.Context, string) desktopruntime.OAuthPasswordState {
			return desktopruntime.OAuthPasswordStored
		},
		SelectRuntime: func(context.Context, string) (desktopruntime.NextConnectionRuntime, error) {
			running := false
			return desktopruntime.NextConnectionRuntime{PortRuntime: desktopruntime.NextPortRuntime{Variant: "next", Root: root, Binary: filepath.Join(root, "core")}, BindHost: "127.0.0.1", Running: &running, Health: "unknown"}, nil
		},
		// These fixtures never bind or query a production port.
		ObservePort: func(_ context.Context, r desktopruntime.PortObservationRequest) desktopruntime.PortObservation {
			state, reason := desktopruntime.PortAvailable, "bind_available"
			if r.CandidatePort == 8765 {
				state, reason = desktopruntime.PortReserved, "stable_reserved"
			}
			if r.CandidatePort == 8766 {
				state, reason = desktopruntime.PortReserved, "memory_reserved"
			}
			return desktopruntime.PortObservation{ConfiguredPort: r.ConfiguredPort, ObservedPort: r.CandidatePort, State: state, ReasonCode: reason, ConfigRevision: r.ConfigRevision, ObservedAt: time.Now().UTC()}
		},
		ReadBasic: func(context.Context, string) (desktopruntime.BasicSettings, error) {
			return desktopruntime.BasicSettings{Port: c.Port, LogLevel: "info"}, nil
		},
		UpdateBasic: func(_ context.Context, _ string, settings desktopruntime.BasicSettings) error {
			c.Port = settings.Port
			c.CoreEndpoint = fmt.Sprintf("http://127.0.0.1:%d", c.Port)
			return nil
		},
		CoreAction: func(context.Context, string, string) error { return nil },
	})
	service.foundation.PreflightPort = func(ctx context.Context, request desktopruntime.PortObservationRequest) (desktopruntime.PortObservation, error) {
		o := service.foundation.ObservePort(ctx, request)
		if o.State != desktopruntime.PortAvailable && o.State != desktopruntime.PortOwnedByNext {
			return o, desktopruntime.ErrPortPreflight
		}
		return o, nil
	}
	service.run = func(_ context.Context, args []string, out, _ io.Writer) error {
		if args[0] == "status" {
			running := false
			return json.NewEncoder(out).Encode(desktopruntime.TunnelStatus{Mode: c.Mode, Observation: &desktopruntime.TunnelObservation{Running: &running, Autostart: "disabled", TokenState: "stored", PublicEndpoint: "unchecked"}})
		}
		return nil
	}
	return service, &c
}

func TestIntegratedSnapshotAndPreflight(t *testing.T) {
	service, c := integratedConnectionFixture(t)
	result := service.Snapshot(context.Background())
	data, _ := json.Marshal(result)
	if result.Error != nil || result.Snapshot.Tunnel.TokenState != "stored" || result.Snapshot.CoreHealth != "unknown" || result.Snapshot.PortObservation.State != desktopruntime.PortAvailable || len(result.Snapshot.Operations) != 11 || result.Snapshot.Tunnel.Generation != c.TunnelGeneration {
		t.Fatal(string(data))
	}
	for _, forbidden := range []string{"passwordBytes", "bearer", "desktopControlSecret", "SIGNING_SECRET", "TUNNEL_TOKEN"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatal("secret in snapshot")
		}
	}
	for port, state := range map[int]desktopruntime.PortState{8765: desktopruntime.PortReserved, 8766: desktopruntime.PortReserved, 19000: desktopruntime.PortAvailable} {
		got := service.PreflightPort(context.Background(), port)
		if got.Error != nil || got.Observation.State != state {
			t.Fatal(got)
		}
	}
	service.foundation.SelectRuntime = func(context.Context, string) (desktopruntime.NextConnectionRuntime, error) {
		return desktopruntime.NextConnectionRuntime{}, desktopruntime.ErrNextIdentityUnavailable
	}
	got := service.PreflightPort(context.Background(), 19000)
	if got.Observation.State != desktopruntime.PortUnknown {
		t.Fatal(got)
	}
	snapshot := service.Snapshot(context.Background())
	for _, op := range snapshot.Snapshot.Operations {
		if op.Access == AccessMutating && (op.Availability != AvailabilityUnavailable || op.DisabledReason != "next_identity_unavailable") {
			t.Fatal(op)
		}
	}
}

func TestConnectionStaleRevisionBeforeAnyEffects(t *testing.T) {
	service, c := integratedConnectionFixture(t)
	service.run = func(context.Context, []string, io.Writer, io.Writer) error {
		t.Fatal("stale request called adapter")
		return nil
	}
	service.foundation.UpdateBasic = func(context.Context, string, desktopruntime.BasicSettings) error { t.Fatal("stale write"); return nil }
	for _, operation := range []func() ConnectionActionResult{
		func() ConnectionActionResult {
			return service.UpdatePort(context.Background(), ConnectionPortRequest{19000, "stale"})
		},
		func() ConnectionActionResult {
			return service.ConfigureTunnel(context.Background(), ConnectionTunnelRequest{Mode: "named", NamedOrigin: "https://next.example.test", NewToken: "SECRET", ConfigRevision: "stale"})
		},
		func() ConnectionActionResult {
			return service.SetTunnelAutostart(context.Background(), ConnectionAutostartRequest{true, "stale"})
		},
		func() ConnectionActionResult { return service.Action(context.Background(), "restart", "stale") },
	} {
		result := operation()
		if result.Error == nil || result.Error.Code != "connection_config_stale" || result.OperationID == "" || result.Completed {
			t.Fatal(result)
		}
	}
	entries, _ := os.ReadDir(service.runtimeRoot)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".connection-token-") || entry.Name() == ".desktop-mutation.lock" {
			t.Fatal("stale request wrote token")
		}
	}
	_ = c
}

func TestConnectionPortRechecksAndFinalFailure(t *testing.T) {
	service, c := integratedConnectionFixture(t)
	// A feedback preflight can succeed while the authoritative one rejects.
	if service.PreflightPort(context.Background(), 19000).Observation.State != desktopruntime.PortAvailable {
		t.Fatal("feedback unavailable")
	}
	preflights, effects := 0, 0
	service.foundation.PreflightPort = func(ctx context.Context, r desktopruntime.PortObservationRequest) (desktopruntime.PortObservation, error) {
		preflights++
		if r.Runtime.Variant != "next" || r.Runtime.Root != service.runtimeRoot || r.ConfigRevision != ConnectionConfigRevision(*c) {
			t.Fatal("untrusted preflight")
		}
		// The outer lock must already be held; reacquisition must time out.
		timeout, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		defer cancel()
		if release, err := desktopruntime.AcquireDesktopMutation(timeout, service.runtimeRoot); err == nil {
			release()
			t.Fatal("no outer lock")
		}
		return desktopruntime.PortObservation{State: desktopruntime.PortUnknown, ReasonCode: "ownership_unavailable"}, desktopruntime.ErrPortPreflight
	}
	service.foundation.UpdateBasic = func(context.Context, string, desktopruntime.BasicSettings) error {
		effects++
		return errors.New("FINAL_BIND_SECRET")
	}
	request := ConnectionPortRequest{19000, ConnectionConfigRevision(*c)}
	result := service.UpdatePort(context.Background(), request)
	if result.Error == nil || preflights != 1 || effects != 0 {
		t.Fatal(result)
	}
	service.foundation.PreflightPort = func(_ context.Context, r desktopruntime.PortObservationRequest) (desktopruntime.PortObservation, error) {
		preflights++
		return service.foundation.ObservePort(context.Background(), r), nil
	}
	result = service.UpdatePort(context.Background(), request)
	data, _ := json.Marshal(result)
	if result.Completed || result.Error == nil || effects != 1 || strings.Contains(string(data), "FINAL_BIND_SECRET") {
		t.Fatal(string(data))
	}
}

func TestConnectionNamedCustomPortHasNoEffects(t *testing.T) {
	service, c := integratedConnectionFixture(t)
	c.Mode = "named"
	service.foundation.UpdateBasic = func(context.Context, string, desktopruntime.BasicSettings) error {
		t.Fatal("Named custom port changed")
		return nil
	}
	result := service.UpdatePort(context.Background(), ConnectionPortRequest{19000, ConnectionConfigRevision(*c)})
	if result.Error == nil || result.Error.Code != "named_manual_route_required" {
		t.Fatal(result)
	}
	c.Port, c.CoreEndpoint = 19000, "http://127.0.0.1:19000"
	adapter := service.run
	service.run = func(ctx context.Context, args []string, out, stderr io.Writer) error {
		if args[0] != "status" {
			t.Fatal("Named configure had effects")
		}
		return adapter(ctx, args, out, stderr)
	}
	result = service.ConfigureTunnel(context.Background(), ConnectionTunnelRequest{Mode: "named", NamedOrigin: "https://next.example.test", NewToken: "SECRET", ConfigRevision: ConnectionConfigRevision(*c)})
	if result.Error == nil || result.Error.Code != "named_manual_route_required" {
		t.Fatal(result)
	}
}

func TestConnectionTokenBridgeCleanupAndNoLeak(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(strconvBool(fail), func(t *testing.T) {
			service, c := integratedConnectionFixture(t)
			adapter := service.run
			tokenPath := ""
			service.run = func(ctx context.Context, args []string, out, stderr io.Writer) error {
				if args[0] == "status" {
					return adapter(ctx, args, out, stderr)
				}
				for i, arg := range args {
					if strings.Contains(arg, "TOKEN_SENTINEL") {
						t.Fatal("token in argv")
					}
					if arg == "--token-file" {
						tokenPath = args[i+1]
					}
				}
				info, err := os.Lstat(tokenPath)
				if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || filepath.Dir(tokenPath) != service.runtimeRoot {
					t.Fatal("unsafe temp token")
				}
				data, _ := os.ReadFile(tokenPath)
				if string(data) != "TOKEN_SENTINEL" {
					t.Fatal("missing token")
				}
				if fail {
					return errors.New("TOKEN_SENTINEL raw error")
				}
				return nil
			}
			result := service.ConfigureTunnel(context.Background(), ConnectionTunnelRequest{Mode: "named", NamedOrigin: "https://next.example.test", NewToken: "TOKEN_SENTINEL", ConfigRevision: ConnectionConfigRevision(*c)})
			data, _ := json.Marshal(result)
			if strings.Contains(string(data), "TOKEN_SENTINEL") || result.Completed == fail || tokenPath == "" {
				t.Fatal(string(data))
			}
			if _, err := os.Lstat(tokenPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("token not cleaned")
			}
		})
	}
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if path, cleanup, err := privateConnectionTokenFile(alias, "TOKEN"); err == nil {
		cleanup()
		t.Fatal("symlink root accepted", path)
	}
}

func strconvBool(value bool) string {
	if value {
		return "failure"
	}
	return "success"
}

func TestConnectionConfigureAutostartSerializeWithoutNestedLock(t *testing.T) {
	service, c := integratedConnectionFixture(t)
	adapter := service.run
	entered, unblock := make(chan struct{}), make(chan struct{})
	var writes atomic.Int32
	service.run = func(ctx context.Context, args []string, out, stderr io.Writer) error {
		if args[0] == "status" {
			return adapter(ctx, args, out, stderr)
		}
		writes.Add(1)
		if args[0] == "configure" {
			close(entered)
			<-unblock
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	first, second := make(chan ConnectionActionResult, 1), make(chan ConnectionActionResult, 1)
	go func() {
		first <- service.ConfigureTunnel(ctx, ConnectionTunnelRequest{Mode: "quick", ConfigRevision: ConnectionConfigRevision(*c)})
	}()
	<-entered
	go func() {
		second <- service.SetTunnelAutostart(ctx, ConnectionAutostartRequest{true, ConnectionConfigRevision(*c)})
	}()
	select {
	case <-second:
		t.Fatal("autostart bypassed configure lock")
	case <-time.After(50 * time.Millisecond):
	}
	close(unblock)
	if result := <-first; !result.Completed || result.Phase != "applied" {
		t.Fatal(result)
	}
	result := <-second
	if runtime.GOOS == "darwin" {
		if result.Error == nil || result.Error.Code != "native_service_management_required" {
			t.Fatal(result)
		}
	} else if !result.Completed {
		t.Fatal(result)
	}
	expected := int32(2)
	if runtime.GOOS == "darwin" {
		expected = 1
	}
	if writes.Load() != expected {
		t.Fatal("unexpected effects")
	}
}

func TestBasicSettingsNextPortBypassAndStableBehavior(t *testing.T) {
	for _, variant := range []string{"next", "stable"} {
		t.Run(variant, func(t *testing.T) {
			t.Setenv("AGENTDOCK_DESKTOP_VARIANT", variant)
			service := NewBasicSettingsService(t.TempDir())
			service.validateIdentity = func(context.Context, string) error { return nil }
			current := desktopruntime.BasicSettings{Port: 19000, LogLevel: "info"}
			service.read = func(context.Context, string) (desktopruntime.BasicSettings, error) { return current, nil }
			writes := 0
			service.update = func(_ context.Context, _ string, requested desktopruntime.BasicSettings) error {
				writes++
				current = requested
				return nil
			}
			read := service.Read(context.Background())
			if read.PortMutable == (variant == "next") {
				t.Fatal("wrong port metadata")
			}
			result := service.Save(context.Background(), BasicSettings{Port: 19001, LogLevel: "info"})
			if variant == "next" {
				if result.Error == nil || result.Error.Code != "port_managed_by_connection" || writes != 0 {
					t.Fatal(result)
				}
			} else if !result.Completed || writes != 1 {
				t.Fatal(result)
			}
			result = service.Save(context.Background(), BasicSettings{Port: current.Port, LogLevel: "debug"})
			if !result.Completed || current.LogLevel != "debug" {
				t.Fatal("non-port settings disabled")
			}
		})
	}
}

func TestConnectionAuthoritativeFixturePortPreflight(t *testing.T) {
	service, c := integratedConnectionFixture(t)
	checks, writes := 0, 0
	service.foundation.PreflightPort = func(_ context.Context, r desktopruntime.PortObservationRequest) (desktopruntime.PortObservation, error) {
		checks++
		state := desktopruntime.PortAvailable
		if checks > 2 {
			state = desktopruntime.PortConflict
		}
		o := desktopruntime.PortObservation{State: state, ReasonCode: "fixture_listener", ObservedPort: r.CandidatePort}
		if state == desktopruntime.PortConflict {
			return o, desktopruntime.ErrPortPreflight
		}
		return o, nil
	}
	update := service.foundation.UpdateBasic
	service.foundation.UpdateBasic = func(ctx context.Context, root string, settings desktopruntime.BasicSettings) error {
		writes++
		return update(ctx, root, settings)
	}
	if result := service.UpdatePort(context.Background(), ConnectionPortRequest{19000, ConnectionConfigRevision(*c)}); !result.Completed {
		t.Fatal(result)
	}
	if result := service.UpdatePort(context.Background(), ConnectionPortRequest{19001, ConnectionConfigRevision(*c)}); result.Completed || result.Error == nil || writes != 1 {
		t.Fatal("occupied fixture accepted", result)
	}
}

func TestConnectionRejectsIdentityBeforeLockCreation(t *testing.T) {
	service, c := integratedConnectionFixture(t)
	service.foundation.SelectRuntime = func(context.Context, string) (desktopruntime.NextConnectionRuntime, error) {
		return desktopruntime.NextConnectionRuntime{}, desktopruntime.ErrNextIdentityUnavailable
	}
	service.run = func(context.Context, []string, io.Writer, io.Writer) error {
		t.Fatal("unowned adapter queried")
		return nil
	}
	result := service.Action(context.Background(), "start", ConnectionConfigRevision(*c))
	if result.Error == nil || result.Error.Code != "next_identity_unavailable" {
		t.Fatal(result)
	}
	entries, err := os.ReadDir(service.runtimeRoot)
	if err != nil || len(entries) != 0 {
		t.Fatal("unowned root written", entries, err)
	}
}

func TestConnectionTunnelCurrentPortRejectsBeforeMutation(t *testing.T) {
	for _, state := range []desktopruntime.PortState{desktopruntime.PortReserved, desktopruntime.PortConflict, desktopruntime.PortUnknown} {
		for _, action := range []string{"configureTunnel", "start", "restart", "regenerate"} {
			t.Run(string(state)+"/"+action, func(t *testing.T) {
				service, c := integratedConnectionFixture(t)
				adapter := service.run
				service.run = func(ctx context.Context, args []string, out, stderr io.Writer) error {
					if args[0] != "status" {
						t.Fatal("mutation after rejected current port")
					}
					return adapter(ctx, args, out, stderr)
				}
				service.foundation.PreflightPort = func(ctx context.Context, request desktopruntime.PortObservationRequest) (desktopruntime.PortObservation, error) {
					if request.CandidatePort != c.Port {
						t.Fatal("candidate is not configured port")
					}
					timeout, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
					defer cancel()
					if release, err := desktopruntime.AcquireDesktopMutation(timeout, service.runtimeRoot); err == nil {
						release()
						t.Fatal("preflight without lock")
					}
					return desktopruntime.PortObservation{State: state, ReasonCode: "fixture_reject"}, desktopruntime.ErrPortPreflight
				}
				var result ConnectionActionResult
				if action == "configureTunnel" {
					result = service.ConfigureTunnel(context.Background(), ConnectionTunnelRequest{Mode: "quick", ConfigRevision: ConnectionConfigRevision(*c)})
				} else {
					result = service.Action(context.Background(), action, ConnectionConfigRevision(*c))
				}
				if result.Completed || result.Error == nil || result.Error.Code != "fixture_reject" {
					t.Fatal(result)
				}
			})
		}
	}
}

func TestConnectionPortRollbackRestoresLifecycleAndOwnership(t *testing.T) {
	service, c := integratedConnectionFixture(t)
	oldPort := c.Port
	running := true
	selections := 0
	service.foundation.SelectRuntime = func(context.Context, string) (desktopruntime.NextConnectionRuntime, error) {
		selections++
		return desktopruntime.NextConnectionRuntime{
			PortRuntime: desktopruntime.NextPortRuntime{Variant: "next", Root: service.runtimeRoot, Binary: "fixture", PID: selections, ProcessInstance: fmt.Sprint(selections)},
			BindHost:    "127.0.0.1", Running: &running,
		}, nil
	}
	checks, writes, coreActions := 0, 0, 0
	service.foundation.PreflightPort = func(_ context.Context, r desktopruntime.PortObservationRequest) (desktopruntime.PortObservation, error) {
		checks++
		state := desktopruntime.PortAvailable
		if checks == 2 {
			state = desktopruntime.PortConflict
		}
		if checks == 3 {
			if r.CandidatePort != oldPort || !running {
				t.Fatal("rollback ownership checked before lifecycle restore")
			}
			state = desktopruntime.PortOwnedByNext
		}
		o := desktopruntime.PortObservation{State: state, ReasonCode: "fixture_final", ObservedPort: r.CandidatePort}
		if state == desktopruntime.PortConflict || state == desktopruntime.PortUnknown {
			return o, desktopruntime.ErrPortPreflight
		}
		return o, nil
	}
	service.foundation.UpdateBasic = func(ctx context.Context, _ string, settings desktopruntime.BasicSettings) error {
		writes++
		if ctx.Err() != nil {
			t.Fatal("rollback used canceled context")
		}
		c.Port = settings.Port
		c.CoreEndpoint = fmt.Sprintf("http://127.0.0.1:%d", c.Port)
		if writes == 1 {
			// Simulate a misleading generic health success while the selected Core
			// actually failed to own the new listener.
			running = false
		} else if settings.Port != oldPort {
			t.Fatal("rollback did not restore old port")
		}
		return nil
	}
	service.foundation.CoreAction = func(_ context.Context, root, action string) error {
		coreActions++
		if root != service.runtimeRoot || action != "restart" {
			t.Fatal("unexpected recovery lifecycle action", root, action)
		}
		running = true
		return nil
	}
	result := service.UpdatePort(context.Background(), ConnectionPortRequest{19000, ConnectionConfigRevision(*c)})
	if result.Completed || result.Error == nil || result.Phase != "rolled_back" || c.Port != oldPort || writes != 2 || coreActions != 1 || checks != 3 {
		t.Fatal(result, c.Port, writes, coreActions, checks)
	}
	if result.PortObservation == nil || result.PortObservation.State != desktopruntime.PortOwnedByNext {
		t.Fatal("rollback ownership was not proven", result.PortObservation)
	}
}

func TestConnectionPortRollbackFailureRequiresRecovery(t *testing.T) {
	for _, failure := range []string{"restart", "ownership"} {
		t.Run(failure, func(t *testing.T) {
			service, c := integratedConnectionFixture(t)
			oldPort := c.Port
			running := true
			service.foundation.SelectRuntime = func(context.Context, string) (desktopruntime.NextConnectionRuntime, error) {
				return desktopruntime.NextConnectionRuntime{
					PortRuntime: desktopruntime.NextPortRuntime{Variant: "next", Root: service.runtimeRoot, Binary: "fixture", PID: 42, ProcessInstance: "fixture"},
					BindHost:    "127.0.0.1", Running: &running,
				}, nil
			}
			checks, writes := 0, 0
			service.foundation.PreflightPort = func(_ context.Context, r desktopruntime.PortObservationRequest) (desktopruntime.PortObservation, error) {
				checks++
				state := desktopruntime.PortAvailable
				if checks == 2 {
					state = desktopruntime.PortConflict
				}
				if checks == 3 {
					state = desktopruntime.PortConflict
				}
				o := desktopruntime.PortObservation{State: state, ReasonCode: "fixture_failure", ObservedPort: r.CandidatePort}
				if state == desktopruntime.PortConflict {
					return o, desktopruntime.ErrPortPreflight
				}
				return o, nil
			}
			service.foundation.UpdateBasic = func(_ context.Context, _ string, settings desktopruntime.BasicSettings) error {
				writes++
				c.Port = settings.Port
				c.CoreEndpoint = fmt.Sprintf("http://127.0.0.1:%d", c.Port)
				if writes == 1 {
					running = false
				}
				return nil
			}
			service.foundation.CoreAction = func(context.Context, string, string) error {
				if failure == "restart" {
					return errors.New("RECOVERY_SECRET")
				}
				running = true
				return nil
			}
			result := service.UpdatePort(context.Background(), ConnectionPortRequest{19000, ConnectionConfigRevision(*c)})
			if result.Completed || result.Error == nil || result.Phase != "recovery_required" || writes != 2 || c.Port != oldPort {
				t.Fatal(result, writes, c.Port)
			}
			if failure == "restart" && checks != 2 {
				t.Fatal("ownership check ran after failed restart", checks)
			}
			if failure == "ownership" && checks != 3 {
				t.Fatal("rollback ownership not checked", checks)
			}
			data, _ := json.Marshal(result)
			if strings.Contains(string(data), "RECOVERY_SECRET") {
				t.Fatal("recovery error leaked")
			}
		})
	}
}

func TestBasicSettingsUnprovenNextHasNoAdapterOrLockEffects(t *testing.T) {
	for _, marker := range []string{"next", "unknown", "stable"} {
		t.Run(marker, func(t *testing.T) {
			t.Setenv("AGENTDOCK_DESKTOP_VARIANT", marker)
			root := t.TempDir()
			if marker == "stable" {
				if err := os.WriteFile(filepath.Join(root, "agentdock.env"), []byte("AGENTDOCK_DESKTOP_VARIANT='next'\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			service := NewBasicSettingsService(root)
			service.read = func(context.Context, string) (desktopruntime.BasicSettings, error) {
				t.Fatal("unproven Next queried service")
				return desktopruntime.BasicSettings{}, nil
			}
			service.update = func(context.Context, string, desktopruntime.BasicSettings) error {
				t.Fatal("unproven Next changed service")
				return nil
			}
			if result := service.Read(context.Background()); result.Error == nil {
				t.Fatal(result)
			}
			if result := service.Save(context.Background(), BasicSettings{Port: 8767, LogLevel: "debug"}); result.Completed || result.Error == nil {
				t.Fatal(result)
			}
			if _, err := os.Lstat(filepath.Join(root, ".desktop-mutation.lock")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("lock written for unowned root")
			}
		})
	}
}

func TestConnectionQuickConfigurationAndLifecyclePhases(t *testing.T) {
	for _, running := range []bool{false, true} {
		for _, action := range []string{"configureTunnel", "updatePort", "start", "restart", "regenerate"} {
			t.Run(fmt.Sprintf("%v/%s", running, action), func(t *testing.T) {
				service, c := integratedConnectionFixture(t)
				adapter := service.run
				service.run = func(ctx context.Context, args []string, out, stderr io.Writer) error {
					if args[0] == "status" {
						return json.NewEncoder(out).Encode(desktopruntime.TunnelStatus{Observation: &desktopruntime.TunnelObservation{Running: &running}})
					}
					c.TunnelGeneration = "updated-generation"
					return adapter(ctx, args, out, stderr)
				}
				var result ConnectionActionResult
				switch action {
				case "configureTunnel":
					result = service.ConfigureTunnel(context.Background(), ConnectionTunnelRequest{Mode: "quick", ConfigRevision: ConnectionConfigRevision(*c)})
				case "updatePort":
					result = service.UpdatePort(context.Background(), ConnectionPortRequest{19000, ConnectionConfigRevision(*c)})
				default:
					result = service.Action(context.Background(), action, ConnectionConfigRevision(*c))
				}
				phase := "waiting_readiness"
				if (!running && (action == "configureTunnel" || action == "updatePort")) || (running && action == "start") {
					phase = "applied"
				}
				if !result.Completed || result.Phase != phase || result.ConfigRevision != ConnectionConfigRevision(*c) || result.TunnelGeneration != c.TunnelGeneration {
					t.Fatal(result)
				}
			})
		}
	}
}
