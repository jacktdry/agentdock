package acp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

func TestB5InstrumentedMCPBorrowedAcrossParallelSessionsAndGracefulRelease(t *testing.T) {
	server := SessionMCPServer{Name: "instrumented", Command: "/opt/next/wrapper", Args: []string{"--", "/opt/next/test-mcp"}, Env: []SessionMCPHeader{{Name: "AGENTDOCK_NEXT_MCP_LAUNCH_CORE_IDENTITY", Value: "{}"}}}
	provider, err := NewInstrumentedSessionProvider(nil, server)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const count = 8
	var wg sync.WaitGroup
	errCh := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "test-" + string(rune('A'+i))
			servers, err := provider.Servers(ctx, id, "/tmp", nil)
			if err != nil || len(servers) != 1 {
				errCh <- errors.New("instrumented server missing")
				return
			}
			data, _ := json.Marshal(servers[0])
			var m map[string]any
			_ = json.Unmarshal(data, &m)
			if _, hasType := m["type"]; hasType {
				errCh <- errors.New("native stdio server illegally has type")
			} else if m["command"] != "/opt/next/wrapper" {
				errCh <- errors.New("wrong wrapper command")
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	if got := provider.Borrowers(); got != count {
		t.Fatalf("borrowers=%d", got)
	}
	for i := 0; i < count-1; i++ {
		id := "test-" + string(rune('A'+i))
		if err := provider.ReleaseSession(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if got := provider.Borrowers(); got != 1 {
		t.Fatalf("prematurely released shared MCP: %d", got)
	}
	if err := provider.ReleaseSession(ctx, "test-H"); err != nil {
		t.Fatal(err)
	}
	if got := provider.Borrowers(); got != 0 {
		t.Fatalf("leaked logical lease: %d", got)
	}
	if err := provider.ReleaseSession(ctx, "test-H"); err != nil || provider.Borrowers() != 0 {
		t.Fatalf("idempotent release failed: %v", err)
	}
}

func TestB5SourceOnlyMCPInstallationMustBeBeforeSessions(t *testing.T) {
	ctx := context.Background()
	server := SessionMCPServer{Name: "instrumented", Command: "/opt/next/wrapper", Args: []string{"--", "/opt/next/test-mcp"}}
	mgr := &Manager{sessions: map[string]SessionRecord{}, loaded: map[string]sessionLifecycleResponse{}, runs: map[string]*Run{}, sessionMCPReleased: make(map[string]struct{})}
	mgr.opts.Agent.Environment = map[string]string{"AGENTDOCK_NEXT_CODEX_MCP_ISOLATION": "source-only"}
	if err := mgr.InstallInstrumentedSessionMCP(server); err != nil {
		t.Fatal(err)
	}
	if err := mgr.InstallInstrumentedSessionMCP(server); err == nil {
		t.Fatal("second write replaced instrumentation")
	}
	mcp, err := mgr.sessionMCPServers(ctx, "test-1", "/tmp", nil)
	if err != nil || len(mcp) != 1 {
		t.Fatalf("no session injection: %#v %v", mcp, err)
	}
	if err := mgr.releaseSessionMCP(ctx, "test-1"); err != nil {
		t.Fatal(err)
	}
	if mgr.InstrumentedSessionMCPBorrowers() != 0 {
		t.Fatal("released logical lease still counted")
	}
	mgr.sessions["existing"] = SessionRecord{ID: "existing"}
	// No mutation is permitted after a Session was created even if closed later.
	if err := mgr.InstallInstrumentedSessionMCP(server); err == nil {
		t.Fatal("installed into active session generation")
	}
}
