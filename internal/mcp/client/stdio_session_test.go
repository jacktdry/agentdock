package client

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestEphemeralStdioSession(t *testing.T) {
	s, err := OpenStdioSession(context.Background(), ServerConfig{Name: "ephemeral", Transport: TransportStdio, Command: os.Args[0], Args: []string{"-test.run=TestMCPStdioHelperProcess"}, StaticEnv: map[string]string{"GO_WANT_MCP_HELPER": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	info := s.Info()
	if info.ServerName != "helper" || info.ServerVersion != "1.0.0" || info.ProtocolVersion == "" || info.PID <= 0 {
		t.Fatalf("info=%+v", info)
	}
	tools := s.Tools()
	tools["echo"].InputSchema["required"] = []string{}
	delete(tools, "echo")
	if len(s.Tools()) != 1 {
		t.Fatal("mutable tools")
	}
	if _, err := s.Call(context.Background(), "echo", nil); err == nil {
		t.Fatal("validation bypassed")
	}
	if _, err := s.Call(context.Background(), "missing", nil); err == nil {
		t.Fatal("unknown tool accepted")
	}
	result, err := s.Call(context.Background(), "echo", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if result["structuredContent"].(map[string]any)["echo"] != "hello" {
		t.Fatalf("%+v", result)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.Call(context.Background(), "echo", map[string]any{"text": "concurrent"})
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Call(context.Background(), "echo", map[string]any{"text": "closed"}); err == nil {
		t.Fatal("closed session accepted call")
	}
	if s.Info() != info {
		t.Fatal("PID observation changed")
	}
}

func TestToolCatalogSharedValidation(t *testing.T) {
	for _, listed := range [][]Tool{{{Name: " "}}, {{Name: "duplicate"}, {Name: "duplicate"}}, {{Name: "bad", InputSchema: map[string]any{"type": "invalid"}}}} {
		if _, err := normalizeToolCatalog("test", listed); err == nil {
			t.Fatal("invalid catalog accepted")
		}
	}
	tools, err := normalizeToolCatalog("test", []Tool{{Name: " trimmed "}})
	if err != nil || tools["trimmed"].inputValidator == nil {
		t.Fatalf("%+v %v", tools, err)
	}
}

func TestEphemeralSessionMissingServerInfo(t *testing.T) {
	s, err := OpenStdioSession(context.Background(), ServerConfig{Name: "no-info", Transport: TransportStdio, Command: os.Args[0], Args: []string{"-test.run=TestMCPStdioHelperProcess"}, StaticEnv: map[string]string{"GO_WANT_MCP_HELPER": "1", "MCP_HELPER_OMIT_SERVER_INFO": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	if s.Info().ServerName != "" || s.Info().ServerVersion != "" {
		t.Fatalf("invented server info: %+v", s.Info())
	}
}

func TestEphemeralRejectsTransportAndStartFailure(t *testing.T) {
	for _, cfg := range []ServerConfig{{Transport: TransportStreamableHTTP, URL: "https://example.invalid"}, {Transport: TransportStdio, Command: "/nonexistent/ephemeral-command"}} {
		if s, err := OpenStdioSession(context.Background(), cfg); s != nil || err == nil {
			t.Fatal("invalid session opened")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := OpenStdioSession(ctx, ServerConfig{Name: "canceled", Transport: TransportStdio, Command: os.Args[0], Args: []string{"-test.run=TestMCPStdioHelperProcess"}, StaticEnv: map[string]string{"GO_WANT_MCP_HELPER": "1"}})
	if err == nil {
		t.Fatal("canceled initialize succeeded")
	}
	var typed *Error
	if !errors.As(err, &typed) {
		t.Fatalf("untyped error: %v", err)
	}
}

func TestEphemeralGracefulAndStubbornClose(t *testing.T) {
	for _, mode := range []string{"graceful", "stubborn"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "eof")
			profile := filepath.Join(dir, "owned-profile")
			if err := os.Mkdir(profile, 0700); err != nil {
				t.Fatal(err)
			}
			s, err := OpenStdioSession(context.Background(), ServerConfig{Name: "cleanup", Transport: TransportStdio, Command: os.Args[0], Args: []string{"-test.run=TestMCPStdioHelperProcess"}, StaticEnv: map[string]string{"GO_WANT_MCP_HELPER": "1", "MCP_HELPER_EOF_MARKER": marker, "MCP_HELPER_CLOSE_MODE": mode, "MCP_HELPER_OWNED_PROFILE": profile}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			cmd := s.client.command // Inspect the actual process exit, not a controller mock.
			started := time.Now()
			results := make(chan error, 8)
			for i := 0; i < 8; i++ {
				go func() { results <- s.Close() }()
			}
			for i := 0; i < 8; i++ {
				select {
				case err := <-results:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(10 * time.Second):
					t.Fatal("Close exceeded cleanup bound")
				}
			}
			elapsed := time.Since(started)
			if raw, err := os.ReadFile(marker); err != nil || string(raw) != "EOF" {
				t.Fatalf("helper never received EOF: %q %v", raw, err)
			}
			if cmd.ProcessState == nil {
				t.Fatal("owned process not reaped")
			}
			if mode == "graceful" {
				if !cmd.ProcessState.Success() {
					t.Fatalf("graceful helper forcibly stopped: %v", cmd.ProcessState)
				}
				if _, err := os.Stat(profile); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("EOF cleanup did not remove owned profile: %v", err)
				}
			} else {
				if cmd.ProcessState.Success() {
					t.Fatal("stubborn helper unexpectedly exited naturally")
				}
				if elapsed < stdioGracePeriod || elapsed > 10*time.Second {
					t.Fatalf("unbounded/early termination: %v", elapsed)
				}
			}
			if err := s.Close(); err != nil {
				t.Fatalf("repeated Close/Wait: %v", err)
			}
		})
	}
}
