package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/uvwt/agentdock/internal/buildinfo"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/execution"
	"github.com/uvwt/agentdock/internal/observability"
	toolmcp "github.com/uvwt/agentdock/internal/tool/mcp"
	toolplugin "github.com/uvwt/agentdock/internal/tool/plugin"
)

const runtimeAPISource = "agentdock-api"

func (r *Runtime) RuntimeStatus() Result {
	tools := r.ToolNames()
	return Result{
		"ok":                    true,
		"source":                runtimeAPISource,
		"service":               config.ServerName,
		"version":               buildinfo.Version,
		"agentdock_home":        r.cfg.AgentDockHome,
		"agentdock_default_dir": r.cfg.AgentDockDefaultDir,
		"path_model":            config.PathModel,
		"auth_enabled":          r.cfg.AuthRequired(),
		"browser_enabled":       r.cfg.BrowserEnabled,
		"memory_enabled":        r.cfg.NexusEndpoint != "",
		"nexus_enabled":         strings.TrimSpace(r.cfg.NexusEndpoint) != "",
		"tool_count":            len(tools),
		"tools":                 tools,
	}
}

func (r *Runtime) RuntimeAnalytics() Result {
	snapshot := r.observer.Snapshot()
	return Result{
		"ok":              true,
		"source":          runtimeAPISource,
		"started_at":      snapshot.StartedAt,
		"recent_capacity": snapshot.RecentCapacity,
		"window_calls":    snapshot.WindowCalls,
		"total_calls":     snapshot.TotalCalls,
		"total_errors":    snapshot.TotalErrors,
		"active_calls":    snapshot.ActiveCalls,
		"tool_stats":      snapshot.ToolStats,
		"recent_calls":    snapshot.RecentCalls,
		"process":         snapshot.Process,
	}
}

// RuntimeDiagnostics 只暴露最近调用的零 Payload 投影，供 Nexus 按需远程排障。
// 本地 analytics 的进程指标与聚合统计不进入跨节点契约。
func (r *Runtime) RuntimeDiagnostics() Result {
	return Result{
		"ok":           true,
		"source":       runtimeAPISource,
		"recent_calls": observability.ProjectDiagnostics(r.observer.RecentCalls()),
	}
}

func (r *Runtime) RuntimeExecution() Result {
	return Result{
		"ok":       true,
		"source":   runtimeAPISource,
		"snapshot": r.execution.Snapshot(),
	}
}

func (r *Runtime) RuntimeActivity(after uint64, limit int) (Result, error) {
	return Result{
		"ok":     true,
		"source": runtimeAPISource,
		"page":   r.execution.Page(after, limit),
	}, nil
}

func (r *Runtime) RuntimeActivityWait(ctx context.Context, after uint64) error {
	return r.execution.Wait(ctx, after)
}

func (r *Runtime) RuntimeInsertions(callID string) Result {
	return Result{
		"ok":         true,
		"source":     runtimeAPISource,
		"insertions": r.execution.Insertions(callID),
	}
}

func (r *Runtime) RuntimeInsertionManage(ctx context.Context, args map[string]any) (Result, error) {
	action, _ := args["action"].(string)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "enqueue":
		callID, _ := args["call_id"].(string)
		text, _ := args["text"].(string)
		return r.runRuntimeManagementMutation(ctx, "runtime_insertion", "enqueue", map[string]any{
			"call_id": callID,
			"text":    text,
		}, func() (Result, error) {
			item, err := r.execution.EnqueueInsertion(callID, text)
			if err != nil {
				switch {
				case errors.Is(err, execution.ErrInsertionTarget):
					return nil, toolError("INSERTION_TARGET_UNAVAILABLE", err.Error(), "not_found")
				case errors.Is(err, execution.ErrInsertionCapacity):
					return nil, toolError("INSERTION_CAPACITY", err.Error(), "runtime")
				default:
					return nil, toolError("INVALID_INSERTION", err.Error(), "validation")
				}
			}
			return Result{"ok": true, "source": runtimeAPISource, "ack": true, "insertion": item}, nil
		})
	case "cancel":
		id, _ := args["insertion_id"].(string)
		return r.runRuntimeManagementMutation(ctx, "runtime_insertion", "cancel", map[string]any{
			"insertion_id": id,
		}, func() (Result, error) {
			item, err := r.execution.CancelInsertion(id)
			if err != nil {
				if errors.Is(err, execution.ErrInsertionNotFound) {
					return nil, toolError("INSERTION_NOT_FOUND", err.Error(), "not_found")
				}
				return nil, toolError("INSERTION_CANCEL_FAILED", err.Error(), "runtime")
			}
			return Result{"ok": true, "source": runtimeAPISource, "insertion": item}, nil
		})
	default:
		return nil, toolError("INVALID_INSERTION_ACTION", "unsupported insertion action", "validation")
	}
}

func (r *Runtime) RuntimeSkills() (Result, error) {
	return r.skills.RuntimeSkills()
}

func (r *Runtime) RuntimeSkill(skill string) (Result, error) {
	return r.skills.RuntimeSkill(skill)
}

func (r *Runtime) RuntimeSkillFiles(skill string) (Result, error) {
	return r.skills.RuntimeSkillFiles(skill)
}

func (r *Runtime) RuntimeSkillFile(skill, relativePath string) (Result, error) {
	return r.skills.RuntimeSkillFile(skill, relativePath)
}

func (r *Runtime) RuntimePlugins(ctx context.Context) (Result, error) {
	result, err := r.plugins.RuntimeList()
	if err != nil {
		return nil, err
	}
	result["ok"] = true
	result["source"] = runtimeAPISource
	return result, nil
}

func (r *Runtime) RuntimePlugin(ctx context.Context, name string) (Result, error) {
	return r.runtimePluginManage(ctx, map[string]any{"action": "inspect", "name": name})
}

// Runtime Plugin API 只暴露只读索引与 inspect；安装、更新、启停和删除仍由
// plugin_manage 的确认与事务语义负责，避免面向 UI 的接口形成第二套生命周期入口。
func (r *Runtime) runtimePluginManage(ctx context.Context, args map[string]any) (Result, error) {
	if err := r.validateToolArguments(toolplugin.ToolManage, args); err != nil {
		return nil, err
	}
	var request toolplugin.ManageRequest
	if err := decodeToolInput(toolplugin.ToolManage, args, &request); err != nil {
		return nil, err
	}
	result, err := r.plugins.Manage(ctx, request)
	if err != nil {
		return nil, err
	}
	result["ok"] = true
	result["source"] = runtimeAPISource
	return result, nil
}

func (r *Runtime) RuntimeTasks(status string, limit int) (Result, error) {
	return r.taskTools.RuntimeTasks(status, limit)
}

func (r *Runtime) RuntimeTask(id string) (Result, error) {
	return r.taskTools.RuntimeTask(id)
}

func (r *Runtime) RuntimeTaskDelete(ctx context.Context, id string) (Result, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return r.taskTools.RuntimeTaskDelete(id)
	}
	return r.runRuntimeManagementMutation(ctx, "runtime_task", "delete", map[string]any{
		"task_id": id,
	}, func() (Result, error) {
		return r.taskTools.RuntimeTaskDelete(id)
	})
}

func (r *Runtime) RuntimeCapabilities(ctx context.Context, refresh bool) (Result, error) {
	result, err := r.AgentDockContext(ctx)
	if err != nil {
		return nil, err
	}
	result["source"] = runtimeAPISource
	return result, nil
}

func (r *Runtime) RuntimeMCPServers(ctx context.Context) (Result, error) {
	return r.runtimeMCPManage(ctx, map[string]any{"action": "list"})
}

func (r *Runtime) RuntimeMCPServer(ctx context.Context, name string) (Result, error) {
	return r.runtimeMCPManage(ctx, map[string]any{"action": "inspect", "name": name})
}

func (r *Runtime) RuntimeMCPManage(ctx context.Context, args map[string]any) (Result, error) {
	request, err := r.prepareRuntimeMCPManage(args)
	if err != nil {
		return nil, err
	}
	action := strings.ToLower(strings.TrimSpace(request.Action))
	return r.runRuntimeManagementMutation(ctx, "runtime_mcp", action, request, func() (Result, error) {
		return r.dispatchRuntimeMCPManage(ctx, request)
	})
}

func (r *Runtime) runtimeMCPManage(ctx context.Context, args map[string]any) (Result, error) {
	request, err := r.prepareRuntimeMCPManage(args)
	if err != nil {
		return nil, err
	}
	return r.dispatchRuntimeMCPManage(ctx, request)
}

func (r *Runtime) prepareRuntimeMCPManage(args map[string]any) (toolmcp.ManageRequest, error) {
	if err := r.validateToolArguments(toolmcp.ToolManage, args); err != nil {
		return toolmcp.ManageRequest{}, err
	}
	var request toolmcp.ManageRequest
	if err := decodeToolInput("mcp_manage", args, &request); err != nil {
		return toolmcp.ManageRequest{}, err
	}
	return request, nil
}

func (r *Runtime) dispatchRuntimeMCPManage(ctx context.Context, request toolmcp.ManageRequest) (Result, error) {
	result, err := r.dynamicMCP.Manage(ctx, request)
	if err != nil {
		return nil, err
	}
	result["ok"] = true
	result["source"] = runtimeAPISource
	return result, nil
}

// RuntimeMCPDesktop only observes authoritative configuration and cached state.
func (r *Runtime) RuntimeMCPDesktop(ctx context.Context) (Result, error) {
	return r.RuntimeMCPDesktopManage(ctx, map[string]any{"action": "desktop_snapshot"})
}

func (r *Runtime) RuntimeMCPDesktopManage(ctx context.Context, args map[string]any) (Result, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return nil, toolError("INVALID_MCP_REQUEST", "Invalid Desktop MCP request", "validation")
	}
	request, err := toolmcp.DecodeDesktopRequest(body)
	if err != nil {
		return nil, err
	}
	if request.Action == "desktop_operation_status" {
		return r.desktopMCPOperationStatus(request.RequestID)
	}
	dispatch := func() (Result, error) {
		result, err := r.dynamicMCP.DesktopManage(ctx, request)
		if err != nil {
			return nil, err
		}
		result["ok"], result["source"] = true, runtimeAPISource
		return result, nil
	}
	switch request.Action {
	case "desktop_snapshot", "desktop_inspect", "desktop_env_snapshot", "desktop_auth_status":
		return dispatch()
	}
	descriptor := desktopAdmissionDescriptor(request)
	return r.runDesktopMCPMutation(ctx, request, descriptor, dispatch)
}

// Never pass the write-only value, raw args or endpoint query to admission, even
// on invalid requests. Only this descriptor is snapshotted/fingerprinted.
func desktopAdmissionDescriptor(r toolmcp.DesktopManageRequest) map[string]any {
	d := map[string]any{
		"action": r.Action, "name": r.Name, "key": r.Key,
		"expected_registry_revision": r.ExpectedRegistryRevision,
		"expected_generation":        r.ExpectedGeneration, "expected_env_revision": r.ExpectedEnvRevision,
	}
	switch r.Action {
	case "desktop_env_set":
		d["value_configured"] = r.Value != nil && *r.Value != ""
	case "desktop_set_enabled":
		d["enabled"] = r.Enabled
		d["reuse_configured_environment"] = r.ReuseConfiguredEnvironment
	case "desktop_authorize":
		d["callback_id"] = r.CallbackID
	case "desktop_create", "desktop_update":
		d["transport"], d["protocol_version"], d["enabled"], d["timeout_ms"] = r.Transport, r.ProtocolVersion, r.Enabled, r.TimeoutMS
		d["command_configured"] = strings.TrimSpace(r.Command) != ""
		d["cwd_configured"] = strings.TrimSpace(r.CWD) != ""
		d["header_env"], d["env_from_env"] = r.HeaderEnv, r.EnvFromEnv
		d["args_configured"], d["args_count"] = r.Args != nil, len(r.Args)
		d["endpoint_configured"] = r.URL != ""
		if endpoint, err := url.Parse(r.URL); err == nil && endpoint.User == nil && (endpoint.Scheme == "https" || endpoint.Scheme == "http") {
			d["endpoint_scheme"] = endpoint.Scheme
		}
	}
	return d
}
