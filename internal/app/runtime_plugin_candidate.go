package app

import (
	"context"
	"encoding/json"

	toolplugin "github.com/uvwt/agentdock/internal/tool/plugin"
)

func (r *Runtime) RuntimePluginDesktopCandidate(ctx context.Context, args map[string]any) (Result, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return nil, toolplugin.DesktopError("PLUGIN_CANDIDATE_INVALID", "validation")
	}
	request, err := toolplugin.DecodeDesktopCandidateRequest(body)
	if err != nil {
		return nil, err
	}
	switch request.Action {
	case "prepare":
		candidate, err := r.plugins.PrepareDesktopCandidate(ctx, request.Source, request.Kind, request.TargetName, request.TargetGeneration)
		if err != nil {
			return nil, err
		}
		return Result{"ok": true, "candidate": candidate}, nil
	case "discard":
		if err := r.plugins.DiscardDesktopCandidate(request.CandidateID); err != nil {
			return nil, err
		}
		return Result{"ok": true, "action": "discard", "candidate_id": request.CandidateID, "completed": true}, nil
	default:
		return nil, toolplugin.DesktopError("PLUGIN_CANDIDATE_INVALID", "validation")
	}
}
