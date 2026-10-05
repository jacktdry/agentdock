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
	Completed        bool                            `json:"completed"`
	OperationID      string                          `json:"operationId"`
	Phase            string                          `json:"phase"`
	TunnelGeneration string                          `json:"tunnelGeneration,omitempty"`
	ConfigRevision   string                          `json:"configRevision,omitempty"`
	PortObservation  *desktopruntime.PortObservation `json:"portObservation,omitempty"`
	Error            *APIError                       `json:"error,omitempty"`
}
type ConnectionService struct {
	runtimeRoot string
	rootError   error
	run         func(context.Context, []string, io.Writer, io.Writer) error
	foundation  ConnectionDependencies
}

func NewConnectionService(root string) *ConnectionService {
	root, err := resolveRuntimeRoot(root)
	return &ConnectionService{runtimeRoot: root, rootError: err, run: desktopruntime.RunTunnelCommandLocked, foundation: defaultConnectionDependencies()}
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

// Only expose HTTPS origins. Reject credentials, query strings and paths rather
// than letting a misconfigured runtime carry secrets across the binding.
func publicOrigin(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" || parsed.RawPath != "" || (parsed.Path != "" && parsed.Path != "/") {
		return ""
	}
	return "https://" + parsed.Host
}
