package desktopapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

const runtimeRootEnv = "AGENTDOCK_RUNTIME_ROOT"

type RuntimeAction string

const (
	RuntimeActionStart   RuntimeAction = "start"
	RuntimeActionStop    RuntimeAction = "stop"
	RuntimeActionRestart RuntimeAction = "restart"
)

type RuntimeStatus struct {
	Running        bool `json:"running"`
	Healthy        bool `json:"healthy"`
	StartupEnabled bool `json:"startupEnabled"`
	NexusConnected bool `json:"nexusConnected"`
}

type RuntimeStatusResult struct {
	RuntimeRoot string        `json:"runtimeRoot"`
	Status      RuntimeStatus `json:"status"`
	Error       *APIError     `json:"error,omitempty"`
}

type RuntimeActionResult struct {
	Action    RuntimeAction `json:"action"`
	Completed bool          `json:"completed"`
	Error     *APIError     `json:"error,omitempty"`
}

type RuntimeService struct {
	runtimeRoot string
	rootError   error
}

func NewRuntimeService(explicitRoot string) *RuntimeService {
	root, err := resolveRuntimeRoot(explicitRoot)
	return &RuntimeService{runtimeRoot: root, rootError: err}
}

func resolveRuntimeRoot(explicitRoot string) (string, error) {
	if root := strings.TrimSpace(explicitRoot); root != "" {
		return root, nil
	}
	if root := strings.TrimSpace(os.Getenv(runtimeRootEnv)); root != "" {
		return root, nil
	}
	if root := strings.TrimSpace(desktopruntime.DefaultRuntimeRoot()); root != "" {
		return root, nil
	}
	return "", errors.New("runtime root is unavailable on this platform; pass --runtime-root or AGENTDOCK_RUNTIME_ROOT")
}

func (s *RuntimeService) Status(ctx context.Context) RuntimeStatusResult {
	if s.rootError != nil {
		return RuntimeStatusResult{Error: safeServiceError("runtime_root_unavailable", s.rootError)}
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	if err := desktopruntime.RunServiceCommand(ctx, []string{"status", "--runtime-root", s.runtimeRoot}, &stdout, io.Discard); err != nil {
		return RuntimeStatusResult{
			RuntimeRoot: s.runtimeRoot,
			Error:       safeContextServiceError(ctx, "runtime_status_failed", err),
		}
	}

	var status desktopruntime.ServiceStatus
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		return RuntimeStatusResult{
			RuntimeRoot: s.runtimeRoot,
			Error:       NewError("runtime_status_decode_failed", "Invalid runtime status", ErrorCategoryInternal, false, nil),
		}
	}
	return RuntimeStatusResult{
		RuntimeRoot: s.runtimeRoot,
		Status: RuntimeStatus{
			Running:        status.Running,
			Healthy:        status.Healthy,
			StartupEnabled: status.StartupEnabled,
			NexusConnected: status.NexusConnected,
		},
	}
}

func (s *RuntimeService) Action(ctx context.Context, action RuntimeAction) RuntimeActionResult {
	if action != RuntimeActionStart && action != RuntimeActionStop && action != RuntimeActionRestart {
		return RuntimeActionResult{
			Action: action,
			Error: NewError(
				"runtime_action_invalid",
				fmt.Sprintf("unsupported runtime action %q", action),
				ErrorCategoryValidation,
				false,
				nil,
			),
		}
	}
	if s.rootError != nil {
		return RuntimeActionResult{
			Action: action,
			Error:  safeServiceError("runtime_root_unavailable", s.rootError),
		}
	}

	operationCtx, finish, err := beginRuntimeMutation(ctx, s.runtimeRoot)
	if err != nil {
		return RuntimeActionResult{Action: action, Error: safeContextServiceError(ctx, "runtime_mutation_busy", err)}
	}
	defer finish()

	if err := desktopruntime.RunServiceCommand(operationCtx, []string{string(action), "--runtime-root", s.runtimeRoot}, io.Discard, io.Discard); err != nil {
		return RuntimeActionResult{
			Action: action,
			Error:  safeContextServiceError(operationCtx, "runtime_action_failed", err),
		}
	}
	return RuntimeActionResult{Action: action, Completed: true}
}
