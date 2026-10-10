//go:build darwin

package client

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uvwt/agentdock/internal/config"
	processcontrol "github.com/uvwt/agentdock/internal/process"
)

// SupervisorOwnedMCPSession is a source-only, Supervisor-local MCP SDK bridge.
// It does NOT start/kill a Worker or expose stdio to a Core or socket peer.
// No Next runtime/BrowserRegistry calls this prototype.
type SupervisorOwnedMCPSession struct {
	mu          sync.Mutex
	session     *mcpsdk.ClientSession
	info        SessionInfo
	workerBirth processcontrol.BirthIdentity
	tools       map[string]Tool
	stdin       io.WriteCloser
	stdout      io.ReadCloser
	timeout     time.Duration
	closed      bool
}

// ConnectSupervisorOwnedMCPPrototype consumes ONCE the pipes belonging to a
// kernel-attested child already launched by THIS Supervisor process.
// SDK initialize + tool catalog validation must succeed before publication.
// A caller MUST subsequently call owned.Wait(ctx) to witness/reap child exit.
func ConnectSupervisorOwnedMCPPrototype(ctx context.Context, cfg ServerConfig,
	owned *processcontrol.SupervisorOwnedStdio) (*SupervisorOwnedMCPSession, error) {
	if ctx == nil || ctx.Err() != nil || owned == nil {
		return nil, processcontrol.ErrSupervisorHandoffUnavailable
	}
	cfg = normalizeServerConfig(cfg)
	if cfg.Name == "" || cfg.Transport != TransportStdio || cfg.TimeoutMS < 1 || cfg.TimeoutMS > maxTimeoutMS ||
		owned.Birth().PID <= 0 {
		return nil, processcontrol.ErrSupervisorHandoffUnavailable
	}
	reader, writer, err := owned.ClaimMCPTransport()
	if err != nil {
		return nil, processcontrol.ErrSupervisorHandoffUnavailable
	}
	// From here, a failed handshake must close the one-time transport.
	connected := false
	defer func() {
		if !connected {
			_ = writer.Close()
			_ = reader.Close()
		}
	}()
	deadline, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutMS)*time.Millisecond)
	defer cancel()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: config.ServerName, Version: "supervisor-prototype"}, nil)
	var options *mcpsdk.ClientSessionOptions
	if cfg.ProtocolVersion != "" {
		options = &mcpsdk.ClientSessionOptions{ProtocolVersion: cfg.ProtocolVersion}
	}
	session, err := client.Connect(deadline, &mcpsdk.IOTransport{Reader: reader, Writer: writer}, options)
	if err != nil {
		return nil, newError("MCP_CONNECTION_FAILED", "connect Supervisor-owned MCP transport", false, nil, err)
	}
	defer func() {
		if !connected {
			_ = session.Close()
		}
	}()
	initialized := session.InitializeResult()
	if initialized == nil || initialized.ProtocolVersion == "" ||
		!supportedMCPProtocolVersion(initialized.ProtocolVersion) {
		return nil, newError("MCP_INVALID_RESPONSE", "Supervisor-owned MCP initialization missing supported version", false, nil, nil)
	}
	listed := make([]Tool, 0)
	for remote, err := range session.Tools(deadline, nil) {
		if err != nil {
			return nil, newError("MCP_CONNECTION_FAILED", "list Supervisor-owned MCP tools", false, nil, err)
		}
		tool, err := convertSDKTool(remote)
		if err != nil {
			return nil, err
		}
		listed = append(listed, tool)
	}
	tools, err := normalizeToolCatalog(cfg.Name, listed)
	if err != nil {
		return nil, err
	}
	info := SessionInfo{PID: owned.Birth().PID, ProtocolVersion: initialized.ProtocolVersion}
	if initialized.ServerInfo != nil {
		info.ServerName = initialized.ServerInfo.Name
		info.ServerVersion = initialized.ServerInfo.Version
	}
	result := &SupervisorOwnedMCPSession{
		session: session, info: info, workerBirth: owned.Birth(), tools: tools, stdin: writer, stdout: reader,
		timeout: time.Duration(cfg.TimeoutMS) * time.Millisecond,
	}
	connected = true
	return result, nil
}
func (s *SupervisorOwnedMCPSession) Info() SessionInfo {
	if s == nil {
		return SessionInfo{}
	}
	return s.info
}
func (s *SupervisorOwnedMCPSession) Tools() map[string]Tool {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	out := make(map[string]Tool, len(s.tools))
	for k, t := range s.tools {
		t.InputSchema, _ = jsonMap(t.InputSchema)
		t.OutputSchema, _ = jsonMap(t.OutputSchema)
		t.Annotations, _ = jsonMap(t.Annotations)
		out[k] = t
	}
	return out
}
func (s *SupervisorOwnedMCPSession) Call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	if s == nil || ctx == nil {
		return nil, processcontrol.ErrSupervisorHandoffUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, newError("MCP_SESSION_CLOSED", "Supervisor-owned MCP session is closed", false, nil, nil)
	}
	tool, ok := s.tools[name]
	if !ok {
		return nil, newError("MCP_TOOL_NOT_FOUND", "MCP tool not found", false, nil, nil)
	}
	if err := validateToolArguments(tool, args); err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	output, err := s.session.CallTool(callCtx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, newError("MCP_CONNECTION_FAILED", "call Supervisor-owned MCP tool", false, nil, err)
	}
	return jsonObject(output)
}

// Close ends ONLY the local SDK session/stdio and never SIGKILLs a PID.
// SupervisorOwnedStdio.Wait remains the sole cmd.Wait/kqueue owner.
func (s *SupervisorOwnedMCPSession) Close() error {
	if s == nil {
		return processcontrol.ErrSupervisorHandoffUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var err error
	if s.session != nil {
		err = errors.Join(err, s.session.Close())
		s.session = nil
	}
	if s.stdin != nil {
		if e := s.stdin.Close(); e != nil && !errors.Is(e, os.ErrClosed) {
			err = errors.Join(err, e)
		}
		s.stdin = nil
	}
	if s.stdout != nil {
		if e := s.stdout.Close(); e != nil && !errors.Is(e, os.ErrClosed) {
			err = errors.Join(err, e)
		}
		s.stdout = nil
	}
	return err
}
