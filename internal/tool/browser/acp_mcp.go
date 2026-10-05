package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
)

type acpBrowserTokenContextKey struct{}

func browserMCPObject(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func browserMCPString(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func (b *ACPBridge) MCPHTTPHandler() http.Handler {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "agentdock-browser-broker", Version: "m6"}, &mcpsdk.ServerOptions{
		Instructions: "Lease-scoped AgentDock Browser Broker. Acquire one lease before browser actions and release it when browser work is complete. Never select global pages.",
	})
	b.registerACPMCPTools(server)
	transport := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, &mcpsdk.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20, PropagateRequestCancellation: true,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if token == "" || token == r.Header.Get("Authorization") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		owner, err := b.owner(token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), acpBrowserTokenContextKey{}, token)
		ctx = requestmeta.WithAuthPrincipal(ctx, acpBrowserAuthPrincipal(owner))
		transport.ServeHTTP(w, r.WithContext(ctx))
	})
}

func acpBrowserAuthPrincipal(owner *acpBrowserOwner) requestmeta.AuthPrincipal {
	if owner == nil {
		return requestmeta.AuthPrincipal{}
	}
	return requestmeta.NewStableAuthPrincipal("acp_bridge", owner.sessionID, owner.profileID)
}

func acpBrowserToken(ctx context.Context) (string, error) {
	token, _ := ctx.Value(acpBrowserTokenContextKey{}).(string)
	if token == "" {
		return "", browserError(ErrLeaseOwnerMismatch, "ACP browser capability missing", "acp", nil, nil)
	}
	return token, nil
}

func acpMCPResult(payload map[string]any, err error) (*mcpsdk.CallToolResult, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	if err != nil {
		code := ErrActionFailed
		var browserErr *Error
		if errors.As(err, &browserErr) {
			code = browserErr.Code
			payload["phase"] = browserErr.Phase
			if browserErr.Details != nil {
				payload["details"] = browserErr.Details
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
	return &mcpsdk.CallToolResult{
		Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: string(encoded)}},
		StructuredContent: payload,
		IsError:           err != nil,
	}, nil
}

func (b *ACPBridge) registerACPMCPTools(server *mcpsdk.Server) {
	add := func(name, description string, schema map[string]any, fn func(context.Context, map[string]any) (map[string]any, error)) {
		server.AddTool(&mcpsdk.Tool{Name: name, Description: description, InputSchema: schema}, func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			args := map[string]any{}
			if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 && string(req.Params.Arguments) != "null" {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return acpMCPResult(nil, browserError(ErrActionInvalid, "browser tool arguments must be an object", "acp", nil, err))
				}
			}
			payload, err := fn(ctx, args)
			return acpMCPResult(payload, err)
		})
	}
	leaseProp := browserMCPString("Browser lease id returned by browser_acquire.")
	add("browser_acquire", "Acquire one lease-scoped browser target using AgentDock workspace routing.", browserMCPObject(map[string]any{
		"url": browserMCPString("Optional initial URL. Defaults to about:blank."),
	}), func(ctx context.Context, args map[string]any) (map[string]any, error) {
		token, err := acpBrowserToken(ctx)
		if err != nil {
			return nil, err
		}
		url, _ := args["url"].(string)
		meta, err := b.Acquire(ctx, token, url)
		if err != nil {
			return nil, err
		}
		return map[string]any{"lease_id": meta.BrowserLeaseID, "profile_class": meta.ProfileClass, "foreground_policy": meta.ForegroundPolicy, "expires_at": meta.ExpiresAt}, nil
	})
	add("browser_navigate", "Navigate the lease target without selecting any global browser page.", browserMCPObject(map[string]any{
		"lease_id": leaseProp, "url": browserMCPString("Absolute or browser-supported target URL."),
	}, "lease_id", "url"), func(ctx context.Context, args map[string]any) (map[string]any, error) {
		return b.acpMCPCall(ctx, args, "navigate_page", map[string]any{"type": "url", "url": args["url"]})
	})
	add("browser_snapshot", "Capture an accessibility snapshot of the lease target.", browserMCPObject(map[string]any{
		"lease_id": leaseProp, "verbose": map[string]any{"type": "boolean"},
	}, "lease_id"), func(ctx context.Context, args map[string]any) (map[string]any, error) {
		call := map[string]any{}
		if v, ok := args["verbose"].(bool); ok {
			call["verbose"] = v
		}
		return b.acpMCPCall(ctx, args, "take_snapshot", call)
	})
	add("browser_screenshot", "Capture the lease target only.", browserMCPObject(map[string]any{
		"lease_id": leaseProp, "full_page": map[string]any{"type": "boolean"},
	}, "lease_id"), func(ctx context.Context, args map[string]any) (map[string]any, error) {
		call := map[string]any{}
		if v, ok := args["full_page"].(bool); ok {
			call["fullPage"] = v
		}
		return b.acpMCPCall(ctx, args, "take_screenshot", call)
	})
	add("browser_click", "Click a snapshot uid in the lease target.", browserMCPObject(map[string]any{
		"lease_id": leaseProp, "uid": browserMCPString("Snapshot uid to click."),
	}, "lease_id", "uid"), func(ctx context.Context, args map[string]any) (map[string]any, error) {
		return b.acpMCPCall(ctx, args, "click", map[string]any{"uid": args["uid"]})
	})
	add("browser_fill", "Fill a snapshot uid in the lease target.", browserMCPObject(map[string]any{
		"lease_id": leaseProp, "uid": browserMCPString("Snapshot uid to fill."), "value": browserMCPString("Value to enter."),
	}, "lease_id", "uid", "value"), func(ctx context.Context, args map[string]any) (map[string]any, error) {
		return b.acpMCPCall(ctx, args, "fill", map[string]any{"uid": args["uid"], "value": args["value"]})
	})
	add("browser_evaluate", "Evaluate JavaScript in the lease target.", browserMCPObject(map[string]any{
		"lease_id": leaseProp, "function": browserMCPString("JavaScript function declaration executed in the target page."),
	}, "lease_id", "function"), func(ctx context.Context, args map[string]any) (map[string]any, error) {
		return b.acpMCPCall(ctx, args, "evaluate_script", map[string]any{"function": args["function"]})
	})
	add("browser_press_key", "Press a key in the lease target.", browserMCPObject(map[string]any{
		"lease_id": leaseProp, "key": browserMCPString("Key or key combination."),
	}, "lease_id", "key"), func(ctx context.Context, args map[string]any) (map[string]any, error) {
		return b.acpMCPCall(ctx, args, "press_key", map[string]any{"key": args["key"]})
	})
	add("browser_release", "Release the lease and only its AgentDock-owned target/resources.", browserMCPObject(map[string]any{
		"lease_id": leaseProp,
	}, "lease_id"), func(ctx context.Context, args map[string]any) (map[string]any, error) {
		token, err := acpBrowserToken(ctx)
		if err != nil {
			return nil, err
		}
		leaseID, _ := args["lease_id"].(string)
		if strings.TrimSpace(leaseID) == "" {
			return nil, browserError(ErrActionInvalid, "lease_id required", "acp", nil, nil)
		}
		meta, err := b.Release(ctx, token, leaseID)
		return map[string]any{"lease_id": leaseID, "cleanup_state": meta.CleanupState, "cleanup_reason": meta.CleanupReason}, err
	})
}

func (b *ACPBridge) acpMCPCall(ctx context.Context, args map[string]any, tool string, callArgs map[string]any) (map[string]any, error) {
	token, err := acpBrowserToken(ctx)
	if err != nil {
		return nil, err
	}
	leaseID, _ := args["lease_id"].(string)
	if strings.TrimSpace(leaseID) == "" {
		return nil, browserError(ErrActionInvalid, "lease_id required", "acp", nil, nil)
	}
	result, err := b.Call(ctx, token, leaseID, tool, callArgs)
	if err != nil {
		return map[string]any{"lease_id": leaseID}, err
	}
	return map[string]any{"lease_id": leaseID, "engine_result": result}, nil
}

func (b *ACPBridge) MCPServerURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d/internal/acp-browser/mcp", port)
}
