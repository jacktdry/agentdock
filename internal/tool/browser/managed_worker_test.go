package browser

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
)

// The fixture projects only required/type/enum invariants from cached 1.7.0
// createTools + injected pageIdSchema (ToolHandler, experimentalPageIdRouting).
// It is committed evidence, never regenerated/read from npm during tests.
func managedToolFixture(t *testing.T) map[string]mcpclient.Tool {
	t.Helper()
	raw, err := os.ReadFile("testdata/managed-1.7.0-schema-projection.json")
	if err != nil {
		t.Fatal(err)
	}
	var listed []mcpclient.Tool
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatal(err)
	}
	tools := map[string]mcpclient.Tool{}
	for _, tool := range listed {
		tools[tool.Name] = tool
	}
	return tools
}

type fakeWorkerSession struct {
	mu          sync.Mutex
	info        mcpclient.SessionInfo
	tools       map[string]mcpclient.Tool
	closed      bool
	closes      int
	identity    string
	toolEntered chan struct{}
	toolGate    <-chan struct{}
}

func (s *fakeWorkerSession) Info() mcpclient.SessionInfo { return s.info }
func (s *fakeWorkerSession) Tools() map[string]mcpclient.Tool {
	if s.toolEntered != nil {
		close(s.toolEntered)
		<-s.toolGate
	}
	return s.tools
}
func (s *fakeWorkerSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.closes++
	}
	return nil
}
func (s *fakeWorkerSession) Call(_ context.Context, tool string, args map[string]any) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("closed")
	}
	return map[string]any{"worker": s.identity, "tool": tool, "args": args}, nil
}
func fakeManagedSession(t *testing.T, id string) *fakeWorkerSession {
	return &fakeWorkerSession{info: mcpclient.SessionInfo{ServerName: ManagedEngineServerName, ServerVersion: ManagedEngineVersion, ProtocolVersion: "2025-06-18", PID: 123}, tools: managedToolFixture(t), identity: id}
}
func assertBrowserCode(t *testing.T, err error, code string) {
	t.Helper()
	var be *Error
	if !errors.As(err, &be) || be.Code != code {
		t.Fatalf("error=%v want %s", err, code)
	}
}

func TestManagedWorkerPinAndVersionProbe(t *testing.T) {
	cfg := ManagedWorkerConfig("/workspace")
	want := []string{"--yes", "chrome-devtools-mcp@1.7.0", "--isolated", "--headless", "--experimentalPageIdRouting", "--no-usage-statistics", "--no-performance-crux"}
	if cfg.Command != "npx" || cfg.Cwd != "/workspace" || !reflect.DeepEqual(cfg.Args, want) {
		t.Fatalf("config=%+v", cfg)
	}
	if strings.Contains(strings.Join(cfg.Args, " "), "@latest") {
		t.Fatal("unpinned launch")
	}
	cfg.Args[0] = "mutated"
	if ManagedWorkerConfig("").Args[0] != "--yes" {
		t.Fatal("mutable launch config")
	}
	for _, version := range []string{"1.10.1", "1.7.0-beta", "v1.7.0", "1.7.0\nwarning", ""} {
		assertBrowserCode(t, validateManagedVersion(version), ErrEngineVersionIncompatible)
	}
	if err := validateManagedVersion(" 1.7.0\n"); err != nil {
		t.Fatal(err)
	}
	version, err := runOwnedVersionProbe(context.Background(), os.Args[0], []string{"-test.run=TestManagedVersionHelper", "--", "m6-version"}, "")
	if err != nil || version != "1.7.0" {
		t.Fatalf("version=%q error=%v", version, err)
	}
	if _, err := runOwnedVersionProbe(context.Background(), os.Args[0], []string{"-test.run=TestManagedVersionHelper", "--", "m6-fail"}, ""); err == nil {
		t.Fatal("failed probe accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runOwnedVersionProbe(ctx, os.Args[0], []string{"-test.run=TestManagedVersionHelper", "--", "m6-version"}, ""); err == nil {
		t.Fatal("canceled probe accepted")
	}
}
func TestManagedVersionHelper(t *testing.T) {
	if len(os.Args) < 2 {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "m6-version":
		_, _ = os.Stdout.WriteString("1.7.0\n")
		_, _ = os.Stderr.WriteString("npm notice New patch version available\n")
		os.Exit(0)
	case "m6-fail":
		os.Exit(2)
	}
}

func TestManagedSchemaInvariants(t *testing.T) {
	tools := managedToolFixture(t)
	caps, err := ValidateManagedTools(tools)
	if err != nil || !caps.PageIDRouting || !caps.BackgroundPages || !caps.NamedIsolatedContexts {
		t.Fatalf("%+v %v", caps, err)
	}
	for _, name := range []string{"close_page", "navigate_page", "take_snapshot", "take_screenshot", "evaluate_script", "click", "fill", "press_key"} {
		for _, mutate := range []string{"optional", "string", "missing"} {
			t.Run(name+"-"+mutate, func(t *testing.T) {
				tools := managedToolFixture(t)
				s := tools[name].InputSchema
				switch mutate {
				case "optional":
					var required []any
					for _, f := range s["required"].([]any) {
						if f != "pageId" {
							required = append(required, f)
						}
					}
					s["required"] = required
				case "string":
					s["properties"].(map[string]any)["pageId"].(map[string]any)["type"] = "string"
				case "missing":
					delete(s["properties"].(map[string]any), "pageId")
				}
				_, err := ValidateManagedTools(tools)
				assertBrowserCode(t, err, ErrEngineSchemaIncompatible)
			})
		}
	}
	for _, name := range []string{"new_page", "list_pages", "take_screenshot", "navigate_page"} {
		t.Run("contract-"+name, func(t *testing.T) {
			tools := managedToolFixture(t)
			s := tools[name].InputSchema
			switch name {
			case "new_page":
				s["required"] = append(s["required"].([]any), "background")
			case "list_pages":
				s["properties"] = map[string]any{"x": map[string]any{"type": "string"}}
				s["required"] = []string{"x"}
			case "take_screenshot":
				s["properties"].(map[string]any)["format"].(map[string]any)["enum"] = []string{"jpeg"}
			case "navigate_page":
				s["properties"].(map[string]any)["type"].(map[string]any)["enum"] = []string{"url"}
			}
			_, err := ValidateManagedTools(tools)
			assertBrowserCode(t, err, ErrEngineSchemaIncompatible)
		})
	}
}

func TestManagedWorkerValidationFailureCleanup(t *testing.T) {
	cases := []struct {
		name, code string
		mutate     func(*fakeWorkerSession)
		probe      string
		probeErr   bool
	}{
		{name: "wrong name", code: ErrEngineVersionIncompatible, mutate: func(s *fakeWorkerSession) { s.info.ServerName = "other" }},
		{name: "wrong handshake version", code: ErrEngineVersionIncompatible, mutate: func(s *fakeWorkerSession) { s.info.ServerVersion = "1.10.1" }},
		{name: "absent server info", code: ErrEngineVersionIncompatible, mutate: func(s *fakeWorkerSession) { s.info = mcpclient.SessionInfo{} }},
		{name: "missing schema", code: ErrEngineSchemaIncompatible, mutate: func(s *fakeWorkerSession) { delete(s.tools, "click") }},
		{name: "probe mismatch", code: ErrEngineVersionIncompatible, probe: "1.10.1"},
		{name: "probe unavailable", code: ErrEngineUnavailable, probeErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := fakeManagedSession(t, "bad")
			if tc.mutate != nil {
				tc.mutate(s)
			}
			opened := false
			r := NewWorkerRegistry(WorkerDependencies{Probe: func(context.Context, mcpclient.ServerConfig) (string, error) {
				if tc.probeErr {
					return "", errors.New("unavailable")
				}
				if tc.probe != "" {
					return tc.probe, nil
				}
				return "1.7.0", nil
			}, Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) { opened = true; return s, nil }})
			info, err := r.StartManaged(context.Background(), ManagedWorkerOptions{})
			assertBrowserCode(t, err, tc.code)
			if info.State != WorkerFailed || info.WorkerID == "" {
				t.Fatalf("%+v", info)
			}
			if opened && !s.closed {
				t.Fatal("failed worker not closed")
			}
			if !opened && s.closed {
				t.Fatal("unopened session closed")
			}
			_, err = r.Call(context.Background(), info.WorkerID, "list_pages", nil)
			assertBrowserCode(t, err, ErrWorkerNotReady)
			if err := r.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestManagedWorkerRegistryIsolationAndShutdown(t *testing.T) {
	a, b := fakeManagedSession(t, "A"), fakeManagedSession(t, "B")
	sessions := []*fakeWorkerSession{a, b}
	i := 0
	r := NewWorkerRegistry(WorkerDependencies{Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return "1.7.0", nil }, Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) {
		s := sessions[i]
		i++
		return s, nil
	}})
	ai, err := r.StartManaged(context.Background(), ManagedWorkerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	bi, err := r.StartManaged(context.Background(), ManagedWorkerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ai.WorkerID == bi.WorkerID || ai.CreatedAt.IsZero() || ai.State != WorkerReady || !ai.Compatibility.PageIDRouting {
		t.Fatal("invalid worker identity")
	}
	for id, want := range map[string]string{ai.WorkerID: "A", bi.WorkerID: "B"} {
		got, err := r.Call(context.Background(), id, "click", map[string]any{"pageId": float64(7), "uid": "u"})
		if err != nil || got["worker"] != want {
			t.Fatalf("%v %v", got, err)
		}
	}
	_, err = r.Call(context.Background(), ai.WorkerID, "select_page", map[string]any{"pageId": 1})
	assertBrowserCode(t, err, ErrEngineSchemaIncompatible)
	var wg sync.WaitGroup
	for j := 0; j < 8; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.Info(ai.WorkerID)
			_, _ = r.Call(context.Background(), ai.WorkerID, "list_pages", nil)
		}()
	}
	if err := r.Stop(ai.WorkerID); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	_, err = r.Call(context.Background(), ai.WorkerID, "list_pages", nil)
	assertBrowserCode(t, err, ErrWorkerNotReady)
	if err := r.Stop(ai.WorkerID); err != nil {
		t.Fatal(err)
	}
	_, err = r.Info("unknown")
	assertBrowserCode(t, err, ErrWorkerNotFound)
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.closes != 1 || b.closes != 1 {
		t.Fatalf("close counts %d %d", a.closes, b.closes)
	}
	if _, err := r.StartManaged(context.Background(), ManagedWorkerOptions{}); err == nil {
		t.Fatal("start after shutdown")
	}
}

func TestManagedWorkerShutdownDuringStart(t *testing.T) {
	entered := make(chan struct{})
	r := NewWorkerRegistry(WorkerDependencies{Probe: func(ctx context.Context, _ mcpclient.ServerConfig) (string, error) {
		close(entered)
		<-ctx.Done()
		return "", ctx.Err()
	}, Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) {
		t.Error("open after canceled probe")
		return nil, errors.New("unexpected")
	}})
	done := make(chan error, 1)
	go func() { _, err := r.StartManaged(context.Background(), ManagedWorkerOptions{}); done <- err }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("canceled start succeeded")
	}
}

func TestManagedWorkerNotReadyDuringValidation(t *testing.T) {
	gate := make(chan struct{})
	s := fakeManagedSession(t, "validating")
	s.toolEntered = make(chan struct{})
	s.toolGate = gate
	r := NewWorkerRegistry(WorkerDependencies{Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return "1.7.0", nil }, Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) { return s, nil }})
	done := make(chan error, 1)
	go func() { _, err := r.StartManaged(context.Background(), ManagedWorkerOptions{}); done <- err }()
	<-s.toolEntered
	r.mu.Lock()
	var id string
	for key := range r.workers {
		id = key
	}
	r.mu.Unlock()
	info, err := r.Info(id)
	if err != nil || info.State != WorkerValidating {
		t.Fatalf("%+v %v", info, err)
	}
	_, err = r.Call(context.Background(), id, "list_pages", nil)
	assertBrowserCode(t, err, ErrWorkerNotReady)
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestManagedWorkerOpenFailureCleanup(t *testing.T) {
	s := fakeManagedSession(t, "partial")
	r := NewWorkerRegistry(WorkerDependencies{Probe: func(context.Context, mcpclient.ServerConfig) (string, error) { return "1.7.0", nil }, Open: func(context.Context, mcpclient.ServerConfig) (WorkerSession, error) {
		return s, &mcpclient.Error{Code: "MCP_SCHEMA_INVALID", Message: "invalid upstream schema"}
	}})
	info, err := r.StartManaged(context.Background(), ManagedWorkerOptions{})
	assertBrowserCode(t, err, ErrEngineSchemaIncompatible)
	if info.State != WorkerFailed || !s.closed {
		t.Fatal("partial open was published or leaked")
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
