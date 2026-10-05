package computer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	toolcore "github.com/uvwt/agentdock/internal/tool/core"
)

type acpComputerTokenContextKey struct{}

func (b *ACPBridge) MCPHTTPHandler() http.Handler {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "agentdock-computer-control", Version: "m6"}, &mcpsdk.ServerOptions{Instructions: "AgentDock Computer Control Broker. Prefer Browser Broker/programmatic tools. Acquire a session first. Foreground-required actions fail closed unless explicitly allowed."})
	b.registerMCPTools(server)
	transport := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20, PropagateRequestCancellation: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if token == "" || token == auth {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		owner, err := b.owner(token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), acpComputerTokenContextKey{}, token)
		ctx = requestmeta.WithAuthPrincipal(ctx, acpComputerAuthPrincipal(owner))
		transport.ServeHTTP(w, r.WithContext(ctx))
	})
}

func acpComputerAuthPrincipal(owner *acpOwner) requestmeta.AuthPrincipal {
	if owner == nil {
		return requestmeta.AuthPrincipal{}
	}
	return requestmeta.NewStableAuthPrincipal("acp_bridge", owner.sessionID, owner.profileID)
}

func acpComputerToken(ctx context.Context) (string, error) {
	token, _ := ctx.Value(acpComputerTokenContextKey{}).(string)
	if token == "" {
		return "", computerError(ErrOwnerMismatch, "ACP computer capability missing", "acp", nil, nil)
	}
	return token, nil
}

func computerMCPResult(value any, err error) (*mcpsdk.CallToolResult, error) {
	payload := map[string]any{}
	if value != nil {
		encoded, _ := json.Marshal(value)
		_ = json.Unmarshal(encoded, &payload)
	}
	if err != nil {
		code := ErrProviderFailed
		var toolErr *toolcore.ToolError
		var typed *Error
		if errors.As(err, &toolErr) {
			code = toolErr.Code
			payload["phase"] = toolErr.Category
			if toolErr.Details != nil {
				payload["details"] = toolErr.Details
			}
		} else if errors.As(err, &typed) {
			code = typed.Code
			payload["phase"] = typed.Phase
			if typed.Details != nil {
				payload["details"] = typed.Details
			}
		}
		payload["ok"] = false
		payload["code"] = code
		payload["error"] = err.Error()
	} else {
		payload["ok"] = true
	}
	encoded, encodeErr := json.Marshal(payload)
	if encodeErr != nil {
		return nil, encodeErr
	}
	return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(encoded)}}, StructuredContent: payload, IsError: err != nil}, nil
}

func (b *ACPBridge) registerMCPTools(server *mcpsdk.Server) {
	add := func(name, description string, schema map[string]any, fn func(context.Context, map[string]any) (any, error)) {
		server.AddTool(&mcpsdk.Tool{Name: name, Description: description, InputSchema: schema}, func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			args := map[string]any{}
			if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 && string(req.Params.Arguments) != "null" {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return computerMCPResult(nil, invalidArgument("computer tool arguments must be an object", name))
				}
			}
			value, err := fn(ctx, args)
			return computerMCPResult(value, err)
		})
	}
	acquireSchema := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
		"capability": map[string]any{"type": "string", "enum": []string{"observe", "act"}},
		"foreground": map[string]any{"type": "string", "enum": []string{"forbidden", "allowed"}},
	}}
	add("computer_acquire", "Acquire a session-scoped Computer Control capability. Foreground defaults to forbidden.", acquireSchema, func(ctx context.Context, args map[string]any) (any, error) {
		token, err := acpComputerToken(ctx)
		if err != nil {
			return nil, err
		}
		capability := Capability(optionalString(args, "capability", string(CapabilityObserve)))
		foreground := ForegroundPolicy(optionalString(args, "foreground", string(ForegroundForbidden)))
		return b.AcquireContext(ctx, token, capability, foreground)
	})
	observeSchema, _ := InputSchema(ToolObserve)
	add("computer_observe", "Read capabilities, permissions, apps/windows, or accessibility state without silently escalating to foreground control.", observeSchema, func(ctx context.Context, args map[string]any) (any, error) {
		token, err := acpComputerToken(ctx)
		if err != nil {
			return nil, err
		}
		id, err := requiredString(args, "session_id")
		if err != nil {
			return nil, err
		}
		req, err := observationFromArgs(args)
		if err != nil {
			return nil, err
		}
		return b.Observe(ctx, token, id, req)
	})
	actSchema, _ := InputSchema(ToolAct)
	add("computer_act", "Run an explicit native GUI action. The Broker rejects it before provider execution unless foreground=allowed.", actSchema, func(ctx context.Context, args map[string]any) (any, error) {
		token, err := acpComputerToken(ctx)
		if err != nil {
			return nil, err
		}
		id, err := requiredString(args, "session_id")
		if err != nil {
			return nil, err
		}
		req, err := actionFromArgs(args)
		if err != nil {
			return nil, err
		}
		return b.Act(ctx, token, id, req)
	})
	releaseSchema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"session_id"}, "properties": map[string]any{"session_id": map[string]any{"type": "string"}}}
	add("computer_release", "Release the session-scoped Computer Control capability.", releaseSchema, func(ctx context.Context, args map[string]any) (any, error) {
		token, err := acpComputerToken(ctx)
		if err != nil {
			return nil, err
		}
		id, err := requiredString(args, "session_id")
		if err != nil {
			return nil, err
		}
		return b.Release(token, id)
	})
}

func observationFromArgs(args map[string]any) (ObservationRequest, error) {
	action, err := requiredString(args, "action")
	if err != nil {
		return ObservationRequest{}, err
	}
	req := ObservationRequest{Action: action, App: optionalString(args, "app", ""), RestoreWindow: optionalBool(args, "restore_window"), NoScreenshot: optionalBool(args, "no_screenshot")}
	if req.WindowID, err = optionalInt64Pointer(args, "window_id"); err != nil {
		return ObservationRequest{}, err
	}
	if req.WindowIndex, err = optionalIntPointer(args, "window_index"); err != nil {
		return ObservationRequest{}, err
	}
	req.Timeout, err = timeoutArg(args)
	return req, err
}
func actionFromArgs(args map[string]any) (ActionRequest, error) {
	action, err := requiredString(args, "action")
	if err != nil {
		return ActionRequest{}, err
	}
	req := ActionRequest{Action: action, App: optionalString(args, "app", ""), Value: optionalString(args, "value", ""), Text: optionalString(args, "text", ""), Key: optionalString(args, "key", ""), Direction: optionalString(args, "direction", ""), SecondaryAction: optionalString(args, "secondary_action", ""), Modifiers: optionalString(args, "modifiers", ""), MouseButton: optionalString(args, "mouse_button", ""), RestoreWindow: optionalBool(args, "restore_window"), NoScreenshot: optionalBool(args, "no_screenshot")}
	if req.WindowID, err = optionalInt64Pointer(args, "window_id"); err != nil {
		return ActionRequest{}, err
	}
	if req.WindowIndex, err = optionalIntPointer(args, "window_index"); err != nil {
		return ActionRequest{}, err
	}
	for key, dst := range map[string]**int{"element_index": &req.ElementIndex, "x": &req.X, "y": &req.Y, "from_element_index": &req.FromElementIndex, "to_element_index": &req.ToElementIndex, "from_x": &req.FromX, "from_y": &req.FromY, "to_x": &req.ToX, "to_y": &req.ToY} {
		value, parseErr := optionalIntPointer(args, key)
		if parseErr != nil {
			return ActionRequest{}, parseErr
		}
		*dst = value
	}
	req.Timeout, err = timeoutArg(args)
	return req, err
}

func (b *ACPBridge) MCPServerURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d/internal/acp-computer/mcp", port)
}
