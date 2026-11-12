package app

import (
	"context"

	toolacp "github.com/uvwt/agentdock/internal/tool/acp"
)

func acpToolSpecs() []ToolSpec {
	return []ToolSpec{
		{Name: "acp_session", Contract: acpToolContract, Title: "Manage ACP sessions", Description: "Manage AgentDock and Adapter-native ACP sessions through info, status, new, list, inspect, open, update, close, and delete. status is observation-only and correlates ACP lifecycle state with adapter process health and M6 Browser/Computer Broker ownership without activating sessions. Native session discovery uses ACP session/list; open negotiates resume/load internally; fork and session settings are expressed through new/update instead of exposing protocol methods as separate actions.", Annotations: mutatingToolAnnotations(true, true), Availability: requiresACP, Handler: typedToolHandler("acp_session", func(ctx context.Context, r *Runtime, request toolacp.SessionRequest) (Result, error) {
			result, err := r.acp.Session(ctx, request)
			if err != nil {
				return nil, err
			}
			if request.Action == "status" {
				result = r.enrichACPStatus(ctx, result)
			}
			return result, nil
		})},
		{Name: "acp_prompt", Contract: acpToolContract, Title: "Run ACP prompts", Description: "Start asynchronous ACP prompt Runs with ACP ContentBlocks, read ordered Run events, or request cancellation. start automatically reactivates managed sessions and capability-driven steering remains an internal implementation detail; cancel drains the Adapter's final updates before the Run settles.", Annotations: mutatingToolAnnotations(true, true), Availability: requiresACP, Handler: typedToolHandler("acp_prompt", func(ctx context.Context, r *Runtime, request toolacp.PromptRequest) (Result, error) {
			return r.acp.Prompt(ctx, request)
		})},
		{Name: "acp_interaction", Contract: acpToolContract, Title: "Handle ACP interactions", Description: "List or respond to pending ACP human interactions. Permission responses remain constrained to Adapter-offered options and local policy; elicitation is not advertised until AgentDock implements its complete structured lifecycle.", Annotations: mutatingToolAnnotations(true, true), Availability: requiresACP, Handler: typedToolHandler("acp_interaction", func(ctx context.Context, r *Runtime, request toolacp.InteractionRequest) (Result, error) {
			return r.acp.Interaction(ctx, request)
		})},
	}
}
