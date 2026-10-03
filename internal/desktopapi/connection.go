package desktopapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

type ConnectionStatus struct {
	Mode           string `json:"mode"`
	Running        bool   `json:"running"`
	Ready          bool   `json:"ready"`
	StartupEnabled bool   `json:"startupEnabled"`
	PublicURL      string `json:"publicURL,omitempty"`
}
type ConnectionStatusResult struct {
	Status ConnectionStatus `json:"status"`
	Error  *APIError        `json:"error,omitempty"`
}
type ConnectionActionResult struct {
	Completed bool      `json:"completed"`
	Error     *APIError `json:"error,omitempty"`
}
type ConnectionService struct {
	runtimeRoot string
	rootError   error
	run         func(context.Context, []string, io.Writer, io.Writer) error
}

func NewConnectionService(root string) *ConnectionService {
	root, err := resolveRuntimeRoot(root)
	return &ConnectionService{runtimeRoot: root, rootError: err, run: desktopruntime.RunTunnelCommand}
}
func (s *ConnectionService) Status(ctx context.Context) ConnectionStatusResult {
	if s.rootError != nil {
		return ConnectionStatusResult{Error: safeServiceError("connection_root_unavailable", s.rootError)}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var output bytes.Buffer
	if err := s.run(ctx, []string{"status", "--runtime-root", s.runtimeRoot}, &output, io.Discard); err != nil {
		return ConnectionStatusResult{Error: safeContextServiceError(ctx, "connection_status_failed", err)}
	}
	var status desktopruntime.TunnelStatus
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		return ConnectionStatusResult{Error: NewError("connection_status_invalid", "Invalid connection status", ErrorCategoryInternal, false, nil)}
	}
	switch status.Mode {
	case "none", "quick", "named", "tailscale":
	default:
		return ConnectionStatusResult{Error: NewError("connection_mode_invalid", "Unsupported connection mode", ErrorCategoryUnavailable, false, nil)}
	}
	return ConnectionStatusResult{Status: ConnectionStatus{Mode: status.Mode, Running: status.Running, Ready: status.Ready, StartupEnabled: status.StartupEnabled, PublicURL: publicOrigin(status.PublicURL)}}
}
func (s *ConnectionService) Action(ctx context.Context, action string) ConnectionActionResult {
	switch action {
	case "start", "stop", "restart", "regenerate":
	default:
		return ConnectionActionResult{Error: NewError("connection_action_invalid", "Unsupported connection action; configuration remains native-only", ErrorCategoryValidation, false, nil)}
	}
	if s.rootError != nil {
		return ConnectionActionResult{Error: safeServiceError("connection_root_unavailable", s.rootError)}
	}
	operationCtx, finish, err := beginRuntimeMutation(ctx, s.runtimeRoot)
	if err != nil {
		return ConnectionActionResult{Error: safeContextServiceError(ctx, "connection_mutation_busy", err)}
	}
	defer finish()

	if action == "regenerate" {
		var output bytes.Buffer
		if err := s.run(operationCtx, []string{"status", "--runtime-root", s.runtimeRoot}, &output, io.Discard); err != nil {
			return ConnectionActionResult{Error: safeContextServiceError(operationCtx, "connection_status_failed", err)}
		}
		var status desktopruntime.TunnelStatus
		if err := json.Unmarshal(output.Bytes(), &status); err != nil {
			return ConnectionActionResult{Error: NewError("connection_status_invalid", "Invalid connection status", ErrorCategoryInternal, false, nil)}
		}
		if status.Mode != "quick" {
			return ConnectionActionResult{Error: NewError("connection_regenerate_unavailable", "Regenerate is available only for quick tunnels", ErrorCategoryUnavailable, false, nil)}
		}
	}

	if err := s.run(operationCtx, []string{action, "--runtime-root", s.runtimeRoot}, io.Discard, io.Discard); err != nil {
		return ConnectionActionResult{Error: safeContextServiceError(operationCtx, "connection_action_failed", err)}
	}
	return ConnectionActionResult{Completed: true}
}

// Only expose HTTPS origins. Reject credentials, query strings and paths rather
// than letting a misconfigured runtime carry secrets across the binding.
func publicOrigin(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return ""
	}
	return "https://" + parsed.Host
}
