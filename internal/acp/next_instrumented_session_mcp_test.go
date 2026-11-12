package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
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

type instrumentedHostProvider struct {
	servers func(context.Context, string) ([]SessionMCPServer, error)
	release func(context.Context, string) error
}

func (p instrumentedHostProvider) Servers(ctx context.Context, id, _ string, _ []string) ([]SessionMCPServer, error) {
	return p.servers(ctx, id)
}

func (p instrumentedHostProvider) ReleaseSession(ctx context.Context, id string) error {
	return p.release(ctx, id)
}

func TestInstrumentedMCPCollisionRollback(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		for _, rollbackFails := range []bool{false, true} {
			t.Run(fmt.Sprintf("cancelled=%t/failure=%t", cancelled, rollbackFails), func(t *testing.T) {
				failRelease := rollbackFails
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				rollbackErr := errors.New("host lease still live")
				leases := map[string]bool{"unrelated": true}
				allocations, releases := 0, 0
				host := instrumentedHostProvider{
					servers: func(_ context.Context, stringID string) ([]SessionMCPServer, error) {
						allocations++
						leases[stringID] = true
						if cancelled {
							cancel()
						}
						return []SessionMCPServer{{Name: "instrumented"}}, nil
					},
					release: func(cleanupCtx context.Context, id string) error {
						releases++
						if id != "failed" || cleanupCtx.Err() != nil {
							t.Fatalf("unsafe rollback: id=%q err=%v", id, cleanupCtx.Err())
						}
						if releases == 1 {
							deadline, ok := cleanupCtx.Deadline()
							if !ok || time.Until(deadline) > 35*time.Second {
								t.Fatal("rollback must be bounded independently of cancellation")
							}
						}
						if failRelease {
							return rollbackErr
						}
						delete(leases, id)
						return nil
					},
				}
				provider, err := NewInstrumentedSessionProvider(host, SessionMCPServer{Name: "instrumented", Command: "/wrapper", Args: []string{"/mcp"}})
				if err != nil {
					t.Fatal(err)
				}
				// A previous successful close must not hide a failed new allocation.
				mgr := &Manager{sessionMCPReleased: map[string]struct{}{"failed": {}}}
				mgr.opts.SessionMCPProvider = provider
				servers, err := mgr.sessionMCPServers(ctx, "failed", "/tmp", nil)
				if servers != nil || !errors.Is(err, errInstrumentedSessionMCPUnavailable) || releases != 1 {
					t.Fatalf("collision: servers=%v err=%v releases=%d", servers, err, releases)
				}
				if rollbackFails {
					if !errors.Is(err, rollbackErr) || provider.Borrowers() != 1 || !leases["failed"] {
						t.Fatalf("rollback failure hidden: err=%v borrowers=%d leases=%v", err, provider.Borrowers(), leases)
					}
					// Retrying preparation cannot allocate over an unresolved rollback.
					if _, err := provider.Servers(context.Background(), "failed", "/tmp", nil); err == nil || allocations != 1 {
						t.Fatalf("pending rollback retried allocation: err=%v allocations=%d", err, allocations)
					}
					if err := mgr.releaseSessionMCP(context.Background(), "failed"); !errors.Is(err, rollbackErr) || provider.Borrowers() != 1 {
						t.Fatalf("release failure falsely reported success: %v", err)
					}
					failRelease = false
					if err := mgr.releaseSessionMCP(context.Background(), "failed"); err != nil {
						t.Fatal(err)
					}
				}
				if provider.Borrowers() != 0 || leases["failed"] || !leases["unrelated"] || len(leases) != 1 {
					t.Fatalf("incorrect cleanup scope: borrowers=%d leases=%v", provider.Borrowers(), leases)
				}
			})
		}
	}
}

func TestInstrumentedMCPCollisionPreservesAcceptedSessionOnRetry(t *testing.T) {
	leases := make(map[string]bool)
	collision := false
	releases := 0
	host := instrumentedHostProvider{
		servers: func(_ context.Context, id string) ([]SessionMCPServer, error) {
			leases[id] = true
			name := "host"
			if collision {
				name = "instrumented"
			}
			return []SessionMCPServer{{Name: name}}, nil
		},
		release: func(_ context.Context, id string) error {
			releases++
			delete(leases, id)
			return nil
		},
	}
	provider, err := NewInstrumentedSessionProvider(host, SessionMCPServer{Name: "instrumented", Command: "/wrapper", Args: []string{"/mcp"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"accepted", "unrelated"} {
		if _, err := provider.Servers(context.Background(), id, "/tmp", nil); err != nil {
			t.Fatal(err)
		}
	}
	collision = true
	if servers, err := provider.Servers(context.Background(), "accepted", "/tmp", nil); servers != nil || !errors.Is(err, errInstrumentedSessionMCPUnavailable) {
		t.Fatalf("collision accepted: %v %v", servers, err)
	}
	if releases != 0 || provider.Borrowers() != 2 || !leases["accepted"] || !leases["unrelated"] {
		t.Fatalf("retry interrupted accepted sessions: releases=%d leases=%v", releases, leases)
	}
	if err := provider.ReleaseSession(context.Background(), "accepted"); err != nil {
		t.Fatal(err)
	}
	if provider.Borrowers() != 1 || len(leases) != 1 || !leases["unrelated"] {
		t.Fatalf("unrelated session affected: leases=%v", leases)
	}
}

func TestInstrumentedMCPCollisionParallelSessions(t *testing.T) {
	var mu sync.Mutex
	leases := make(map[string]bool)
	host := instrumentedHostProvider{
		servers: func(_ context.Context, id string) ([]SessionMCPServer, error) {
			mu.Lock()
			leases[id] = true
			mu.Unlock()
			if id == "unrelated" {
				return []SessionMCPServer{{Name: "host"}}, nil
			}
			return []SessionMCPServer{{Name: "instrumented"}}, nil
		},
		release: func(_ context.Context, id string) error {
			mu.Lock()
			delete(leases, id)
			mu.Unlock()
			return nil
		},
	}
	provider, err := NewInstrumentedSessionProvider(host, SessionMCPServer{Name: "instrumented", Command: "/wrapper", Args: []string{"/mcp"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Servers(context.Background(), "unrelated", "/tmp", nil); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() {
			// Contend on both reused and distinct failed bootstrap identities.
			for _, id := range []string{"shared-failure", fmt.Sprintf("failed-%d", i)} {
				if servers, err := provider.Servers(context.Background(), id, "/tmp", nil); servers != nil || !errors.Is(err, errInstrumentedSessionMCPUnavailable) {
					t.Errorf("collision accepted: %v %v", servers, err)
				}
			}
		})
	}
	wg.Wait()
	if provider.Borrowers() != 1 || len(leases) != 1 || !leases["unrelated"] {
		t.Fatalf("parallel rollback affected ownership: borrowers=%d leases=%v", provider.Borrowers(), leases)
	}
	if len(provider.sessionLocks) != 0 {
		t.Fatal("completed operations retained session locks")
	}
}

func TestInstrumentedMCPBlockedSessionDoesNotInterruptOthers(t *testing.T) {
	for _, phase := range []string{"servers", "rollback", "release"} {
		t.Run(phase, func(t *testing.T) {
			blocked, resume := make(chan struct{}), make(chan struct{})
			var resumeOnce sync.Once
			var mu sync.Mutex
			leases := make(map[string]bool)
			host := instrumentedHostProvider{
				servers: func(_ context.Context, id string) ([]SessionMCPServer, error) {
					mu.Lock()
					leases[id] = true
					mu.Unlock()
					if id == "A" && phase == "servers" {
						close(blocked)
						<-resume
					}
					if id == "A" && phase == "rollback" {
						return []SessionMCPServer{{Name: "instrumented"}}, nil
					}
					return []SessionMCPServer{{Name: "host"}}, nil
				},
				release: func(_ context.Context, id string) error {
					if id == "A" && phase != "servers" {
						close(blocked)
						<-resume
					}
					mu.Lock()
					delete(leases, id)
					mu.Unlock()
					return nil
				},
			}
			provider, err := NewInstrumentedSessionProvider(host, SessionMCPServer{Name: "instrumented", Command: "/wrapper", Args: []string{"/mcp"}})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if phase == "release" {
				if _, err := provider.Servers(ctx, "A", "/tmp", nil); err != nil {
					t.Fatal(err)
				}
			}
			aDone := make(chan struct{})
			aResult := make(chan error, 1)
			go func() {
				defer close(aDone)
				if phase == "release" {
					aResult <- provider.ReleaseSession(ctx, "A")
					return
				}
				_, err := provider.Servers(ctx, "A", "/tmp", nil)
				if phase == "rollback" {
					if errors.Is(err, errInstrumentedSessionMCPUnavailable) {
						err = nil
					} else {
						err = fmt.Errorf("expected collision, got %v", err)
					}
				} else if err == nil {
					err = provider.ReleaseSession(ctx, "A")
				}
				aResult <- err
			}()
			bDone := make(chan error, 1)
			bSettled := make(chan struct{})
			bStarted := false
			t.Cleanup(func() {
				resumeOnce.Do(func() { close(resume) })
				select {
				case <-aDone:
				case <-time.After(3 * time.Second):
					t.Error("session A did not settle after resume")
				}
				if bStarted {
					select {
					case <-bSettled:
					case <-time.After(3 * time.Second):
						t.Error("session B did not settle after resume")
					}
				}
			})
			select {
			case <-blocked:
			case <-time.After(3 * time.Second):
				t.Fatal("session A never entered the blocked host call")
			}
			bStarted = true
			go func() {
				defer close(bSettled)
				servers, err := provider.Servers(ctx, "B", "/tmp", nil)
				if err == nil && len(servers) != 2 {
					err = fmt.Errorf("session B server count=%d", len(servers))
				}
				if err == nil {
					err = provider.ReleaseSession(ctx, "B")
				}
				bDone <- err
			}()
			// A resumes only after B has acquired AND released. The timeout is
			// a deadlock guard, not the ordering mechanism.
			select {
			case err := <-bDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("unrelated session B blocked behind session A")
			}
			mu.Lock()
			isolated := leases["A"] && !leases["B"] && len(leases) == 1
			mu.Unlock()
			if !isolated {
				t.Fatal("session B cleanup affected blocked session A")
			}
			resumeOnce.Do(func() { close(resume) })
			select {
			case err := <-aResult:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("session A did not finish after resume")
			}
			<-aDone
			if provider.Borrowers() != 0 || len(provider.sessionLocks) != 0 {
				t.Fatal("settled sessions retained borrowers or lifecycle locks")
			}
		})
	}
}

func TestInstrumentedMCPSessionLockCancellationAndReclamation(t *testing.T) {
	provider, err := NewInstrumentedSessionProvider(nil, SessionMCPServer{Name: "instrumented", Command: "/wrapper", Args: []string{"/mcp"}})
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := provider.lockSession(context.Background(), "A")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if release, err := provider.lockSession(ctx, "A"); release != nil || !errors.Is(err, errInstrumentedSessionMCPUnavailable) {
		t.Fatalf("cancelled waiter acquired session: %v", err)
	}
	if len(provider.sessionLocks) != 1 || provider.sessionLocks["A"].users != 1 {
		t.Fatal("cancelled waiter retained a reference or removed the active lock")
	}
	unlock()
	if len(provider.sessionLocks) != 0 {
		t.Fatal("idle lifecycle lock was not reclaimed")
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
