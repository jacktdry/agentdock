package computer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	toolcore "github.com/uvwt/agentdock/internal/tool/core"
)

type Service struct {
	broker  *Broker
	initErr error
}

func NewService() *Service {
	provider, err := NewOrcaProvider(OrcaProviderConfig{})
	if err != nil {
		return &Service{initErr: err}
	}
	broker, err := NewBroker(provider)
	if err != nil {
		return &Service{initErr: err}
	}
	return &Service{broker: broker}
}

func NewServiceWithBroker(broker *Broker) *Service { return &Service{broker: broker} }
func (s *Service) Broker() *Broker {
	if s == nil {
		return nil
	}
	return s.broker
}

func (s *Service) Close() error {
	if s == nil || s.broker == nil {
		return nil
	}
	return s.broker.Close()
}

func (s *Service) HandleSession(_ context.Context, args map[string]any) (toolcore.Result, error) {
	if err := s.ready(); err != nil {
		return computerFailure(err), nil
	}
	action, err := requiredString(args, "action")
	if err != nil {
		return computerFailure(err), nil
	}
	switch action {
	case "acquire":
		capability := Capability(optionalString(args, "capability", string(CapabilityObserve)))
		foreground := ForegroundPolicy(optionalString(args, "foreground", string(ForegroundForbidden)))
		owner := OwnerScope{Kind: OwnerDirect, OwnerTaskID: optionalString(args, "owner_task_id", "")}
		meta, err := s.broker.Acquire(AcquireRequest{Owner: owner, Capability: capability, ForegroundPolicy: foreground})
		if err != nil {
			return computerFailure(err), nil
		}
		return computerSuccess(meta), nil
	case "release":
		id, err := requiredString(args, "session_id")
		if err != nil {
			return computerFailure(err), nil
		}
		meta, err := s.broker.ReleaseDirect(id)
		if err != nil {
			return computerFailure(err), nil
		}
		return computerSuccess(meta), nil
	default:
		return computerFailure(unsupportedAction(action)), nil
	}
}

func (s *Service) HandleObserve(ctx context.Context, args map[string]any) (toolcore.Result, error) {
	if err := s.ready(); err != nil {
		return computerFailure(err), nil
	}
	id, err := requiredString(args, "session_id")
	if err != nil {
		return computerFailure(err), nil
	}
	action, err := requiredString(args, "action")
	if err != nil {
		return computerFailure(err), nil
	}
	req := ObservationRequest{Action: action, App: optionalString(args, "app", ""), RestoreWindow: optionalBool(args, "restore_window"), NoScreenshot: optionalBool(args, "no_screenshot")}
	if req.WindowID, err = optionalInt64Pointer(args, "window_id"); err != nil {
		return computerFailure(err), nil
	}
	if req.WindowIndex, err = optionalIntPointer(args, "window_index"); err != nil {
		return computerFailure(err), nil
	}
	if req.Timeout, err = timeoutArg(args); err != nil {
		return computerFailure(err), nil
	}
	result, err := s.broker.ObserveDirect(ctx, id, req)
	if err != nil {
		return computerFailure(err), nil
	}
	return computerSuccess(result), nil
}

func (s *Service) HandleAct(ctx context.Context, args map[string]any) (toolcore.Result, error) {
	if err := s.ready(); err != nil {
		return computerFailure(err), nil
	}
	id, err := requiredString(args, "session_id")
	if err != nil {
		return computerFailure(err), nil
	}
	action, err := requiredString(args, "action")
	if err != nil {
		return computerFailure(err), nil
	}
	req := ActionRequest{
		Action: action, App: optionalString(args, "app", ""), Value: optionalString(args, "value", ""), Text: optionalString(args, "text", ""), Key: optionalString(args, "key", ""),
		Direction: optionalString(args, "direction", ""), SecondaryAction: optionalString(args, "secondary_action", ""), Modifiers: optionalString(args, "modifiers", ""), MouseButton: optionalString(args, "mouse_button", ""), RestoreWindow: optionalBool(args, "restore_window"), NoScreenshot: optionalBool(args, "no_screenshot"),
	}
	if req.WindowID, err = optionalInt64Pointer(args, "window_id"); err != nil {
		return computerFailure(err), nil
	}
	if req.WindowIndex, err = optionalIntPointer(args, "window_index"); err != nil {
		return computerFailure(err), nil
	}
	for key, dst := range map[string]**int{"element_index": &req.ElementIndex, "x": &req.X, "y": &req.Y, "from_element_index": &req.FromElementIndex, "to_element_index": &req.ToElementIndex, "from_x": &req.FromX, "from_y": &req.FromY, "to_x": &req.ToX, "to_y": &req.ToY} {
		value, parseErr := optionalIntPointer(args, key)
		if parseErr != nil {
			return computerFailure(parseErr), nil
		}
		*dst = value
	}
	if req.Timeout, err = timeoutArg(args); err != nil {
		return computerFailure(err), nil
	}
	result, err := s.broker.ActDirect(ctx, id, req)
	if err != nil {
		return computerFailure(err), nil
	}
	return computerSuccess(result), nil
}

func (s *Service) ready() error {
	if s == nil {
		return computerError(ErrProviderUnavailable, "computer control service unavailable", "service", nil, nil)
	}
	if s.initErr != nil {
		return s.initErr
	}
	if s.broker == nil {
		return computerError(ErrProviderUnavailable, "computer control broker unavailable", "service", nil, nil)
	}
	return nil
}

func computerSuccess(value any) toolcore.Result {
	encoded, _ := json.Marshal(value)
	result := toolcore.Result{}
	_ = json.Unmarshal(encoded, &result)
	result["computer_ok"] = true
	return result
}

func computerFailure(err error) toolcore.Result {
	var typed *Error
	if !errors.As(err, &typed) {
		typed = computerError(ErrProviderFailed, "computer control operation failed", "runtime", nil, err)
	}
	value := map[string]any{"code": typed.Code, "message": typed.Message, "phase": typed.Phase}
	if typed.Details != nil {
		value["details"] = typed.Details
	}
	return toolcore.Result{"computer_ok": false, "code": typed.Code, "error": value}
}

func requiredString(args map[string]any, key string) (string, error) {
	value, ok := args[key].(string)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", invalidArgument(fmt.Sprintf("%s is required", key), key)
	}
	return value, nil
}
func optionalString(args map[string]any, key, def string) string {
	if value, ok := args[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return def
}
func optionalBool(args map[string]any, key string) bool { value, _ := args[key].(bool); return value }
func optionalIntPointer(args map[string]any, key string) (*int, error) {
	value, ok := integerValue(args[key])
	if !ok {
		if _, exists := args[key]; !exists {
			return nil, nil
		}
		return nil, invalidArgument(key+" must be an integer", key)
	}
	v := int(value)
	return &v, nil
}
func optionalInt64Pointer(args map[string]any, key string) (*int64, error) {
	value, ok := integerValue(args[key])
	if !ok {
		if _, exists := args[key]; !exists {
			return nil, nil
		}
		return nil, invalidArgument(key+" must be an integer", key)
	}
	return &value, nil
}
func integerValue(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		if v == float64(int64(v)) {
			return int64(v), true
		}
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	}
	return 0, false
}
func timeoutArg(args map[string]any) (time.Duration, error) {
	value, ok := integerValue(args["timeout_ms"])
	if !ok {
		if _, exists := args["timeout_ms"]; !exists {
			return defaultProviderTimeout, nil
		}
		return 0, invalidArgument("timeout_ms must be an integer", "timeout_ms")
	}
	if value < 1 || value > 300000 {
		return 0, invalidArgument("timeout_ms out of range", "timeout_ms")
	}
	return time.Duration(value) * time.Millisecond, nil
}
