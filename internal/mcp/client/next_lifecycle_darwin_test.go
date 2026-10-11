//go:build darwin

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func ownedNextFixture(gen string) ServerConfig {
	return ServerConfig{Name: "mcp-echo", Generation: gen, SourceType: "standalone", Enabled: true, Transport: TransportStdio, Command: os.Args[0], Args: []string{"-test.run=^TestMCPStdioHelperProcess$"}, StaticEnv: map[string]string{"GO_WANT_MCP_HELPER": "1"}, TimeoutMS: 5000}
}
func nextCtl(t *testing.T, n int) *NextStdioLeaseController {
	t.Helper()
	c, e := NewNextStdioLeaseController(n)
	if e != nil {
		t.Fatal(e)
	}
	return c
}

func TestNextStdioLifecycleTwentyOwnedCycles(t *testing.T) {
	c := nextCtl(t, 2)
	for i := 0; i < 20; i++ {
		cfg := ownedNextFixture(fmt.Sprintf("registry-generation-%d", i))
		lease, err := c.Acquire(context.Background(), "next-managed-session-1", cfg, NextLeaseEphemeral, 0)
		if err != nil {
			t.Fatalf("cycle %d: open denied: %v", i, err)
		}
		before := c.Snapshot()
		if before.Namespace != "next" || before.Managed != 1 || before.Borrowers != 1 || before.Ready != 1 {
			t.Fatalf("cycle %d: %+v", i, before)
		}
		c.mu.Lock()
		owned := c.resources[lease.key]
		cmd := owned.session.client.command
		c.mu.Unlock()
		got, err := c.Call(context.Background(), lease, "echo", map[string]any{"text": "hello"})
		if err != nil || got["structuredContent"].(map[string]any)["echo"] != "hello" {
			t.Fatalf("call: %+v %v", got, err)
		}
		if err := c.Release(context.Background(), lease); err != nil {
			t.Fatalf("release: %v", err)
		}
		if cmd.ProcessState == nil {
			t.Fatalf("cycle %d: owned process was not waited", i)
		}
		after := c.Snapshot()
		if after.Managed != 0 || after.Ready != 0 || after.Borrowers != 0 || after.Inflight != 0 || after.CleanupRequired != 0 {
			t.Fatalf("cycle %d leaked: %+v", i, after)
		}
		if _, err := c.Call(context.Background(), lease, "echo", nil); !errors.Is(err, errNextLeaseDenied) {
			t.Fatal("released token accepted")
		}
		if err := c.Release(context.Background(), lease); !errors.Is(err, errNextLeaseDenied) {
			t.Fatal("replayed release accepted")
		}
	}
}

func TestNextStdioLeaseBorrowPolicyGenerationAndCapacity(t *testing.T) {
	c := nextCtl(t, 1)
	cfg := ownedNextFixture("rev-one")
	first, err := c.Acquire(context.Background(), "session-a", cfg, NextLeasePersistent, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Acquire(context.Background(), "session-a", cfg, NextLeasePersistent, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.id == second.id {
		t.Fatal("duplicate borrower token")
	}
	if c.Snapshot().Managed != 1 || c.Snapshot().Borrowers != 2 {
		t.Fatal(c.Snapshot())
	}
	mutated := cfg
	mutated.Generation = "rev-two"
	attempts := []struct {
		owner  string
		cfg    ServerConfig
		policy NextLeasePolicy
		ttl    time.Duration
	}{
		{"session-a", mutated, NextLeasePersistent, 0},
		{"session-b", cfg, NextLeasePersistent, 0},
		{"session-a", cfg, NextLeaseEphemeral, 0},
		{"session-a", cfg, NextLeasePersistent, time.Second},
	}
	for _, x := range attempts {
		if _, err := c.Acquire(context.Background(), x.owner, x.cfg, x.policy, x.ttl); !errors.Is(err, errNextLeaseDenied) {
			t.Fatalf("bad acquire allowed: %+v", x)
		}
	}
	if err := c.Drain(context.Background(), "session-a", cfg.Name, cfg.Generation); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("drained borrowed resource")
	}
	if err := c.Release(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().Managed != 1 || c.Snapshot().Borrowers != 1 {
		t.Fatal(c.Snapshot())
	}
	if _, err := c.Call(context.Background(), first, "echo", nil); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("released token called")
	}
	if err := c.Release(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().Ready != 1 || c.Snapshot().Borrowers != 0 {
		t.Fatal("persistent ended on prompt terminal")
	}
	if _, err := c.Acquire(context.Background(), "session-a", mutated, NextLeasePersistent, 0); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("stale-generation reused session")
	}
	if err := c.Drain(context.Background(), "session-a", cfg.Name, "wrong-generation"); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("wrong-generation drained")
	}
	if err := c.Drain(context.Background(), "session-a", cfg.Name, cfg.Generation); err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().Managed != 0 {
		t.Fatal(c.Snapshot())
	}
	next, err := c.Acquire(context.Background(), "session-a", mutated, NextLeaseEphemeral, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Release(context.Background(), next); err != nil {
		t.Fatal(err)
	}
}

func TestNextStdioLeaseIdleManagedAndCloseFence(t *testing.T) {
	c := nextCtl(t, 1)
	cfg := ownedNextFixture("idle-rev")
	lease, err := c.Acquire(context.Background(), "session-a", cfg, NextLeaseIdleManaged, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if count := c.ReapIdle(context.Background(), time.Now().Add(48*time.Hour)); count != 0 {
		t.Fatal("borrower reaped")
	}
	if err := c.Release(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	if count := c.ReapIdle(context.Background(), time.Now()); count != 0 {
		t.Fatal("TTL not expired but reaped")
	}
	if count := c.ReapIdle(context.Background(), time.Now().Add(2*time.Second)); count != 1 {
		t.Fatalf("reaped = %d", count)
	}
	if c.Snapshot().Managed != 0 {
		t.Fatal(c.Snapshot())
	}
	// Inflight tool work must settle before drain. Close is exclusive; new work
	// and new borrowers are fenced even when a drain is waiting.
	lease, err = c.Acquire(context.Background(), "session-a", cfg, NextLeasePersistent, 0)
	if err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	rec := c.resources[lease.key]
	rec.inflight++
	rec.signal()
	c.mu.Unlock()
	if err := c.Release(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- c.Drain(context.Background(), "session-a", cfg.Name, cfg.Generation) }()
	deadline := time.Now().Add(time.Second)
	for c.Snapshot().Draining == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.Snapshot().Draining != 1 {
		t.Fatal("drain not entered")
	}
	if _, err := c.Acquire(context.Background(), "session-a", cfg, NextLeasePersistent, 0); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("borrow during drain")
	}
	select {
	case <-done:
		t.Fatal("drain closed inflight process")
	case <-time.After(30 * time.Millisecond):
	}
	c.mu.Lock()
	rec.inflight--
	rec.signal()
	c.mu.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("drain did not finish")
	}
	if c.Snapshot().Managed != 0 {
		t.Fatal(c.Snapshot())
	}
}

func TestNextStdioLeaseInvalidAuthorityAndRecovery(t *testing.T) {
	c := nextCtl(t, 2)
	cfg := ownedNextFixture("rev")
	candidates := []ServerConfig{cfg, cfg, cfg, cfg, cfg, cfg}
	candidates[1].SourceType = "plugin"
	candidates[2].PluginName = "plugins-A"
	candidates[3].Generation = ""
	candidates[4].Enabled = false
	candidates[5].Transport = TransportStreamableHTTP
	for i, wrong := range candidates[1:] {
		if _, err := c.Acquire(context.Background(), "next-tenant", wrong, NextLeaseEphemeral, 0); !errors.Is(err, errNextLeaseDenied) {
			t.Fatalf("invalid authority %d accepted", i)
		}
	}
	if _, err := c.Acquire(context.Background(), "next-tenant", cfg, NextLeaseIdleManaged, 0); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("idle no TTL accepted")
	}
	if _, err := c.Acquire(context.Background(), "next-tenant", cfg, NextLeaseEphemeral, time.Second); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("ephemeral nonzero TTL accepted")
	}
	if _, err := c.Acquire(context.Background(), "", cfg, NextLeaseEphemeral, 0); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("unknown owner accepted")
	}
	if _, err := c.Acquire(context.Background(), "bad\x00owner", cfg, NextLeaseEphemeral, 0); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("nonportable owner accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Acquire(canceled, "next-tenant", cfg, NextLeaseEphemeral, 0); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("canceled owner accepted")
	}
	if c.Snapshot().Managed != 0 {
		t.Fatal("invalid acquisition started a child")
	}
}

func TestNextStdioLeaseParallelBoundedCapacity(t *testing.T) {
	c := nextCtl(t, 4)
	cfg := ownedNextFixture("same-config")
	var successes atomic.Int32
	start := make(chan struct{})
	out := make(chan NextStdioLease, 8)
	failures := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			lease, err := c.Acquire(context.Background(), fmt.Sprintf("owner-%d", i), cfg, NextLeasePersistent, 0)
			if err != nil {
				failures <- err
				return
			}
			successes.Add(1)
			out <- lease
		}(i)
	}
	close(start)
	wg.Wait()
	close(out)
	close(failures)
	if successes.Load() != 4 {
		t.Fatalf("unbounded Next worker concurrency: %d", successes.Load())
	}
	for err := range failures {
		if !errors.Is(err, errNextLeaseDenied) {
			t.Fatalf("untyped error: %v", err)
		}
	}
	if c.Snapshot().Managed != 4 {
		t.Fatalf("capacity exceeded: %+v", c.Snapshot())
	}
	for lease := range out {
		if err := c.Release(context.Background(), lease); err != nil {
			t.Fatal(err)
		}
		owner, server, found := strings.Cut(lease.key, "\x00")
		if !found {
			t.Fatal("bad test key")
		}
		if err := c.Drain(context.Background(), owner, server, lease.generation); err != nil {
			t.Fatal(err)
		}
	}
	if c.Snapshot().Managed != 0 {
		t.Fatal(c.Snapshot())
	}
}

// The helper emits invalid JSON, never performing an MCP initialize. Once
// Start and PID birth witness succeeded, even failed cleanup is quarantined
// rather than pretending that spawning an exact second server is safe.
func TestNextBrokenMCPHelper(t *testing.T) {
	if os.Getenv("GO_WANT_NEXT_BROKEN") != "1" {
		return
	}
	_, _ = os.Stdout.Write([]byte("not valid json\n"))
	time.Sleep(120 * time.Millisecond)
}

func TestNextStdioFailedHandshakeQuarantinesObservedSpawn(t *testing.T) {
	c := nextCtl(t, 1)
	cfg := ownedNextFixture("broken-rev")
	cfg.Args = []string{"-test.run=^TestNextBrokenMCPHelper$"}
	cfg.StaticEnv = map[string]string{"GO_WANT_NEXT_BROKEN": "1"}
	lease, err := c.Acquire(context.Background(), "session-a", cfg, NextLeaseEphemeral, 0)
	if !errors.Is(err, errNextLeaseDenied) || lease != (NextStdioLease{}) {
		t.Fatalf("invalid handshake admitted: %+v %v", lease, err)
	}
	if c.Snapshot().CleanupRequired != 1 || c.Snapshot().Managed != 1 {
		t.Fatalf("ambiguous started child not quarantined: %+v", c.Snapshot())
	}
	if _, err := c.Acquire(context.Background(), "session-a", cfg, NextLeaseEphemeral, 0); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("retry spawned second process")
	}
	if err := c.Drain(context.Background(), "session-a", cfg.Name, cfg.Generation); !errors.Is(err, errNextLeaseDenied) {
		t.Fatal("unknown cleanup owner falsely reaped")
	}
}

func TestNextStdioTwentyPersistentCloseResumeCycles(t *testing.T) {
	c := nextCtl(t, 2)
	cfg := ownedNextFixture("same-registry-revision")
	for i := 0; i < 20; i++ {
		lease, err := c.Acquire(context.Background(), "session-a", cfg, NextLeasePersistent, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Call(context.Background(), lease, "echo", map[string]any{"text": "resume"}); err != nil {
			t.Fatal(err)
		}
		if err := c.Release(context.Background(), lease); err != nil {
			t.Fatal(err)
		}
		if c.Snapshot().Managed != 1 || c.Snapshot().Ready != 1 {
			t.Fatal("persistent terminated early")
		}
		if err := c.Drain(context.Background(), "session-a", cfg.Name, cfg.Generation); err != nil {
			t.Fatal(err)
		}
		if c.Snapshot().Managed != 0 || c.Snapshot().CleanupRequired != 0 {
			t.Fatalf("cycle %d: %+v", i, c.Snapshot())
		}
	}
}

func TestNextStdioDiagnosticProjectionNoSecrets(t *testing.T) {
	c := nextCtl(t, 2)
	cfg := ownedNextFixture("CANARY-REGISTRY-CAPABILITY-SECRET")
	cfg.Args = []string{"-test.run=^TestMCPStdioHelperProcess$", "CANARY-ARGV-SECRET"}
	// Do not actually launch a helper with invalid argv; a synthetic metadata-only
	// starting state is sufficient to prove redaction of the pure DTO projection.
	c.mu.Lock()
	rec := &nextResource{key: nextKey("next-session-A", "safe-server"), cfg: cfg, generation: cfg.Generation, policy: NextLeasePersistent, state: "starting", borrowers: map[string]bool{}, changed: make(chan struct{})}
	c.resources[rec.key] = rec
	c.mu.Unlock()
	got := c.ResourceSnapshot()
	if len(got) != 1 || got[0].Namespace != "next" || got[0].State != "starting" || got[0].OwnerSessionID != "next-session-A" {
		t.Fatalf("incorrect projection: %+v", got)
	}
	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, str := range []string{"CANARY", "argv", "command", "generation", "process_birth", "pid", "StaticEnv", "token", "tool_arguments", "url"} {
		if strings.Contains(string(wire), str) {
			t.Fatalf("secret/authority leaked %s: %s", str, wire)
		}
	}
	c.mu.Lock()
	rec.state = "cleanup_required"
	rec.signal()
	c.mu.Unlock()
	got = c.ResourceSnapshot()
	if !got[0].CleanupRequired || got[0].State != "cleanup_required" {
		t.Fatal("quarantine diagnostics missing")
	}
}
