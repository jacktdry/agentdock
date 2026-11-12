package app

import (
	"context"
	"encoding/json"
	toolplugin "github.com/uvwt/agentdock/internal/tool/plugin"
)

func (r *Runtime) RuntimePluginDesktop(ctx context.Context) (Result, error) {
	return r.RuntimePluginDesktopManage(ctx, map[string]any{"action": "desktop_snapshot"})
}
func (r *Runtime) RuntimePluginDesktopManage(ctx context.Context, args map[string]any) (Result, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return nil, toolplugin.DesktopError("INVALID_PLUGIN_REQUEST", "validation")
	}
	request, err := toolplugin.DecodeDesktopRequest(body)
	if err != nil {
		return nil, err
	}
	if request.Action == "desktop_operation_status" {
		result, err := r.desktopPluginOperationStatus(request.RequestID)
		if result != nil {
			result["ok"] = true
		}
		return result, err
	}
	dispatch := func() (Result, error) {
		result, err := r.plugins.DesktopManage(ctx, request)
		if err != nil {
			return nil, err
		}
		result["ok"], result["source"] = true, runtimeAPISource
		return result, nil
	}
	switch request.Action {
	case "desktop_snapshot", "desktop_inspect", "desktop_env_snapshot":
		return dispatch()
	}
	return r.runDesktopPluginMutation(ctx, request, desktopPluginAdmissionDescriptor(request), dispatch)
}
func desktopPluginAdmissionDescriptor(r toolplugin.DesktopManageRequest) map[string]any {
	return map[string]any{"action": r.Action, "name": r.Name, "component": r.Component, "key": r.Key, "enabled": r.Enabled,
		"value_configured": r.Value != nil && *r.Value != "", "expected_registry_revision": r.ExpectedRegistryRevision,
		"expected_generation": r.ExpectedGeneration, "expected_env_revision": r.ExpectedEnvRevision}
}
