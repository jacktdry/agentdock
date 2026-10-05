package desktopapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

// The renderer confirms these operations; confirmation metadata is not an
// authorization token. The local desktop binding remains the authority boundary.
func frozenConnectionOperations() []OperationCapability {
	result := []OperationCapability{}
	for _, name := range []string{"snapshot", "preflightPort", "revealOAuthPassword", "testPublicEndpoint", "updatePort", "configureTunnel", "setTunnelAutostart", "start", "stop", "restart", "regenerate"} {
		mutating := name == "updatePort" || name == "configureTunnel" || name == "setTunnelAutostart" || name == "start" || name == "stop" || name == "restart" || name == "regenerate"
		access := AccessRead
		if mutating {
			access = AccessMutating
		}
		confirm := mutating && name != "setTunnelAutostart" && name != "start"
		result = append(result, OperationCapability{Name: name, Access: access, RequiresConfirmation: confirm, Availability: AvailabilityAvailable})
	}
	return result
}

func connectionOperations(c desktopruntime.ConnectionConfig, selected desktopruntime.NextConnectionRuntime, selectionErr error, tunnel desktopruntime.TunnelObservation) []OperationCapability {
	operations := frozenConnectionOperations()
	for i := range operations {
		op := &operations[i]
		disable := func(reason string, native bool) {
			op.Availability = AvailabilityUnavailable
			op.DisabledReason = reason
			op.NativeRequired = native
		}
		if op.Name == "start" {
			op.RequiresConfirmation = c.Mode != "none" && (tunnel.Running == nil || !*tunnel.Running)
		}
		if op.Access == AccessRead {
			continue
		}
		// Available macOS mutations still delegate service effects to the native
		// SMAppService adapter; availability does not imply Go-owned registration.
		op.NativeRequired = runtime.GOOS == "darwin"
		if selectionErr != nil {
			disable("next_identity_unavailable", false)
			continue
		}
		if tunnel.RecoveryRequired {
			disable("recovery_required", false)
			continue
		}
		if runtime.GOOS == "darwin" && (op.Name == "stop" || op.Name == "setTunnelAutostart") {
			disable("native_service_management_required", true)
			continue
		}
		if op.Name == "setTunnelAutostart" && (tunnel.Autostart == "unavailable" || tunnel.Autostart == "unknown") {
			disable("tunnel_autostart_unavailable", false)
			continue
		}
		if op.Name == "regenerate" && c.Mode != "quick" {
			disable("quick_only", false)
		}
		if (op.Name == "start" || op.Name == "restart") && c.Mode == "none" {
			disable("tunnel_mode_none", false)
		}
		if (op.Name == "start" || op.Name == "restart") && c.Mode == "named" && c.Port != 8767 {
			disable("named_manual_route_required", false)
		}
	}
	return operations
}

func safeCoreHealth(value string) string {
	if value == "healthy" || value == "unhealthy" {
		return value
	}
	return "unknown"
}

func (s *ConnectionService) tunnelObservation(ctx context.Context, selectionErr error) desktopruntime.TunnelObservation {
	safe := desktopruntime.TunnelObservation{Autostart: "unavailable", TokenState: "unavailable", PublicEndpoint: "not_configured"}
	if selectionErr != nil {
		return safe
	}
	var output bytes.Buffer
	if s.run(ctx, []string{"status", "--runtime-root", s.runtimeRoot}, &output, io.Discard) != nil {
		return safe
	}
	var status desktopruntime.TunnelStatus
	if json.Unmarshal(output.Bytes(), &status) != nil || status.Observation == nil {
		return safe
	}
	o := status.Observation
	safe.Registered, safe.Running, safe.Connected = o.Registered, o.Running, o.Connected
	safe.RecoveryRequired = o.RecoveryRequired
	switch o.Autostart {
	case "enabled", "disabled", "unknown", "unavailable":
		safe.Autostart = o.Autostart
	}
	switch o.TokenState {
	case "stored", "missing", "unreadable", "unavailable":
		safe.TokenState = o.TokenState
	}
	switch o.PublicEndpoint {
	case "not_configured", "unchecked", "checking", "reachable", "unreachable", "unexpected_endpoint", "stale":
		safe.PublicEndpoint = o.PublicEndpoint
	}
	if o.RemoteRoute == "manual_route_required" {
		safe.RemoteRoute = o.RemoteRoute
	}
	return safe
}

func portRequest(c desktopruntime.ConnectionConfig, revision string, candidate int, selected desktopruntime.NextConnectionRuntime) desktopruntime.PortObservationRequest {
	return desktopruntime.PortObservationRequest{Runtime: selected.PortRuntime, Host: selected.BindHost, ConfiguredPort: c.Port, CandidatePort: candidate, ConfigRevision: revision}
}

func (s *ConnectionService) observePort(ctx context.Context, c desktopruntime.ConnectionConfig, revision string, candidate int, selected desktopruntime.NextConnectionRuntime, selectionErr error) desktopruntime.PortObservation {
	request := portRequest(c, revision, candidate, selected)
	if selectionErr == nil || candidate == 8765 || candidate == 8766 {
		return s.foundation.ObservePort(ctx, request)
	}
	return desktopruntime.PortObservation{ConfiguredPort: c.Port, ObservedPort: candidate, ConfigRevision: revision, State: desktopruntime.PortUnknown, ReasonCode: "next_identity_unavailable", ObservedAt: time.Now().UTC()}
}

type ConnectionPreflightResult struct {
	Observation desktopruntime.PortObservation `json:"observation"`
	Error       *APIError                      `json:"error,omitempty"`
}

func (s *ConnectionService) PreflightPort(ctx context.Context, candidatePort int) ConnectionPreflightResult {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, revision, err := s.config(ctx)
	if err != nil {
		return ConnectionPreflightResult{Error: err}
	}
	selected, selectionErr := s.foundation.SelectRuntime(ctx, s.runtimeRoot)
	observation := s.observePort(ctx, c, revision, candidatePort, selected, selectionErr)
	if !s.unchanged(ctx, revision) {
		return ConnectionPreflightResult{Error: connectionFailure("connection_config_stale")}
	}
	return ConnectionPreflightResult{Observation: observation}
}

type ConnectionPortRequest struct {
	CandidatePort  int    `json:"candidatePort"`
	ConfigRevision string `json:"configRevision"`
}

type ConnectionTunnelRequest struct {
	Mode           string `json:"mode"`
	NamedOrigin    string `json:"namedOrigin"`
	NewToken       string `json:"newToken,omitempty"`
	ConfigRevision string `json:"configRevision"`
}

type ConnectionAutostartRequest struct {
	Enabled        bool   `json:"enabled"`
	ConfigRevision string `json:"configRevision"`
}

func connectionFailure(code string) *APIError {
	category := ErrorCategoryUnavailable
	if code == "connection_request_invalid" {
		category = ErrorCategoryValidation
	}
	return NewError(code, "Connection operation unavailable; refresh configuration and check the disabled reason", category, code == "connection_config_stale", nil)
}

type connectionApply func(context.Context, desktopruntime.ConnectionConfig, string, desktopruntime.NextConnectionRuntime, desktopruntime.TunnelObservation, *ConnectionActionResult) error

func (s *ConnectionService) mutate(ctx context.Context, revision, operation string, apply connectionApply) (result ConnectionActionResult) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		result.Error = connectionFailure("connection_operation_unavailable")
		return
	}
	result.OperationID, result.Phase = hex.EncodeToString(id[:]), "failed"
	if s.rootError != nil {
		result.Error = connectionReadError()
		return
	}
	// Reject unowned roots and stale requests before creating any lock directory.
	_, before, readErr := s.config(ctx)
	if readErr != nil {
		result.Error = readErr
		return
	}
	if _, err := s.foundation.SelectRuntime(ctx, s.runtimeRoot); err != nil {
		result.Error = connectionFailure("next_identity_unavailable")
		return
	}
	if revision == "" || revision != before {
		result.Error = connectionFailure("connection_config_stale")
		return
	}
	operationCtx, finish, err := beginRuntimeMutation(ctx, s.runtimeRoot)
	if err != nil {
		result.Error = safeContextServiceError(ctx, "connection_mutation_busy", err)
		return
	}
	defer finish()
	c, current, apiErr := s.config(operationCtx)
	if apiErr != nil {
		result.Error = apiErr
		return
	}
	result.ConfigRevision = current
	if revision == "" || revision != current {
		result.Error = connectionFailure("connection_config_stale")
		return
	}
	selected, selectionErr := s.foundation.SelectRuntime(operationCtx, s.runtimeRoot)
	tunnel := s.tunnelObservation(operationCtx, selectionErr)
	for _, capability := range connectionOperations(c, selected, selectionErr, tunnel) {
		if capability.Name == operation && capability.Availability != AvailabilityAvailable {
			result.Error = connectionFailure(capability.DisabledReason)
			return
		}
	}
	// No read preflight or earlier snapshot can authorize this mutation.
	if !s.unchanged(operationCtx, revision) {
		result.Error = connectionFailure("connection_config_stale")
		return
	}
	if operation == "configureTunnel" || operation == "start" || operation == "restart" || operation == "regenerate" {
		observation, err := s.foundation.PreflightPort(operationCtx, portRequest(c, current, c.Port, selected))
		result.PortObservation = &observation
		if err != nil {
			result.Error = connectionFailure(observation.ReasonCode)
			return
		}
	}
	if err := apply(operationCtx, c, revision, selected, tunnel, &result); err != nil {
		if result.Error == nil {
			result.Error = safeContextServiceError(operationCtx, "connection_mutation_failed", err)
		}
		var outcome *desktopruntime.TunnelMutationError
		if errors.As(err, &outcome) {
			result.Phase = outcome.Phase
		}
		if errors.Is(err, desktopruntime.ErrTunnelRecoveryRequired) {
			result.Phase = "recovery_required"
			result.Error = connectionFailure("recovery_required")
		}
		if errors.Is(err, desktopruntime.ErrNamedManualRouteRequired) {
			result.Error = connectionFailure("named_manual_route_required")
		}
		return
	}
	waiting := result.Phase == "waiting_readiness"
	result.Completed, result.Phase = true, "applied"
	if waiting {
		result.Phase = "waiting_readiness"
	}
	if config, after, e := s.config(operationCtx); e == nil {
		result.TunnelGeneration = config.TunnelGeneration
		result.ConfigRevision = after
	}
	return
}

func (s *ConnectionService) UpdatePort(ctx context.Context, request ConnectionPortRequest) ConnectionActionResult {
	return s.mutate(ctx, request.ConfigRevision, "updatePort", func(ctx context.Context, c desktopruntime.ConnectionConfig, revision string, selected desktopruntime.NextConnectionRuntime, tunnel desktopruntime.TunnelObservation, result *ConnectionActionResult) error {
		if c.Mode == "named" && request.CandidatePort != 8767 {
			result.Error = connectionFailure("named_manual_route_required")
			return desktopruntime.ErrNamedManualRouteRequired
		}
		settings, err := s.foundation.ReadBasic(ctx, s.runtimeRoot)
		if err != nil {
			return err
		}
		if settings.Port != c.Port {
			result.Error = connectionFailure("connection_config_stale")
			return desktopruntime.ErrPortPreflight
		}
		observation, err := s.foundation.PreflightPort(ctx, portRequest(c, revision, request.CandidatePort, selected))
		result.PortObservation = &observation
		if err != nil {
			result.Error = connectionFailure(observation.ReasonCode)
			return err
		}
		if selected.Running == nil {
			return desktopruntime.ErrNextIdentityUnavailable
		}
		wasRunning := *selected.Running
		old := settings
		settings.Port = request.CandidatePort
		// UpdateBasicSettings owns local target coherence, stopped-state preservation,
		// final Core bind/restart failure, and Quick invalidation. It does not take
		// the outer Desktop lock or wait for Quick URL publication.
		if err := s.foundation.UpdateBasic(ctx, s.runtimeRoot, settings); err != nil {
			return err
		}
		// Re-select after restart: the previous PID cannot authorize the new listener.
		afterConfig, afterRevision, configErr := s.config(ctx)
		fresh, selectionErr := s.foundation.SelectRuntime(ctx, s.runtimeRoot)
		verificationErr := error(nil)
		if configErr != nil || selectionErr != nil || fresh.Running == nil || afterConfig.Port != settings.Port {
			verificationErr = desktopruntime.ErrPortPreflight
		} else {
			final, err := s.foundation.PreflightPort(ctx, portRequest(afterConfig, afterRevision, settings.Port, fresh))
			result.PortObservation = &final
			if err != nil || (wasRunning && (!*fresh.Running || final.State != desktopruntime.PortOwnedByNext)) ||
				(!wasRunning && (*fresh.Running || final.State != desktopruntime.PortAvailable)) {
				verificationErr = desktopruntime.ErrPortPreflight
			}
		}
		if verificationErr != nil {
			recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
			defer cancel()
			if err := s.foundation.UpdateBasic(recovery, s.runtimeRoot, old); err != nil {
				result.Phase = "recovery_required"
				return desktopruntime.ErrTunnelRecoveryRequired
			}
			// UpdateBasic preserves the lifecycle it observes at rollback time. If
			// the failed port transition already stopped a previously-running Core,
			// restore that original lifecycle explicitly while the outer mutation
			// lock is still held.
			if wasRunning {
				if err := s.foundation.CoreAction(recovery, s.runtimeRoot, "restart"); err != nil {
					result.Phase = "recovery_required"
					return desktopruntime.ErrTunnelRecoveryRequired
				}
			}
			rollbackConfig, rollbackRevision, configErr := s.config(recovery)
			rollbackRuntime, selectionErr := s.foundation.SelectRuntime(recovery, s.runtimeRoot)
			if configErr != nil || selectionErr != nil || rollbackRuntime.Running == nil || rollbackConfig.Port != old.Port {
				result.Phase = "recovery_required"
				return desktopruntime.ErrTunnelRecoveryRequired
			}
			rollbackObservation, rollbackErr := s.foundation.PreflightPort(recovery, portRequest(rollbackConfig, rollbackRevision, old.Port, rollbackRuntime))
			result.PortObservation = &rollbackObservation
			if wasRunning {
				if rollbackErr != nil || !*rollbackRuntime.Running || rollbackObservation.State != desktopruntime.PortOwnedByNext {
					result.Phase = "recovery_required"
					return desktopruntime.ErrTunnelRecoveryRequired
				}
			} else if rollbackErr != nil || *rollbackRuntime.Running || rollbackObservation.State != desktopruntime.PortAvailable {
				result.Phase = "recovery_required"
				return desktopruntime.ErrTunnelRecoveryRequired
			}
			result.Phase = "rolled_back"
			return verificationErr
		}
		if c.Mode == "quick" && tunnel.Running != nil && *tunnel.Running && old.Port != settings.Port {
			result.Phase = "waiting_readiness"
		}
		return nil
	})
}

func (s *ConnectionService) Action(ctx context.Context, action string, configRevision string) ConnectionActionResult {
	switch action {
	case "start", "stop", "restart", "regenerate":
	default:
		return ConnectionActionResult{Phase: "failed", Error: connectionFailure("connection_request_invalid")}
	}
	return s.mutate(ctx, configRevision, action, func(ctx context.Context, c desktopruntime.ConnectionConfig, _ string, _ desktopruntime.NextConnectionRuntime, tunnel desktopruntime.TunnelObservation, result *ConnectionActionResult) error {
		err := s.run(ctx, []string{action, "--runtime-root", s.runtimeRoot}, io.Discard, io.Discard)
		if err == nil && c.Mode == "quick" && action != "stop" &&
			!(action == "start" && tunnel.Running != nil && *tunnel.Running && c.PublicOrigin != "") {
			result.Phase = "waiting_readiness"
		}
		return err
	})
}

func (s *ConnectionService) ConfigureTunnel(ctx context.Context, request ConnectionTunnelRequest) ConnectionActionResult {
	return s.mutate(ctx, request.ConfigRevision, "configureTunnel", func(ctx context.Context, c desktopruntime.ConnectionConfig, _ string, _ desktopruntime.NextConnectionRuntime, tunnel desktopruntime.TunnelObservation, result *ConnectionActionResult) error {
		invalid := func() error {
			result.Error = connectionFailure("connection_request_invalid")
			return errors.New("invalid request")
		}
		if request.Mode != "none" && request.Mode != "quick" && request.Mode != "named" {
			return invalid()
		}
		if request.Mode != "named" && (request.NamedOrigin != "" || request.NewToken != "") {
			return invalid()
		}
		if request.Mode == "named" && c.Port != 8767 {
			result.Error = connectionFailure("named_manual_route_required")
			return desktopruntime.ErrNamedManualRouteRequired
		}
		if runtime.GOOS == "darwin" && request.Mode == "none" && (tunnel.Running == nil || *tunnel.Running) {
			result.Error = connectionFailure("native_service_management_required")
			return errors.New("native required")
		}
		args := []string{"configure", "--runtime-root", s.runtimeRoot, "--mode", request.Mode}
		if request.Mode == "named" {
			origin := publicOrigin(request.NamedOrigin)
			if origin == "" {
				return invalid()
			}
			args = append(args, "--server-url", origin)
			if request.NewToken == "" {
				if tunnel.TokenState != "stored" {
					result.Error = connectionFailure("tunnel_token_unavailable")
					return errors.New("token unavailable")
				}
			} else {
				token := strings.TrimSpace(request.NewToken)
				if token == "" || len(request.NewToken) > 16*1024 || strings.ContainsAny(token, "\r\n\x00") {
					return invalid()
				}
				path, cleanup, err := privateConnectionTokenFile(s.runtimeRoot, token)
				if err != nil {
					return err
				}
				defer cleanup()
				args = append(args, "--token-file", path)
			}
		}
		err := s.run(ctx, args, io.Discard, io.Discard)
		if err == nil && request.Mode == "quick" && tunnel.Running != nil && *tunnel.Running {
			result.Phase = "waiting_readiness"
		}
		return err
	})
}

func privateConnectionTokenFile(root, token string) (string, func(), error) {
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil || canonical != filepath.Clean(root) {
		return "", nil, errors.New("unsafe token root")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
		return "", nil, errors.New("unsafe token root")
	}
	file, err := os.CreateTemp(root, ".connection-token-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(file.Name()) }
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, err
	}
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		_ = file.Close()
		cleanup()
		return "", nil, errors.New("unsafe token file")
	}
	_, writeErr := io.WriteString(file, token)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		cleanup()
		return "", nil, err
	}
	return file.Name(), cleanup, nil
}

func (s *ConnectionService) SetTunnelAutostart(ctx context.Context, request ConnectionAutostartRequest) ConnectionActionResult {
	return s.mutate(ctx, request.ConfigRevision, "setTunnelAutostart", func(ctx context.Context, _ desktopruntime.ConnectionConfig, _ string, _ desktopruntime.NextConnectionRuntime, _ desktopruntime.TunnelObservation, _ *ConnectionActionResult) error {
		return s.run(ctx, []string{"autostart", "--runtime-root", s.runtimeRoot, "--enabled", strconv.FormatBool(request.Enabled)}, io.Discard, io.Discard)
	})
}
