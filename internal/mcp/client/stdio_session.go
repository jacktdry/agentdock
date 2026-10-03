package client

import (
	"context"
	"errors"
	"sync"
	"time"
)

type SessionInfo struct {
	ProtocolVersion string
	ServerName      string
	ServerVersion   string
	PID             int // Observation only; this value does not confer process ownership.
}

// StdioSession owns one ephemeral SDK connection; it never uses Manager, OAuth
// or persistent registry storage. Calls are bounded by the configured timeout.
type StdioSession struct {
	mu       sync.RWMutex
	client   *sdkProtocolClient
	info     SessionInfo
	tools    map[string]Tool
	timeout  time.Duration
	closed   bool
	closeErr error
}

func OpenStdioSession(ctx context.Context, cfg ServerConfig) (*StdioSession, error) {
	cfg = normalizeServerConfig(cfg)
	if cfg.Transport != TransportStdio || cfg.Command == "" || cfg.TimeoutMS < 1 || cfg.TimeoutMS > maxTimeoutMS {
		return nil, newError("MCP_CONFIG_INVALID", "ephemeral session requires a stdio command and bounded timeout", false, nil, nil)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutMS)*time.Millisecond)
	defer cancel()
	c := newStdioClient(cfg)
	if err := c.initialize(ctx); err != nil {
		return nil, errors.Join(err, c.close())
	}
	listed, err := c.listTools(ctx)
	if err != nil {
		return nil, errors.Join(err, c.close())
	}
	tools, err := normalizeToolCatalog(cfg.Name, listed)
	if err != nil {
		return nil, errors.Join(err, c.close())
	}
	return &StdioSession{client: c, info: c.info, tools: tools, timeout: time.Duration(cfg.TimeoutMS) * time.Millisecond}, nil
}

func (s *StdioSession) Info() SessionInfo { return s.info }

func (s *StdioSession) Tools() map[string]Tool {
	// Deep copies keep callers from mutating validation/schema state.
	result := make(map[string]Tool, len(s.tools))
	for name, t := range s.tools {
		t.InputSchema, _ = jsonMap(t.InputSchema)
		t.OutputSchema, _ = jsonMap(t.OutputSchema)
		t.Annotations, _ = jsonMap(t.Annotations)
		result[name] = t
	}
	return result
}

func (s *StdioSession) Call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, newError("MCP_SESSION_CLOSED", "ephemeral session is closed", false, nil, nil)
	}
	t, ok := s.tools[name]
	if !ok {
		return nil, newError("MCP_TOOL_NOT_FOUND", "MCP tool not found", false, map[string]any{"tool": name}, nil)
	}
	if err := validateToolArguments(t, args); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.client.callTool(ctx, name, args)
}

func (s *StdioSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.closeErr = s.client.close()
	}
	return s.closeErr
}
