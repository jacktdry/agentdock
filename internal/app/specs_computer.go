package app

import "context"

func computerToolSpecs() []ToolSpec {
	return []ToolSpec{
		{Name: "computer_session", Contract: computerToolContract, Title: "Computer control session", Description: "Acquire or release an AgentDock Computer Control Broker session. Orca is the default provider; foreground native GUI control is forbidden unless explicitly acquired with foreground=allowed.", Annotations: mutatingToolAnnotations(false, true), Handler: func(ctx context.Context, r *Runtime, args map[string]any) (Result, error) {
			return r.computer.HandleSession(ctx, args)
		}},
		{Name: "computer_observe", Contract: computerToolContract, Title: "Computer observation", Description: "Inspect computer-use capabilities, running apps/windows, accessibility state, or provider permissions through AgentDock Computer Control Broker. permissions and restore_window are foreground-gated and never run silently under foreground=forbidden.", Annotations: mutatingToolAnnotations(false, true), Handler: func(ctx context.Context, r *Runtime, args map[string]any) (Result, error) {
			return r.computer.HandleObserve(ctx, args)
		}},
		{Name: "computer_act", Contract: computerToolContract, Title: "Computer action", Description: "Perform an explicit native GUI action through AgentDock Computer Control Broker. Current native actions are foreground-required and are rejected before provider execution unless the session was acquired with foreground=allowed.", Annotations: mutatingToolAnnotations(true, true), Handler: func(ctx context.Context, r *Runtime, args map[string]any) (Result, error) {
			return r.computer.HandleAct(ctx, args)
		}},
	}
}
