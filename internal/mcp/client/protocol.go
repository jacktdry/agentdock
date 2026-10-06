package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	sdkjsonrpc "github.com/modelcontextprotocol/go-sdk/jsonrpc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uvwt/agentdock/internal/buildinfo"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/envstore"
	"github.com/uvwt/agentdock/internal/mcp/oauthclient"
	processcontrol "github.com/uvwt/agentdock/internal/process"
)

// go-sdk 公开 jsonrpc 包未导出 ErrRejected，只能通过稳定的 wire code 识别。
const sdkTransportRejectedCode int64 = -32005

const stdioGracePeriod = 2500 * time.Millisecond
const stdioTerminateWait = 3 * time.Second

var supportedMCPProtocolVersions = map[string]struct{}{
	"2024-11-05": {},
	"2025-03-26": {},
	"2025-06-18": {},
	"2025-11-25": {},
	"2026-07-28": {},
}

func supportedMCPProtocolVersion(version string) bool {
	_, ok := supportedMCPProtocolVersions[strings.TrimSpace(version)]
	return ok
}

type protocolClient interface {
	initialize(context.Context) error
	listTools(context.Context) ([]Tool, error)
	callTool(context.Context, string, map[string]any) (map[string]any, error)
	close() error
}

type sdkProtocolClient struct {
	info       SessionInfo
	cfg        ServerConfig
	session    *mcpsdk.ClientSession
	command    *exec.Cmd
	stdin      io.WriteCloser
	controller *processcontrol.Controller
	stderr     *tailBuffer
	oauth      sdkauth.OAuthHandler
	httpClient *http.Client
	closeOnce  sync.Once
	closeErr   error
}

func newStreamableHTTPClient(cfg ServerConfig, oauth sdkauth.OAuthHandler) *sdkProtocolClient {
	return &sdkProtocolClient{cfg: cfg, oauth: oauth}
}

func newStdioClient(cfg ServerConfig) *sdkProtocolClient {
	return &sdkProtocolClient{cfg: cfg, stderr: newTailBuffer(64 << 10)}
}

func (c *sdkProtocolClient) initialize(ctx context.Context) error {
	transport, err := c.transport()
	if err != nil {
		return err
	}
	client := mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: config.ServerName, Version: buildinfo.Version},
		&mcpsdk.ClientOptions{Capabilities: &mcpsdk.ClientCapabilities{}},
	)
	var sessionOptions *mcpsdk.ClientSessionOptions
	if c.cfg.ProtocolVersion != "" {
		sessionOptions = &mcpsdk.ClientSessionOptions{ProtocolVersion: c.cfg.ProtocolVersion}
	}
	session, err := client.Connect(ctx, transport, sessionOptions)
	if err != nil {
		return errors.Join(c.wrapSDKError("initialize MCP session", err), c.cleanupProcess())
	}
	initialized := session.InitializeResult()
	if initialized == nil || strings.TrimSpace(initialized.ProtocolVersion) == "" {
		return errors.Join(newError("MCP_INVALID_RESPONSE", "MCP initialize response omitted protocolVersion", false, map[string]any{"server": c.cfg.Name}, nil), session.Close(), c.cleanupProcess())
	}
	c.info.ProtocolVersion = initialized.ProtocolVersion
	if initialized.ServerInfo != nil {
		c.info.ServerName = initialized.ServerInfo.Name
		c.info.ServerVersion = initialized.ServerInfo.Version
	}
	if c.command != nil {
		c.info.PID = c.command.Process.Pid
	}
	c.session = session
	return nil
}

func (c *sdkProtocolClient) transport() (mcpsdk.Transport, error) {
	switch c.cfg.Transport {
	case TransportStreamableHTTP:
		headers, err := resolveHTTPHeaders(c.cfg)
		if err != nil {
			return nil, err
		}
		oauth := c.oauth
		if strings.TrimSpace(headers.Get("Authorization")) != "" {
			// 显式 Authorization Header 始终优先于自动 OAuth；否则 SDK 即使拿到
			// access token，也会在真正发请求前被静态 Header 覆盖。
			oauth = nil
		}
		endpoint, err := url.Parse(c.cfg.URL)
		if err != nil || endpoint.Hostname() == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
			return nil, errors.New("invalid MCP HTTP endpoint")
		}
		// Copy the client so repeated setup cannot stack wrappers or mutate callers.
		httpClient := &http.Client{}
		if c.httpClient != nil {
			*httpClient = *c.httpClient
		}
		previousRedirect := httpClient.CheckRedirect
		httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if !sameHTTPOrigin(endpoint, req.URL) {
				return errMCPOriginRejected
			}
			if previousRedirect != nil {
				return previousRedirect(req, via)
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		}
		httpClient.Transport = headerRoundTripper{headers: headers, endpoint: endpoint, base: httpClient.Transport}
		return &mcpsdk.StreamableClientTransport{
			Endpoint:             c.cfg.URL,
			HTTPClient:           httpClient,
			OAuthHandler:         oauth,
			MaxRetries:           -1,
			DisableStandaloneSSE: true,
		}, nil
	case TransportStdio:
		return c.startStdioTransport()
	default:
		return nil, newError("MCP_TRANSPORT_UNSUPPORTED", fmt.Sprintf("unsupported MCP transport %q", c.cfg.Transport), false, map[string]any{"server": c.cfg.Name}, nil)
	}
}

func (c *sdkProtocolClient) startStdioTransport() (mcpsdk.Transport, error) {
	environment, err := stdioEnvironment(c.cfg)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(c.cfg.Command, c.cfg.Args...)
	cmd.WaitDelay = 2 * time.Second
	cmd.Dir = c.cfg.Cwd
	cmd.Env = environment
	cmd.Stderr = c.stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, newError("MCP_START_FAILED", "open MCP stdio stdout", false, map[string]any{"server": c.cfg.Name}, err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, newError("MCP_START_FAILED", "open MCP stdio stdin", false, map[string]any{"server": c.cfg.Name}, err)
	}
	processcontrol.Configure(cmd)
	if err := cmd.Start(); err != nil {
		return nil, newError("MCP_START_FAILED", "start MCP stdio server", false, map[string]any{"server": c.cfg.Name, "command": c.cfg.Command}, err)
	}
	controller, err := processcontrol.Attach(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, newError("MCP_START_FAILED", "attach MCP stdio process controller", false, map[string]any{"server": c.cfg.Name}, err)
	}
	c.command = cmd
	c.stdin = stdin
	c.controller = controller
	return &mcpsdk.IOTransport{Reader: stdout, Writer: stdin}, nil
}

func (c *sdkProtocolClient) listTools(ctx context.Context) ([]Tool, error) {
	if c.session == nil {
		return nil, newError("MCP_CONNECTION_FAILED", "MCP session is not initialized", true, map[string]any{"server": c.cfg.Name}, nil)
	}
	tools := make([]Tool, 0)
	for remote, err := range c.session.Tools(ctx, nil) {
		if err != nil {
			return nil, c.wrapSDKError("list MCP tools", err)
		}
		tool, err := convertSDKTool(remote)
		if err != nil {
			return nil, newError("MCP_INVALID_RESPONSE", "decode MCP tool definition", false, map[string]any{"server": c.cfg.Name}, err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func (c *sdkProtocolClient) callTool(ctx context.Context, name string, arguments map[string]any) (map[string]any, error) {
	if c.session == nil {
		return nil, newError("MCP_CONNECTION_FAILED", "MCP session is not initialized", true, map[string]any{"server": c.cfg.Name}, nil)
	}
	result, err := c.session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return nil, c.wrapSDKError("call MCP tool", err)
	}
	return jsonObject(result)
}

func (c *sdkProtocolClient) close() error {
	c.closeOnce.Do(func() {
		if c.session != nil {
			c.closeErr = errors.Join(c.closeErr, c.session.Close())
			c.session = nil
		}
		c.closeErr = errors.Join(c.closeErr, c.cleanupProcess())
	})
	return c.closeErr
}

// cleanupProcess has exactly one Wait owner, created only after protocol close.
// It lets the server perform EOF cleanup before escalating within its owned tree.
func (c *sdkProtocolClient) cleanupProcess() error {
	var result error
	if c.stdin != nil {
		if err := c.stdin.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			result = errors.Join(result, err)
		}
		c.stdin = nil
	}
	cmd, controller := c.command, c.controller
	// Detach ownership here so initialize-failure cleanup followed by close cannot
	// start another Wait, including when termination/reaping reports a timeout.
	c.command, c.controller = nil, nil
	if cmd == nil {
		if controller != nil {
			result = errors.Join(result, controller.Close())
		}
		return result
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	grace := time.NewTimer(stdioGracePeriod)
	defer grace.Stop()
	select {
	case err := <-waited:
		if !errors.Is(err, exec.ErrWaitDelay) {
			// Natural exit: never SIGKILL the group. Windows Close still releases the
			// owned job and enforces its existing kill-on-close descendant boundary.
			if controller != nil {
				result = errors.Join(result, controller.Close())
			}
			return errors.Join(result, processWaitError(err))
		}
		// Parent exited but descendant-held pipes failed to drain within WaitDelay.
		// Wait is already complete; clean the owned tree and preserve the pipe error.
		if controller != nil {
			result = errors.Join(result, controller.Terminate(), controller.Close())
		}
		return errors.Join(result, err)
	case <-grace.C:
	}
	var treeErr error
	if controller != nil {
		treeErr = errors.Join(controller.Terminate(), controller.Close())
	} else {
		treeErr = errors.New("MCP process cleanup has no owned process controller")
	}
	result = errors.Join(result, treeErr)
	if treeErr != nil {
		// Last resort solely to unblock this Wait. This does not establish successful
		// tree cleanup; the controller error remains in the returned result.
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			result = errors.Join(result, err)
		}
	}
	terminated := time.NewTimer(stdioTerminateWait)
	defer terminated.Stop()
	select {
	case err := <-waited:
		return errors.Join(result, processWaitError(err))
	case <-terminated.C:
		return errors.Join(result, errors.New("MCP owned process did not exit after bounded termination"))
	}
}

func processWaitError(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func (c *sdkProtocolClient) wrapSDKError(operation string, err error) error {
	if err == nil {
		return nil
	}
	details := map[string]any{"server": c.cfg.Name}
	if errors.Is(err, errMCPOriginRejected) {
		// net/http wraps redirect errors with the untrusted Location URL.
		return newError("MCP_CONNECTION_FAILED", errMCPOriginRejected.Error(), false, details, errMCPOriginRejected)
	}
	var authRequired *oauthclient.AuthRequiredError
	if errors.As(err, &authRequired) {
		return newError("MCP_AUTH_REQUIRED", authRequired.Error(), false, details, err)
	}
	if c.stderr != nil && c.stderr.String() != "" {
		details["stderr"] = c.stderr.String()
	}
	var rpcErr *sdkjsonrpc.Error
	if errors.As(err, &rpcErr) {
		details["rpc_code"] = rpcErr.Code
		if len(rpcErr.Data) > 0 {
			details["rpc_data"] = strings.TrimSpace(string(rpcErr.Data))
		}
		message := strings.TrimSpace(rpcErr.Message)
		if message == "" {
			message = fmt.Sprintf("MCP JSON-RPC error %d", rpcErr.Code)
		}
		// go-sdk 的 transport rejection 是请求级故障，并明确保留逻辑连接可用。
		if rpcErr.Code == sdkTransportRejectedCode {
			return newError("MCP_TRANSPORT_REJECTED", message, true, details, err)
		}
		return newError("MCP_REMOTE_ERROR", message, false, details, err)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return newError("MCP_TIMEOUT", operation+" timed out", true, details, err)
	}
	return newError("MCP_CONNECTION_FAILED", operation, true, details, err)
}

func convertSDKTool(remote *mcpsdk.Tool) (Tool, error) {
	if remote == nil {
		return Tool{}, errors.New("MCP tools/list returned a null tool")
	}
	input, err := jsonMap(remote.InputSchema)
	if err != nil {
		return Tool{}, fmt.Errorf("decode inputSchema: %w", err)
	}
	output, err := jsonMap(remote.OutputSchema)
	if err != nil {
		return Tool{}, fmt.Errorf("decode outputSchema: %w", err)
	}
	annotations, err := jsonMap(remote.Annotations)
	if err != nil {
		return Tool{}, fmt.Errorf("decode annotations: %w", err)
	}
	return Tool{
		Name:         remote.Name,
		Title:        remote.Title,
		Description:  remote.Description,
		InputSchema:  input,
		OutputSchema: output,
		Annotations:  annotations,
	}, nil
}

func jsonMap(value any) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	return jsonObject(value)
}

func jsonObject(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func stdioEnvironment(cfg ServerConfig) ([]string, error) {
	environment := envstore.MinimalSystemEnv()
	for childName, hostName := range cfg.EnvFromEnv {
		value, ok := os.LookupEnv(hostName)
		if !ok {
			return nil, newError(
				"MCP_CREDENTIAL_REQUIRED",
				"required MCP stdio environment variable is missing",
				false,
				map[string]any{"server": cfg.Name, "env": hostName},
				nil,
			)
		}
		environment[childName] = value
	}
	// portable Plugin mcp.json 中的 env 是包内公开默认值。用户的
	// env/mcp/<storage-key>.env 以及宿主保留变量在 RuntimeEnv 中最后覆盖。
	for key, value := range cfg.StaticEnv {
		environment[key] = value
	}
	for key, value := range cfg.RuntimeEnv {
		environment[key] = value
	}
	return envstore.Format(environment), nil
}

func resolveHTTPHeaders(cfg ServerConfig) (http.Header, error) {
	headers := make(http.Header, len(cfg.StaticHeaders)+len(cfg.HeaderEnv)+1)
	for name, value := range cfg.StaticHeaders {
		headers.Set(name, value)
	}
	headers.Set("User-Agent", config.ServerName+"/"+buildinfo.Version)
	requiredEnv := make(map[string]struct{}, len(cfg.RequiredEnv))
	for _, envName := range cfg.RequiredEnv {
		requiredEnv[envName] = struct{}{}
	}
	for header, envName := range cfg.HeaderEnv {
		value, ok := cfg.RuntimeEnv[envName]
		if !ok && cfg.SourceType != "plugin" {
			value, ok = os.LookupEnv(envName)
		}
		if !ok || value == "" {
			if cfg.SourceType == "plugin" {
				if _, required := requiredEnv[envName]; !required {
					// optional HeaderEnv 缺失时不要发送空 Header。空 Authorization 会覆盖
					// OAuthHandler 生成的 Bearer token，也会改变匿名服务的请求语义。
					continue
				}
			}
			return nil, newError(
				"MCP_CREDENTIAL_REQUIRED",
				"required MCP HTTP header environment variable is missing",
				false,
				map[string]any{"server": cfg.Name, "header": header, "env": envName},
				nil,
			)
		}
		headers.Set(header, value)
	}
	return headers, nil
}

var errMCPOriginRejected = errors.New("MCP HTTP request to another origin rejected")

func sameHTTPOrigin(a, b *url.URL) bool {
	if a == nil || b == nil || a.Hostname() == "" || b.Hostname() == "" {
		return false
	}
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		if strings.EqualFold(u.Scheme, "https") {
			return "443"
		}
		return "80"
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

type headerRoundTripper struct {
	headers  http.Header
	endpoint *url.URL
	base     http.RoundTripper
}

func (t headerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	// Guard every dispatch, including SDK-originated requests, before adding secrets.
	if !sameHTTPOrigin(t.endpoint, request.URL) {
		return nil, errMCPOriginRejected
	}
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	for name, values := range t.headers {
		clone.Header.Del(name)
		for _, value := range values {
			clone.Header.Add(name, value)
		}
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

type tailBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit, data: make([]byte, 0, limit)}
}

func (b *tailBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(data)
	if len(data) >= b.limit {
		b.data = append(b.data[:0], data[len(data)-b.limit:]...)
		return original, nil
	}
	if overflow := len(b.data) + len(data) - b.limit; overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
	}
	b.data = append(b.data, data...)
	return original, nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.data))
}
