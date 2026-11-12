//go:build darwin

package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	"github.com/uvwt/agentdock/internal/permission"
)

func nextCodexAuthorizationFixture(t *testing.T) (*Runtime, config.ACPProfile, nextCodexMCPConfig, context.Context) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("AGENTDOCK_DESKTOP_VARIANT", "next")
	t.Setenv(nextCodexMCPRegistrationEnv, "source-only")
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
	// Missing isolation options deliberately prevent even a fixture Adapter
	// from starting after Allow. We test the uncertain-bootstrap reservation.
	profile := config.ACPProfile{ID: "codex", Kind: "codex", Enabled: true, Command: os.Args[0]}
	nextCodexCLIPrerequisite(t, &profile, filepath.Join(root, ".agentdock-next"))
	rt, err := NewRuntime(config.Config{AgentDockHome: filepath.Join(root, ".agentdock-next"), AgentDockDefaultDir: work, ACPEnabled: true, ACPProfiles: []config.ACPProfile{profile}, ACPDefaultProfile: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	if err := os.Chmod(rt.cfg.AgentDockHome, 0700); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "fixture")
	if err := os.WriteFile(exe, []byte("fixture-only"), 0700); err != nil {
		t.Fatal(err)
	}
	c := nextCodexMCPConfig{ServerID: "fixture", WrapperExecutable: exe, BackendExecutable: exe, BackendArgs: []string{"--token=secret-fixture"}}
	c.ConfigDigest = nextCodexMCPConfigDigest(c.ServerID, c.WrapperExecutable, c.BackendExecutable, c.BackendArgs)
	// Synthetic context is test input only; production must receive this from
	// a trusted authenticated request. No bootstrap identity is manufactured.
	ctx := requestmeta.WithAuthPrincipal(context.Background(), requestmeta.AuthPrincipal{Kind: "test", ID: "principal-a", Authenticated: true, Stable: true})
	return rt, profile, c, ctx
}

func nextCodexTestKey() string { return hex.EncodeToString(make([]byte, 32)) }

func assertNoNextCodexStartup(t *testing.T, rt *Runtime) {
	t.Helper()
	if rt.nextCodexStartupLock != nil || rt.nextCodexRegistry != nil || rt.nextCodexPin != nil {
		t.Fatal("denied request allocated startup resources")
	}
	if _, err := os.Lstat(filepath.Join(rt.cfg.AgentDockHome, nextCodexStartupLockName)); !os.IsNotExist(err) {
		t.Fatalf("denied request created a lock: %v", err)
	}
	if _, err := rt.acp.NextCodexAppServerBirths(context.Background(), make([]byte, 32)); err == nil {
		t.Fatal("denied request bootstrapped ACP")
	}
}

func TestNextCodexMCPAuthorizationRejectsUntrustedAndInvalidBeforeStartup(t *testing.T) {
	for _, action := range []string{"prepare", "attach"} {
		for _, name := range []string{"anonymous", "unstable", "unauthenticated", "payload-marker", "invalid-config", "invalid-server", "invalid-utf8", "nul-arg", "invalid-gate", "canceled"} {
			t.Run(action+"/"+name, func(t *testing.T) {
				rt, profile, c, ctx := nextCodexAuthorizationFixture(t)
				switch name {
				case "anonymous":
					ctx = context.Background()
				case "unstable":
					ctx = requestmeta.WithAuthPrincipal(context.Background(), requestmeta.AuthPrincipal{Kind: "test", ID: "a", Authenticated: true})
				case "unauthenticated":
					ctx = requestmeta.WithAuthPrincipal(context.Background(), requestmeta.AuthPrincipal{Kind: "test", ID: "a", Stable: true})
				case "payload-marker":
					ctx = context.WithValue(context.Background(), "authenticated", true)
					t.Setenv("AGENTDOCK_AUTHENTICATED", "true")
				case "invalid-config":
					c.ConfigDigest = strings.Repeat("0", 64)
				case "invalid-server":
					c.ServerID = strings.Repeat("a", 81)
					c.ConfigDigest = nextCodexMCPConfigDigest(c.ServerID, c.WrapperExecutable, c.BackendExecutable, c.BackendArgs)
				case "invalid-utf8", "nul-arg":
					if name == "invalid-utf8" {
						c.BackendArgs = []string{string([]byte{0xff})}
					} else {
						c.BackendArgs = []string{string([]byte{0})}
					}
					c.ConfigDigest = nextCodexMCPConfigDigest(c.ServerID, c.WrapperExecutable, c.BackendExecutable, c.BackendArgs)
				case "invalid-gate":
					rt.admission = nil
				case "canceled":
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				}
				var err error
				if action == "prepare" {
					_, err = rt.prepareNextCodexMCPRegistry(ctx, profile, nextCodexTestKey(), c)
				} else {
					err = rt.attachNextCodexInstrumentedMCP(ctx, c)
				}
				if err == nil {
					t.Fatal("untrusted input admitted")
				}
				assertNoNextCodexStartup(t, rt)
			})
		}
	}
}

func TestNextCodexMCPAuthorizationAskDenyAndRedactedBinding(t *testing.T) {
	for _, action := range []string{"prepare", "attach"} {
		for _, mode := range []string{permission.Rules, permission.ReadOnly} {
			t.Run(action+"/"+mode, func(t *testing.T) {
				rt, profile, c, ctx := nextCodexAuthorizationFixture(t)
				replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = mode })
				var err error
				if action == "prepare" {
					_, err = rt.prepareNextCodexMCPRegistry(ctx, profile, nextCodexTestKey(), c)
				} else {
					err = rt.attachNextCodexInstrumentedMCP(ctx, c)
				}
				code := "PERMISSION_DENIED"
				if mode == permission.Rules {
					code = "APPROVAL_REQUIRED"
				}
				requirePermissionError(t, err, code)
				assertNoNextCodexStartup(t, rt)
				if call := lastExecutionCall(t, rt); call.ErrorCode != code {
					t.Fatalf("missing M8 audit: %+v", call)
				}
				records, err := rt.permissions.History()
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(records)
				if strings.Contains(string(raw), "secret-fixture") || strings.Contains(string(raw), c.BackendExecutable) {
					t.Fatal("approval leaked credentials or paths")
				}
				if mode == permission.Rules {
					if len(records) != 1 || records[0].ID == "" {
						t.Fatalf("missing exact fingerprint: %+v", records)
					}
				}
			})
		}
	}
}

func approveNextCodexTestRequest(t *testing.T, rt *Runtime, err error) string {
	t.Helper()
	e := requirePermissionError(t, err, "APPROVAL_REQUIRED")
	id, _ := e.Details["approval_id"].(string)
	_, err = rt.permissions.ApproveOnce(context.Background(), permission.Mutation{ApprovalID: id, ApprovalVersion: permissionDetailUint64(t, e.Details, "approval_version"), PolicyRevision: permissionDetailUint64(t, e.Details, "policy_revision"), Actor: "fixture-control"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestNextCodexMCPApprovalBindsWrapperAndPrincipal(t *testing.T) {
	rt, profile, c, ctx := nextCodexAuthorizationFixture(t)
	replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Rules })
	_, err := rt.prepareNextCodexMCPRegistry(ctx, profile, nextCodexTestKey(), c)
	id := approveNextCodexTestRequest(t, rt, err)
	other := requestmeta.WithAuthPrincipal(context.Background(), requestmeta.AuthPrincipal{Kind: "test", ID: "principal-b", Authenticated: true, Stable: true})
	_, err = rt.prepareNextCodexMCPRegistry(other, profile, nextCodexTestKey(), c)
	requirePermissionError(t, err, "APPROVAL_REQUIRED")
	changed := c
	changed.WrapperExecutable = filepath.Join(filepath.Dir(c.WrapperExecutable), "other-wrapper")
	if err := os.WriteFile(changed.WrapperExecutable, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	changed.ConfigDigest = nextCodexMCPConfigDigest(changed.ServerID, changed.WrapperExecutable, changed.BackendExecutable, changed.BackendArgs)
	_, err = rt.prepareNextCodexMCPRegistry(ctx, profile, nextCodexTestKey(), changed)
	requirePermissionError(t, err, "APPROVAL_REQUIRED")
	assertNoNextCodexStartup(t, rt)
	_, err = rt.prepareNextCodexMCPRegistry(ctx, profile, nextCodexTestKey(), c)
	if err == nil || rt.nextCodexStartupLock == nil {
		t.Fatal("exact approved retry did not reserve before bounded bootstrap failure")
	}
	record, err := rt.permissions.Approval(id)
	if err != nil || record.Status != permission.Consumed || record.DispatchOutcome != permission.Failed {
		t.Fatalf("approval settlement: %+v %v", record, err)
	}
}

func TestNextCodexMCPReservationAcrossCoreInstancesAndClose(t *testing.T) {
	rt, profile, c, ctx := nextCodexAuthorizationFixture(t)
	replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Full })
	_, err := rt.prepareNextCodexMCPRegistry(ctx, profile, nextCodexTestKey(), c)
	if err == nil || rt.nextCodexStartupLock == nil {
		t.Fatalf("failed bootstrap must retain a durable reservation: %v", err)
	}
	path := filepath.Join(rt.cfg.AgentDockHome, nextCodexStartupLockName)
	state, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(state), "unresolved-v1") {
		t.Fatalf("missing durable crash sentinel: %q %v", state, err)
	}
	// Source-only Next permits exactly one permission writer, so a competing
	// Core is rejected at NewRuntime itself, before touching permissions.
	other, err := NewRuntime(rt.cfg)
	if err == nil {
		_ = other.Close()
		t.Fatal("concurrent Core unexpectedly loaded permission state")
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	other, err = NewRuntime(rt.cfg)
	if err != nil {
		t.Fatalf("authority lease was not released after Close: %v", err)
	}
	t.Cleanup(func() { _ = other.Close() })
	// Policy admission is allowed in the second Core but native bootstrap
	// must still reject the unresolved generation marker left by the first.
	replaceRuntimePermissionPolicy(t, other, func(p *permission.Policy) { p.GlobalMode = permission.Full })
	_, err = other.prepareNextCodexMCPRegistry(ctx, profile, nextCodexTestKey(), c)
	if err == nil || other.nextCodexStartupLock != nil {
		t.Fatal("restarted Core bypassed unresolved crash marker")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(state) != string(after) {
		t.Fatal("Runtime.Close cleared or modified unresolved marker")
	}
}

func TestNextCodexMCPRevisionAndShutdownFenceAfterAdmission(t *testing.T) {
	for _, name := range []string{"revision", "shutdown"} {
		t.Run(name, func(t *testing.T) {
			rt, profile, c, ctx := nextCodexAuthorizationFixture(t)
			replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Rules })
			_, err := rt.prepareNextCodexMCPRegistry(ctx, profile, nextCodexTestKey(), c)
			id := approveNextCodexTestRequest(t, rt, err)
			// Block dispatch, not admission. Observe exact grant consumption while
			// lifecycleMu is held: admission must never need this mutex.
			rt.lifecycleMu.Lock()
			done := make(chan error, 1)
			go func() { _, err := rt.prepareNextCodexMCPRegistry(ctx, profile, nextCodexTestKey(), c); done <- err }()
			deadline := time.Now().Add(5 * time.Second)
			consumed := false
			for time.Now().Before(deadline) {
				record, _ := rt.permissions.Approval(id)
				if record.Status == permission.Consumed {
					consumed = true
					break
				}
				time.Sleep(time.Millisecond)
			}
			if name == "revision" {
				replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Full })
			} else {
				rt.closing = true
			}
			rt.lifecycleMu.Unlock()
			if !consumed {
				t.Fatal("admission blocked on lifecycleMu")
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("stale or closing dispatch accepted")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("dispatch did not finish")
			}
			assertNoNextCodexStartup(t, rt)
		})
	}
}
