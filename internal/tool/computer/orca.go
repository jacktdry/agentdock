package computer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const defaultProviderTimeout = 30 * time.Second

type OrcaProviderConfig struct {
	Command string
	Timeout time.Duration
}

type OrcaProvider struct {
	command string
	timeout time.Duration
	run     func(context.Context, string, []string, string) ([]byte, []byte, error)
}

type orcaEnvelope struct {
	ID     string         `json:"id"`
	OK     bool           `json:"ok"`
	Result map[string]any `json:"result"`
	Error  *struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data,omitempty"`
	} `json:"error,omitempty"`
	Meta map[string]any `json:"_meta,omitempty"`
}

func NewOrcaProvider(cfg OrcaProviderConfig) (*OrcaProvider, error) {
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		command = resolveOrcaCommand()
	}
	if _, err := exec.LookPath(command); err != nil {
		return nil, computerError(ErrProviderUnavailable, "Orca computer-use provider is unavailable", "provider", &ErrorDetails{Provider: ProviderOrca, Reason: command}, err)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultProviderTimeout
	}
	return &OrcaProvider{command: command, timeout: timeout, run: runOrcaCommand}, nil
}

func resolveOrcaCommand() string {
	if value := strings.TrimSpace(os.Getenv("ORCA_CLI_COMMAND")); value != "" {
		return value
	}
	if strings.TrimSpace(os.Getenv("ORCA_DEV_REPO_ROOT")) != "" {
		return "orca-dev"
	}
	if runtime.GOOS == "linux" {
		return "orca-ide"
	}
	return "orca"
}

func (p *OrcaProvider) ID() ProviderID { return ProviderOrca }

func (p *OrcaProvider) FrontmostApp(ctx context.Context) (*AppIdentity, error) {
	return platformFrontmostApp(ctx)
}

func (p *OrcaProvider) Observe(ctx context.Context, req ObservationRequest) (map[string]any, error) {
	args := []string{"computer"}
	switch req.Action {
	case "capabilities":
		args = append(args, "capabilities")
	case "permissions":
		args = append(args, "permissions")
	case "list_apps":
		args = append(args, "list-apps")
	case "list_windows":
		if strings.TrimSpace(req.App) == "" {
			return nil, invalidArgument("list_windows requires app", req.Action)
		}
		args = append(args, "list-windows", "--app", req.App)
	case "get_app_state":
		if strings.TrimSpace(req.App) == "" {
			return nil, invalidArgument("get_app_state requires app", req.Action)
		}
		args = append(args, "get-app-state", "--app", req.App)
		args = appendWindowSelector(args, req.WindowID, req.WindowIndex)
		if req.RestoreWindow {
			args = append(args, "--restore-window")
		}
		if req.NoScreenshot {
			args = append(args, "--no-screenshot")
		}
	default:
		return nil, unsupportedAction(req.Action)
	}
	return p.invoke(ctx, args, "")
}

func (p *OrcaProvider) Act(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if strings.TrimSpace(req.App) == "" {
		return nil, invalidArgument("computer action requires app", req.Action)
	}
	args := []string{"computer", orcaActionName(req.Action), "--app", req.App}
	if args[1] == "" {
		return nil, unsupportedAction(req.Action)
	}
	args = appendWindowSelector(args, req.WindowID, req.WindowIndex)
	stdin := ""
	switch req.Action {
	case "click":
		if req.ElementIndex != nil {
			args = append(args, "--element-index", fmt.Sprint(*req.ElementIndex))
		} else if req.X != nil && req.Y != nil {
			args = append(args, "--x", fmt.Sprint(*req.X), "--y", fmt.Sprint(*req.Y))
		} else {
			return nil, invalidArgument("click requires element_index or x/y", req.Action)
		}
		if req.Modifiers != "" {
			args = append(args, "--modifiers", req.Modifiers)
		}
		if req.MouseButton != "" {
			args = append(args, "--mouse-button", req.MouseButton)
		}
	case "set_value":
		if req.ElementIndex == nil {
			return nil, invalidArgument("set_value requires element_index", req.Action)
		}
		args = append(args, "--element-index", fmt.Sprint(*req.ElementIndex), "--value-stdin")
		stdin = req.Value
	case "type_text", "paste_text":
		args = append(args, "--text-stdin")
		stdin = req.Text
	case "press_key", "hotkey":
		if strings.TrimSpace(req.Key) == "" {
			return nil, invalidArgument(req.Action+" requires key", req.Action)
		}
		args = append(args, "--key", req.Key)
	case "scroll":
		if strings.TrimSpace(req.Direction) == "" {
			return nil, invalidArgument("scroll requires direction", req.Action)
		}
		if req.ElementIndex != nil {
			args = append(args, "--element-index", fmt.Sprint(*req.ElementIndex))
		} else if req.X != nil && req.Y != nil {
			args = append(args, "--x", fmt.Sprint(*req.X), "--y", fmt.Sprint(*req.Y))
		} else {
			return nil, invalidArgument("scroll requires element_index or x/y", req.Action)
		}
		args = append(args, "--direction", req.Direction)
	case "drag":
		switch {
		case req.FromElementIndex != nil && req.ToElementIndex != nil:
			args = append(args, "--from-element-index", fmt.Sprint(*req.FromElementIndex), "--to-element-index", fmt.Sprint(*req.ToElementIndex))
		case req.FromX != nil && req.FromY != nil && req.ToX != nil && req.ToY != nil:
			args = append(args, "--from-x", fmt.Sprint(*req.FromX), "--from-y", fmt.Sprint(*req.FromY), "--to-x", fmt.Sprint(*req.ToX), "--to-y", fmt.Sprint(*req.ToY))
		default:
			return nil, invalidArgument("drag requires from/to element indexes or coordinates", req.Action)
		}
	case "perform_secondary_action":
		if req.ElementIndex == nil || strings.TrimSpace(req.SecondaryAction) == "" {
			return nil, invalidArgument("perform_secondary_action requires element_index and secondary_action", req.Action)
		}
		args = append(args, "--element-index", fmt.Sprint(*req.ElementIndex), "--action", req.SecondaryAction)
	}
	if req.RestoreWindow {
		args = append(args, "--restore-window")
	}
	if req.NoScreenshot {
		args = append(args, "--no-screenshot")
	}
	return p.invoke(ctx, args, stdin)
}

func appendWindowSelector(args []string, windowID *int64, windowIndex *int) []string {
	if windowID != nil {
		return append(args, "--window-id", fmt.Sprint(*windowID))
	}
	if windowIndex != nil {
		return append(args, "--window-index", fmt.Sprint(*windowIndex))
	}
	return args
}

func orcaActionName(action string) string {
	switch action {
	case "click":
		return "click"
	case "set_value":
		return "set-value"
	case "type_text":
		return "type-text"
	case "paste_text":
		return "paste-text"
	case "press_key":
		return "press-key"
	case "hotkey":
		return "hotkey"
	case "scroll":
		return "scroll"
	case "drag":
		return "drag"
	case "perform_secondary_action":
		return "perform-secondary-action"
	default:
		return ""
	}
}

func (p *OrcaProvider) invoke(parent context.Context, args []string, stdin string) (map[string]any, error) {
	timeout := p.timeout
	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	stdout, stderr, runErr := p.run(ctx, p.command, append(args, "--json"), stdin)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, computerError(ErrTimeout, "Orca computer-use operation timed out", "provider", &ErrorDetails{Provider: ProviderOrca, Action: strings.Join(args, " ")}, ctx.Err())
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, computerError(ErrProviderFailed, "Orca computer-use operation cancelled", "provider", &ErrorDetails{Provider: ProviderOrca}, ctx.Err())
	}
	var envelope orcaEnvelope
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		cause := runErr
		if cause == nil {
			cause = err
		}
		return nil, computerError(ErrProviderFailed, "Orca returned an invalid JSON response", "provider", &ErrorDetails{Provider: ProviderOrca, Reason: truncateDiagnostic(string(stderr), 600)}, cause)
	}
	if !envelope.OK {
		reason := "provider reported failure"
		providerCode := ""
		if envelope.Error != nil {
			providerCode = envelope.Error.Code
			reason = envelope.Error.Code + ": " + envelope.Error.Message
		}
		return nil, computerError(ErrProviderFailed, "Orca computer-use operation failed", "provider", &ErrorDetails{Provider: ProviderOrca, ProviderErrorCode: providerCode, Reason: reason}, runErr)
	}
	if envelope.Result == nil {
		envelope.Result = map[string]any{}
	}
	return envelope.Result, nil
}

func runOrcaCommand(ctx context.Context, command string, args []string, stdin string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func invalidArgument(message, action string) *Error {
	return computerError(ErrInvalidArgument, message, "validation", &ErrorDetails{Action: action}, nil)
}

func unsupportedAction(action string) *Error {
	return computerError(ErrUnsupportedAction, "unsupported computer control action", "validation", &ErrorDetails{Action: action}, nil)
}

func truncateDiagnostic(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
