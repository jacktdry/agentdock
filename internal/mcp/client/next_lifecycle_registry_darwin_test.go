//go:build darwin

package client

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

func p5NextGateFixture(t *testing.T) (*Manager, *NextStdioLeaseController, *NextStdioRegistryGate, string, string) {
	t.Helper()
	t.Setenv("NEXT_GATE_MCP_HELPER", "1")
	home := t.TempDir()
	manager, err := NewManager(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Errorf("close registry: %v", err)
		}
	})
	_, err = manager.Add(ServerConfig{Name: "next-p5-echo", Description: "P5 checked native echo", Enabled: true, Transport: TransportStdio, Command: os.Args[0], Args: []string{"-test.run=^TestMCPStdioHelperProcess$"}, EnvFromEnv: map[string]string{"GO_WANT_MCP_HELPER": "NEXT_GATE_MCP_HELPER"}, TimeoutMS: 4000})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := manager.Registry()
	if err != nil {
		t.Fatal(err)
	}
	controller := nextCtl(t, 2)
	bridge, err := NewNextStdioRegistryGate(manager, controller)
	if err != nil {
		t.Fatal(err)
	}
	return manager, controller, bridge, registry.Revision, registry.Servers["next-p5-echo"].Generation
}

func TestNextRegistryGateAuthoritativeTwentyCycles(t *testing.T) {
	_, ctrl, g, rev, gen := p5NextGateFixture(t)
	for n := 0; n < 20; n++ {
		lease, err := g.AcquireChecked(context.Background(), "next-session-one", "next-p5-echo", rev, gen, NextLeaseEphemeral, 0)
		if err != nil {
			t.Fatalf("cycle %d: %v", n, err)
		}
		got, err := g.CallChecked(context.Background(), lease, "echo", map[string]any{"text": "ok"})
		if err != nil || got["structuredContent"].(map[string]any)["echo"] != "ok" {
			t.Fatalf("cycle %d call: %#v %v", n, got, err)
		}
		if err := g.Release(context.Background(), lease); err != nil {
			t.Fatalf("cycle %d release: %v", n, err)
		}
		if ctrl.Snapshot().Managed != 0 || ctrl.Snapshot().CleanupRequired != 0 {
			t.Fatalf("cycle %d leaked: %+v", n, ctrl.Snapshot())
		}
	}
}

func TestNextRegistryGateBlocksForgedAndRevokedAuthority(t *testing.T) {
	manager, ctrl, g, rev, gen := p5NextGateFixture(t)
	cases := []struct{ name, rev, gen string }{
		{"not-found", rev, gen}, {"next-p5-echo", "false-revision", gen}, {"next-p5-echo", rev, "false-generation"},
	}
	for _, tc := range cases {
		if _, err := g.AcquireChecked(context.Background(), "owner", tc.name, tc.rev, tc.gen, NextLeasePersistent, 0); !errors.Is(err, errNextLeaseDenied) {
			t.Fatalf("forged registry case allowed %+v", tc)
		}
	}
	if ctrl.Snapshot().Managed != 0 {
		t.Fatal("invalid authority launched MCP")
	}
	lease, err := g.AcquireChecked(context.Background(), "owner", "next-p5-echo", rev, gen, NextLeasePersistent, 0)
	if err != nil {
		t.Fatal(err)
	}
	change, err := manager.SetEnabledChecked("next-p5-echo", false, rev, gen)
	if err != nil || !change.Persisted {
		t.Fatalf("disable P5 failed: %+v %v", change, err)
	}
	if result, err := g.CallChecked(context.Background(), lease, "echo", map[string]any{"text": "must-not-call"}); !errors.Is(err, errNextLeaseDenied) || result != nil {
		t.Fatalf("disabled tool dispatched: %v %+v", err, result)
	}
	if ctrl.Snapshot().CleanupRequired != 1 {
		t.Fatalf("stale owned process not quarantined: %+v", ctrl.Snapshot())
	}
	if err := g.Release(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	if ctrl.Snapshot().Managed != 0 {
		t.Fatalf("revoked process not reaped: %+v", ctrl.Snapshot())
	}
	if _, err := g.AcquireChecked(context.Background(), "owner", "next-p5-echo", rev, gen, NextLeaseEphemeral, 0); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("old revision created a new child")
	}
	latest, err := manager.Registry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.AcquireChecked(context.Background(), "owner", "next-p5-echo", latest.Revision, latest.Servers["next-p5-echo"].Generation, NextLeaseEphemeral, 0); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("disabled latest revision created child")
	}
}

func TestNextRegistryGateDisallowsNonStdioOrPlugin(t *testing.T) {
	mgr, ctrl, g, rev, gen := p5NextGateFixture(t)
	_, err := mgr.Add(ServerConfig{Name: "http-only", Description: "HTTP only", Enabled: true, Transport: TransportStreamableHTTP, URL: "https://example.com/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := mgr.Registry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.AcquireChecked(context.Background(), "owner", "http-only", updated.Revision, updated.Servers["http-only"].Generation, NextLeasePersistent, 0); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("HTTP converted into owned stdio")
	}
	if _, err := g.AcquireChecked(context.Background(), "owner", "next-p5-echo", rev, gen, NextLeasePersistent, 0); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("old revision accepted after unrelated P5 mutation")
	}
	if ctrl.Snapshot().Managed != 0 {
		t.Fatalf("unauthorized child launched: %+v", ctrl.Snapshot())
	}
}

func TestNextRegistryGateParallelNoDuplicateOwnedServer(t *testing.T) {
	_, ctrl, g, rev, gen := p5NextGateFixture(t)
	const contenders = 8
	start := make(chan struct{})
	acquired := make(chan NextRegistryLease, contenders)
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			lease, err := g.AcquireChecked(context.Background(), "same-owner", "next-p5-echo", rev, gen, NextLeasePersistent, 0)
			if err == nil {
				acquired <- lease
			}
		}()
	}
	close(start)
	wg.Wait()
	close(acquired)
	seen := make([]NextRegistryLease, 0)
	for lease := range acquired {
		seen = append(seen, lease)
	}
	if len(seen) == 0 {
		t.Fatal("all parallel P5 acquisitions denied")
	}
	if ctrl.Snapshot().Managed != 1 {
		t.Fatalf("same owner duplicated process: %+v", ctrl.Snapshot())
	}
	for _, lease := range seen {
		if err := g.Release(context.Background(), lease); err != nil {
			t.Fatal(err)
		}
	}
	if err := ctrl.Drain(context.Background(), "same-owner", "next-p5-echo", gen); err != nil {
		t.Fatal(err)
	}
	if ctrl.Snapshot().Managed != 0 {
		t.Fatal(ctrl.Snapshot())
	}
}

func TestNextRegistryGateRealConfigRevocation(t *testing.T) {
	mgr, ctrl, g, rev, gen := p5NextGateFixture(t)
	lease, err := g.AcquireChecked(context.Background(), "owner", "next-p5-echo", rev, gen, NextLeasePersistent, 0)
	if err != nil {
		t.Fatal(err)
	}
	// A different revision/generation cannot masquerade as original through the
	// same name, even when a tool already has a borrowed stdio connection.
	_, err = mgr.Update("next-p5-echo", ServerConfig{Name: "next-p5-echo", Description: "P5 checked native echo", Enabled: true, Transport: TransportStdio, Command: os.Args[0], Args: []string{"-test.run=^TestMCPStdioHelperProcess$"}, EnvFromEnv: map[string]string{"GO_WANT_MCP_HELPER": "NEXT_GATE_MCP_HELPER"}, TimeoutMS: 4000}, rev, gen)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.CallChecked(context.Background(), lease, "echo", map[string]any{"text": "denied"}); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("old generation executed")
	}
	if err := g.Release(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	if ctrl.Snapshot().Managed != 0 {
		t.Fatalf("old instance retained: %+v", ctrl.Snapshot())
	}
}

func TestNextRegistryGateNoHiddenNetworkInSnapshot(t *testing.T) {
	mgr, _, g, rev, gen := p5NextGateFixture(t)
	cfg, err := g.checkedConfig("next-p5-echo", rev, gen)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Command != os.Args[0] || cfg.SourceType != "standalone" || cfg.Generation != gen {
		t.Fatal("not based on authoritative P5 registry")
	}
	before := time.Now()
	for i := 0; i < 20; i++ {
		_, err = g.checkedConfig("next-p5-echo", rev, gen)
		if err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(before) > 5*time.Second {
		t.Fatal("registry check unexpectedly slow")
	}
	_ = mgr
}

// Deterministic revocation between successful MCP handshake and publication.
// This rejects the old authority, reaps ONLY the child created by this gate,
// and never publishes a NextRegistryLease capability.
func TestNextRegistryGateRevocationDuringHandshakePublication(t *testing.T) {
	manager, ctl, gate, rev, gen := p5NextGateFixture(t)
	entered := make(chan struct{})
	resume := make(chan struct{})
	gate.postHandshakeHook = func() { close(entered); <-resume }
	type acquireResult struct {
		lease NextRegistryLease
		err   error
	}
	result := make(chan acquireResult, 1)
	go func() {
		lease, err := gate.AcquireChecked(context.Background(), "owner", "next-p5-echo", rev, gen, NextLeasePersistent, 0)
		result <- acquireResult{lease, err}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(resume)
		t.Fatal("handshake not completed")
	}
	snapshot := ctl.Snapshot()
	if snapshot.Managed != 1 || snapshot.Ready != 1 {
		close(resume)
		t.Fatalf("candidate must be spawned: %+v", snapshot)
	}
	changed, err := manager.SetEnabledChecked("next-p5-echo", false, rev, gen)
	if err != nil || !changed.Persisted {
		close(resume)
		t.Fatalf("revocation failed: %+v %v", changed, err)
	}
	close(resume)
	select {
	case r := <-result:
		if !errors.Is(r.err, errNextLeaseDenied) || r.lease.lease.id != "" || r.lease.revision != "" {
			t.Fatalf("stale authority returned lease: %+v %v", r.lease, r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("stale cleanup blocked")
	}
	if snap := ctl.Snapshot(); snap.Managed != 0 || snap.CleanupRequired != 0 {
		t.Fatalf("owned candidate not closed: %+v", snap)
	}
}
