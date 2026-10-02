package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/desktopruntime"
)

const runtimeRootEnv = "AGENTDOCK_RUNTIME_ROOT"

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
		return RuntimeStatusResult{Error: apiError("runtime_root_unavailable", s.rootError)}
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	if err := desktopruntime.RunServiceCommand(ctx, []string{"status", "--runtime-root", s.runtimeRoot}, &stdout, &stderr); err != nil {
		return RuntimeStatusResult{
			RuntimeRoot: s.runtimeRoot,
			Error:       apiError("runtime_status_failed", joinCommandError(err, stderr.String())),
		}
	}

	var status desktopruntime.ServiceStatus
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		return RuntimeStatusResult{
			RuntimeRoot: s.runtimeRoot,
			Error:       apiError("runtime_status_decode_failed", err),
		}
	}
	return RuntimeStatusResult{RuntimeRoot: s.runtimeRoot, Status: status}
}

func (s *RuntimeService) Action(ctx context.Context, action string) RuntimeActionResult {
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "start" && action != "stop" && action != "restart" {
		return RuntimeActionResult{Action: action, Error: apiError("runtime_action_invalid", fmt.Errorf("unsupported action %q", action))}
	}
	if s.rootError != nil {
		return RuntimeActionResult{Action: action, Error: apiError("runtime_root_unavailable", s.rootError)}
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	if err := desktopruntime.RunServiceCommand(ctx, []string{action, "--runtime-root", s.runtimeRoot}, &stdout, &stderr); err != nil {
		return RuntimeActionResult{Action: action, Error: apiError("runtime_action_failed", joinCommandError(err, stderr.String()))}
	}
	return RuntimeActionResult{Action: action, Completed: true}
}

func joinCommandError(err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, stderr)
}
