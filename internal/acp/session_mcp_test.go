package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordingSessionMCPProvider struct {
	mu       sync.Mutex
	servers  []string
	releases []string
}

func (p *recordingSessionMCPProvider) Servers(_ context.Context, sessionID, _ string, _ []string) ([]SessionMCPServer, error) {
	p.mu.Lock()
	p.servers = append(p.servers, sessionID)
	p.mu.Unlock()
	return []SessionMCPServer{{Name: "agentdock-browser", Type: "http", URL: "http://127.0.0.1:8765/internal/acp-browser/mcp", Headers: []SessionMCPHeader{{Name: "Authorization", Value: "Bearer " + sessionID}}}}, nil
}

func (p *recordingSessionMCPProvider) ReleaseSession(_ context.Context, sessionID string) error {
	p.mu.Lock()
	p.releases = append(p.releases, sessionID)
	p.mu.Unlock()
	return nil
}

func (p *recordingSessionMCPProvider) snapshot() ([]string, []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.servers...), append([]string(nil), p.releases...)
}

func TestSessionMCPProviderFollowsACPNewLoadForkAndCloseLifecycle(t *testing.T) {
	provider := &recordingSessionMCPProvider{}
	workspace := t.TempDir()
	manager, err := NewManager(Options{
		Home: t.TempDir(), DefaultCWD: workspace,
		Agent: AgentSpec{Name: "helper", Command: os.Args[0], Args: []string{"-test.run=TestACPHelperProcess"}, Environment: map[string]string{
			"GO_WANT_ACP_HELPER": "1", "GO_ACP_HELPER_AGENT_INFO_NAME": "helper-acp", "GO_ACP_HELPER_REQUIRE_MCP": "1",
		}},
		MaxConcurrentRuns: 2, InteractionTimeout: 3 * time.Second, SessionMCPProvider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()

	created, err := manager.NewSession(context.Background(), workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created.Session.ID == "" {
		t.Fatal("local session id missing")
	}
	serverCalls, _ := provider.snapshot()
	if len(serverCalls) != 1 || serverCalls[0] != created.Session.ID {
		t.Fatalf("server calls=%v", serverCalls)
	}

	if _, err := manager.CloseSession(context.Background(), created.Session.ID); err != nil {
		t.Fatal(err)
	}
	_, releases := provider.snapshot()
	if len(releases) != 1 || releases[0] != created.Session.ID {
		t.Fatalf("releases=%v", releases)
	}

	if _, err := manager.CloseSession(context.Background(), created.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.EnsureSessionActive(context.Background(), created.Session.ID); err != nil {
		t.Fatal(err)
	}
	serverCalls, _ = provider.snapshot()
	if len(serverCalls) < 2 || serverCalls[len(serverCalls)-1] != created.Session.ID {
		t.Fatalf("load server calls=%v", serverCalls)
	}

	forked, err := manager.ForkSession(context.Background(), created.Session.ID, workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	if forked.Session.ID == created.Session.ID {
		t.Fatal("fork reused local session id")
	}
	serverCalls, _ = provider.snapshot()
	if serverCalls[len(serverCalls)-1] != forked.Session.ID {
		t.Fatalf("fork server calls=%v", serverCalls)
	}
	if err := manager.DeleteSession(context.Background(), forked.Session.ID); err != nil {
		t.Fatal(err)
	}
	_, releases = provider.snapshot()
	beforeDelete := len(releases)
	_ = manager.DeleteSession(context.Background(), forked.Session.ID)
	_, repeated := provider.snapshot()
	if len(repeated) != beforeDelete {
		t.Fatal("repeated delete released capability again")
	}
	if releases[len(releases)-1] != forked.Session.ID {
		t.Fatalf("delete releases=%v", releases)
	}
}

func TestSessionMCPCapabilityFailureAndShutdown(t *testing.T) {
	for _, failed := range []string{"session/new", "session/fork", ""} {
		t.Run(failed, func(t *testing.T) {
			provider := &recordingSessionMCPProvider{}
			workspace := t.TempDir()
			trace := filepath.Join(t.TempDir(), "trace.jsonl")
			m, err := NewManager(Options{Home: t.TempDir(), DefaultCWD: workspace, Agent: AgentSpec{Name: "helper", Command: os.Args[0], Args: []string{"-test.run=TestACPHelperProcess"}, Environment: map[string]string{"GO_WANT_ACP_HELPER": "1", "GO_ACP_HELPER_REQUIRE_MCP": "1", "GO_ACP_HELPER_FAIL_METHOD": failed, "GO_ACP_HELPER_MCP_TRACE": trace}}, SessionMCPProvider: provider})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			created, err := m.NewSession(context.Background(), workspace, nil)
			if failed == "session/new" {
				if err == nil {
					t.Fatal("new succeeded")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if failed == "session/fork" {
					if _, err := m.ForkSession(context.Background(), created.Session.ID, workspace, nil); err == nil {
						t.Fatal("fork succeeded")
					}
				} else {
					for _, activate := range []func(context.Context, string) (SessionResult, error){m.LoadSession, m.ResumeSession} {
						// Simulate an adapter restart without releasing the still-live capability.
						m.mu.Lock()
						delete(m.loaded, created.Session.ID)
						m.mu.Unlock()
						if _, err := activate(context.Background(), created.Session.ID); err != nil {
							t.Fatal(err)
						}
					}
					calls, _ := provider.snapshot()
					if len(calls) != 3 {
						t.Fatalf("calls=%v", calls)
					}
					for _, id := range calls {
						if id != created.Session.ID {
							t.Fatalf("activation changed local identity: %v", calls)
						}
					}
					if _, err := m.NewSession(context.Background(), workspace, nil); err != nil {
						t.Fatal(err)
					}
				}
			}
			calls, releases := provider.snapshot()
			if failed != "" && (len(releases) != 1 || releases[0] != calls[len(calls)-1]) {
				t.Fatalf("preallocation leaked: calls=%v releases=%v", calls, releases)
			}
			data, err := os.ReadFile(trace)
			if err != nil {
				t.Fatal(err)
			}
			// The remote request must carry the capability prepared by the provider.
			var message rpcMessage
			if err := json.Unmarshal([]byte(strings.Split(string(data), "\n")[0]), &message); err != nil {
				t.Fatal(err)
			}
			var params struct {
				MCPServers []SessionMCPServer `json:"mcpServers"`
			}
			if err := json.Unmarshal(message.Params, &params); err != nil || len(params.MCPServers) != 1 || calls[0] == "" || params.MCPServers[0].Headers[0].Value != "Bearer "+calls[0] {
				t.Fatalf("request=%s calls=%v", message.Params, calls)
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			_, after := provider.snapshot()
			if failed == "" {
				want := []string{created.Session.ID, calls[len(calls)-1]}
				sort.Strings(want)
				sort.Strings(after)
				if !reflect.DeepEqual(after, want) {
					t.Fatalf("shutdown releases=%v want=%v", after, want)
				}
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			_, again := provider.snapshot()
			if !reflect.DeepEqual(after, again) {
				t.Fatal("repeated shutdown released twice")
			}
		})
	}
}
