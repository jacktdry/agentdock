//go:build darwin

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uvwt/agentdock/internal/permission"
)

func TestNextNativeMCPIDOnlyRejectsBeforeAnyM8OrNativeStartup(t *testing.T) {
	for _, name := range []string{"empty-installed-catalog", "invalid-id", "nil-resolver", "missing-server", "mismatched-id", "mismatched-digest", "canceled"} {
		t.Run(name, func(t *testing.T) {
			rt, profile, original, ctx := nextCodexAuthorizationFixture(t)
			// Even permissive policy cannot convert missing/synthetic unsigned
			// source-runner configuration into Native MCP launch authority.
			replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Full })
			id := "fixture"
			resolve := func(string) (nextCodexMCPConfig, error) { return original, nil }
			switch name {
			case "empty-installed-catalog":
				// Production has an EMPTY signed catalog and the source test runner
				// has no signed NEXT bundle: never implicitly create a backend.
				resolve = resolveNextNativeMCPServer
			case "invalid-id":
				id = "../bin/sh"
			case "nil-resolver":
				resolve = nil
			case "missing-server":
				resolve = func(string) (nextCodexMCPConfig, error) { return nextCodexMCPConfig{}, errNextNativeMCPUnavailable }
			case "mismatched-id":
				resolve = func(string) (nextCodexMCPConfig, error) {
					v := original
					v.ServerID = "another"
					v.ConfigDigest = nextCodexMCPConfigDigest(v.ServerID, v.WrapperExecutable, v.BackendExecutable, v.BackendArgs)
					return v, nil
				}
			case "mismatched-digest":
				resolve = func(string) (nextCodexMCPConfig, error) {
					v := original
					v.ConfigDigest = "caller-spoofed"
					return v, nil
				}
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, err := rt.prepareNextNativeMCPRegistryResolved(ctx, profile, nextCodexTestKey(), id, resolve)
			if !errors.Is(err, errNextNativeMCPUnavailable) {
				t.Fatalf("prepare failed open or wrong reason: %v", err)
			}
			err = rt.attachNextNativeMCPResolved(ctx, id, resolve)
			if !errors.Is(err, errNextNativeMCPUnavailable) {
				t.Fatalf("attach failed open or wrong reason: %v", err)
			}
			if rt.nextNativeMCPPrepared != nil {
				t.Fatal("untrusted config was pinned")
			}
			assertNoNextCodexStartup(t, rt)
			if _, err := os.Lstat(filepath.Join(rt.cfg.AgentDockHome, nextCodexStartupLockName)); !os.IsNotExist(err) {
				t.Fatalf("untrusted request wrote journal: %v", err)
			}
		})
	}
}

func TestNextNativeMCPIDOnlyStillRequiresM8Authorization(t *testing.T) {
	for _, mode := range []string{permission.Rules, permission.ReadOnly} {
		t.Run(mode, func(t *testing.T) {
			rt, profile, c, ctx := nextCodexAuthorizationFixture(t)
			replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = mode })
			resolve := func(id string) (nextCodexMCPConfig, error) {
				if id != c.ServerID {
					return nextCodexMCPConfig{}, errNextNativeMCPUnavailable
				}
				return c, nil
			}
			_, err := rt.prepareNextNativeMCPRegistryResolved(ctx, profile, nextCodexTestKey(), c.ServerID, resolve)
			want := "PERMISSION_DENIED"
			if mode == permission.Rules {
				want = "APPROVAL_REQUIRED"
			}
			requirePermissionError(t, err, want)
			assertNoNextCodexStartup(t, rt)
			if rt.nextNativeMCPPrepared != nil {
				t.Fatal("M8-blocked prepare pinned a backend")
			}
			// Even a correct ID and signed catalog resolution are insufficient
			// to attach without the previously prepared Core/M8 generation.
			err = rt.attachNextNativeMCPResolved(ctx, c.ServerID, resolve)
			if !errors.Is(err, errNextNativeMCPUnavailable) {
				t.Fatalf("unprepared attach accepted: %v", err)
			}
		})
	}
}

func TestNextNativeMCPIDOnlyRechecksCatalogAfterM8Approval(t *testing.T) {
	rt, profile, c, ctx := nextCodexAuthorizationFixture(t)
	replaceRuntimePermissionPolicy(t, rt, func(p *permission.Policy) { p.GlobalMode = permission.Rules })
	calls := 0
	resolve := func(id string) (nextCodexMCPConfig, error) {
		calls++
		if id != c.ServerID {
			return nextCodexMCPConfig{}, errNextNativeMCPUnavailable
		}
		if calls <= 2 {
			return c, nil
		}
		changed := c
		changed.BackendArgs = []string{"--other-immutable-argument"}
		changed.ConfigDigest = nextCodexMCPConfigDigest(changed.ServerID, changed.WrapperExecutable, changed.BackendExecutable, changed.BackendArgs)
		return changed, nil
	}
	_, err := rt.prepareNextNativeMCPRegistryResolved(ctx, profile, nextCodexTestKey(), c.ServerID, resolve)
	id := approveNextCodexTestRequest(t, rt, err)
	_, err = rt.prepareNextNativeMCPRegistryResolved(ctx, profile, nextCodexTestKey(), c.ServerID, resolve)
	if !errors.Is(err, errNextNativeMCPUnavailable) {
		t.Fatalf("catalog changed after M8 Allow but was accepted: %v", err)
	}
	if calls != 3 {
		t.Fatalf("resolver calls = %d, expected initial, retry, and after-admission recheck", calls)
	}
	if rt.nextNativeMCPPrepared != nil {
		t.Fatal("changed catalog pinned after admission")
	}
	assertNoNextCodexStartup(t, rt)
	record, err := rt.permissions.Approval(id)
	if err != nil || record.Status != permission.Consumed || record.DispatchOutcome != permission.Failed {
		t.Fatalf("M8 consumed approval must be settled as failure: %+v %v", record, err)
	}
}
