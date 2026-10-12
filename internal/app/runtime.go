package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	acpruntime "github.com/uvwt/agentdock/internal/acp"
	"github.com/uvwt/agentdock/internal/browserpolicy"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/controlplane"
	"github.com/uvwt/agentdock/internal/envstore"
	"github.com/uvwt/agentdock/internal/evolution"
	"github.com/uvwt/agentdock/internal/execution"
	"github.com/uvwt/agentdock/internal/httpx/requestmeta"
	mcpclient "github.com/uvwt/agentdock/internal/mcp/client"
	"github.com/uvwt/agentdock/internal/mcp/launchregistry"
	"github.com/uvwt/agentdock/internal/observability"
	"github.com/uvwt/agentdock/internal/permission"
	pluginruntime "github.com/uvwt/agentdock/internal/plugin"
	"github.com/uvwt/agentdock/internal/taskstate"
	toolacp "github.com/uvwt/agentdock/internal/tool/acp"
	toolbrowser "github.com/uvwt/agentdock/internal/tool/browser"
	toolcommand "github.com/uvwt/agentdock/internal/tool/command"
	toolcomputer "github.com/uvwt/agentdock/internal/tool/computer"
	toolcontract "github.com/uvwt/agentdock/internal/tool/contract"
	toolcore "github.com/uvwt/agentdock/internal/tool/core"
	toolfile "github.com/uvwt/agentdock/internal/tool/file"
	toolmcp "github.com/uvwt/agentdock/internal/tool/mcp"
	toolmedia "github.com/uvwt/agentdock/internal/tool/media"
	toolplugin "github.com/uvwt/agentdock/internal/tool/plugin"
	toolrecall "github.com/uvwt/agentdock/internal/tool/recall"
	toolskill "github.com/uvwt/agentdock/internal/tool/skill"
	tooltask "github.com/uvwt/agentdock/internal/tool/task"
	"github.com/uvwt/agentdock/internal/workspace"
)

type Result = toolcore.Result

type Runtime struct {
	cfg                   config.Config
	toolNames             []string
	toolValidators        map[string]*toolcontract.InputValidator
	ws                    *workspace.Workspace
	skills                *toolskill.Service
	command               *toolcommand.Service
	files                 *toolfile.Service
	dynamicMCP            *toolmcp.Service
	plugins               *toolplugin.Service
	media                 *toolmedia.Service
	browser               *toolbrowser.Service
	acpBrowser            *toolbrowser.ACPBridge
	acpSharedRestartGuard *sharedBrowserRestartGuard
	computer              *toolcomputer.Service
	acpComputer           *toolcomputer.ACPBridge
	recall                *toolrecall.Service
	evolution             *evolution.Service
	taskTools             *tooltask.Service
	acp                   *toolacp.Service
	// Core-only, Next opt-in, never installed or activated by NewRuntime.
	// Source-only Next uses one permission writer across all Core processes.
	nextPermissionOwner       *nextPermissionOwner
	nextCodexStartupLock      *nextCodexStartupLock
	nextCodexPrincipal        requestmeta.AuthPrincipal
	nextCodexPolicyRevision   uint64
	nextCodexRegistry         *launchregistry.PrivateCoreListener
	nextCodexPin              *launchregistry.GenerationPin
	nextCodexRegisteredID     string
	nextCodexRegisteredDigest string
	observer                  *observability.Recorder
	execution                 *execution.Store
	permissions               *permission.Store
	admission                 *permission.AdmissionGate
	permissionCtl             *permission.ControlAuthority
	tracing                   *observability.Tracing
	lifecycleMu               sync.RWMutex
	pluginDesktopOpsMu        sync.Mutex
	pluginDesktopOps          map[string]desktopPluginOperationRecord
	mcpDesktopOpsMu           sync.Mutex
	mcpDesktopOps             map[string]desktopMCPOperationRecord
	mcpDesktopFingerprintKey  [32]byte
	commandCtx                context.Context
	commandCancel             context.CancelFunc
	closing                   bool
	closeOnce                 sync.Once
	closeErr                  error
}

func NewRuntime(cfg config.Config) (*Runtime, error) {
	toolNames, toolValidators, err := compileAvailableToolContracts(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize tool contracts: %w", err)
	}
	ws, err := workspace.New(cfg.AgentDockDefaultDir)
	if err != nil {
		return nil, err
	}
	var nextWriter *nextPermissionOwner
	if nextCodexExclusiveAuthorityEnabled(cfg) {
		nextWriter, err = acquireNextPermissionOwner(cfg.AgentDockHome)
		if err != nil {
			return nil, fmt.Errorf("Next permission writer already owned or unsafe: %w", err)
		}
	}
	// Closing a partially initialized Runtime and this deferred release are both
	// safe: fd Close is idempotent. The successful Runtime retains ownership.
	keepWriter := false
	defer func() {
		if !keepWriter && nextWriter != nil {
			_ = nextWriter.close()
		}
	}()
	executionStore := execution.NewStore(execution.DefaultEventCapacity, execution.DefaultCallCapacity)
	permissionStore, err := permission.NewStore(cfg.AgentDockHome, executionStore.Epoch())
	if err != nil {
		return nil, fmt.Errorf("initialize permission store: %w", err)
	}
	admissionGate, err := permission.NewAdmissionGate(permissionStore)
	if err != nil {
		return nil, fmt.Errorf("initialize permission admission: %w", err)
	}
	permissionControl, err := permission.NewControlAuthority()
	if err != nil {
		return nil, fmt.Errorf("initialize desktop permission control authority: %w", err)
	}
	protectedControlPaths, err := controlplane.NewPathSet(filepath.Join(cfg.AgentDockHome, "permissions"))
	if err != nil {
		return nil, fmt.Errorf("initialize protected control paths: %w", err)
	}
	envs, err := envstore.New(cfg.AgentDockHome)
	if err != nil {
		return nil, err
	}
	pluginManager, err := pluginruntime.NewManager(cfg.AgentDockHome)
	if err != nil {
		return nil, err
	}
	skills, err := toolskill.New(cfg, ws, envs, pluginManager)
	if err != nil {
		return nil, err
	}
	mcpClients, err := mcpclient.NewManager(cfg.AgentDockHome, envs)
	if err != nil {
		return nil, err
	}
	tasks, err := taskstate.New(filepath.Join(cfg.AgentDockHome, "tasks"))
	if err != nil {
		_ = mcpClients.Close()
		return nil, err
	}
	commandCtx, commandCancel := context.WithCancel(context.Background())
	var mcpDesktopFingerprintKey [32]byte
	if _, err := rand.Read(mcpDesktopFingerprintKey[:]); err != nil {
		_ = mcpClients.Close()
		commandCancel()
		return nil, fmt.Errorf("initialize Desktop MCP request binding: %w", err)
	}
	runtime := &Runtime{
		cfg: cfg, ws: ws, skills: skills,
		toolNames: toolNames, toolValidators: toolValidators,
		observer:                 observability.NewRecorder(observability.DefaultRecentCapacity),
		execution:                executionStore,
		permissions:              permissionStore,
		admission:                admissionGate,
		permissionCtl:            permissionControl,
		mcpDesktopFingerprintKey: mcpDesktopFingerprintKey,
		commandCtx:               commandCtx,
		commandCancel:            commandCancel,
	}
	runtime.command = toolcommand.New(func() config.Config { return runtime.cfg }, ws, envs, func(ctx context.Context, skillRef string) (toolcommand.SkillLease, error) {
		resolved, release, err := skills.Acquire(ctx, skillRef)
		if err != nil {
			return toolcommand.SkillLease{}, err
		}
		var envScope *envstore.Scope
		skillDataDir := ""
		runtimeEnv := map[string]string(nil)
		switch resolved.SourceType {
		case "managed":
			scope := envstore.Scope{Kind: envstore.ScopeSkill, Name: resolved.Name}
			envScope = &scope
			skillDataDir, err = config.SkillDataDir(cfg, resolved.Name)
			if err != nil {
				release()
				return toolcommand.SkillLease{}, fmt.Errorf("resolve managed Skill data directory: %w", err)
			}
		case "plugin":
			scope := envstore.Scope{Kind: envstore.ScopePluginSkill, Plugin: resolved.PluginName, Name: resolved.Name}
			envScope = &scope
			skillDataDir, err = config.PluginSkillDataDir(cfg, resolved.PluginName, resolved.Name)
			if err != nil {
				release()
				return toolcommand.SkillLease{}, fmt.Errorf("resolve Plugin Skill data directory: %w", err)
			}
			pluginDataDir, dataErr := pluginManager.EnsureDataDir(resolved.PluginName)
			if dataErr != nil {
				release()
				return toolcommand.SkillLease{}, fmt.Errorf("prepare Plugin data directory: %w", dataErr)
			}
			runtimeEnv = map[string]string{config.PluginDataDirEnvKey: pluginDataDir}
		}
		return toolcommand.SkillLease{
			Name: resolved.Name, Root: resolved.Root, EnvScope: envScope,
			SkillDataDir: skillDataDir, RuntimeEnv: runtimeEnv, Release: release,
		}, nil
	}, runtime.commandExecutionContext)
	runtime.command.SetSessionLifecycleHook(runtime.observeCommandSession)
	runtime.files = toolfile.New(ws, skills.ResolveResource, runtime.command.CommandEnv)
	runtime.files.SetProtectedPathSet(protectedControlPaths)
	runtime.dynamicMCP = toolmcp.New(mcpClients, envs)
	runtime.dynamicMCP.SetExecutionStore(runtime.execution)
	if err := runtime.configureMCPOAuthCallbacks(); err != nil {
		_ = runtime.dynamicMCP.Close()
		commandCancel()
		return nil, err
	}
	runtime.plugins = toolplugin.New(cfg, pluginManager, mcpClients, envs, ws)
	if err := runtime.plugins.ReconcileMCP(); err != nil {
		_ = runtime.dynamicMCP.Close()
		commandCancel()
		return nil, fmt.Errorf("initialize Plugin MCP runtime: %w", err)
	}
	runtime.media = toolmedia.New(cfg, ws, runtime.command.InternalCommandEnv)
	runtime.media.SetProtectedPathSet(protectedControlPaths)
	runtime.browser = toolbrowser.New(
		toolbrowser.Config{AgentDockHome: cfg.AgentDockHome, ExecutablePath: cfg.BrowserExecutablePath, CDPURL: cfg.BrowserCDPURL, ReuseExistingCDP: cfg.BrowserReuseExistingCDP},
		runtime.media.PublishBrowserScreenshot,
	)
	runtime.computer = toolcomputer.NewService()
	runtime.recall = toolrecall.New(func() config.Config { return runtime.cfg })
	runtime.evolution = evolution.New(func() config.Config { return runtime.cfg }, tasks)
	runtime.taskTools = tooltask.New(func() config.Config { return runtime.cfg }, tasks, runtime.evolution)
	runtime.tracing = observability.NewTracing(nil)

	if cfg.ACPEnabled && cfg.BrowserEnabled && !cfg.Stdio {
		catalog, catalogErr := browserpolicy.NewCatalog(cfg.BrowserCatalog)
		if catalogErr != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("initialize ACP browser catalog: %w", catalogErr)
		}
		planner, plannerErr := toolbrowser.NewRoutePlanner(cfg.BrowserWorkspacePolicies, catalog, nil)
		if plannerErr != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("initialize ACP browser route planner: %w", plannerErr)
		}
		bridge, guard, bridgeErr := newRuntimeACPBrowserBridge(cfg, planner)
		if bridgeErr != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("initialize ACP browser bridge: %w", bridgeErr)
		}
		bridge.SetAdmissionHook(runtime.admitHostOperation)
		runtime.acpBrowser = bridge
		runtime.acpSharedRestartGuard = guard
	}
	if cfg.ACPEnabled && !cfg.Stdio && runtime.computer != nil && runtime.computer.Broker() != nil {
		computerBridge, computerBridgeErr := toolcomputer.NewACPBridge(runtime.computer.Broker())
		if computerBridgeErr != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("initialize ACP computer control bridge: %w", computerBridgeErr)
		}
		computerBridge.SetAdmissionHook(runtime.admitHostOperation)
		runtime.acpComputer = computerBridge
	}

	if cfg.ACPEnabled {
		managers := make(map[string]*acpruntime.Manager)
		for _, profile := range cfg.EffectiveACPProfiles() {
			profile := profile
			acpEnvironment := make(map[string]string, len(profile.EnvFromEnv))
			for childName, hostName := range profile.EnvFromEnv {
				value, exists := os.LookupEnv(hostName)
				if !exists {
					_ = toolacp.NewMulti(cfg.EffectiveACPDefaultProfile(), managers).Close()
					_ = runtime.Close()
					return nil, fmt.Errorf("required ACP environment variable %s for profile %s is missing", hostName, profile.ID)
				}
				acpEnvironment[childName] = value
			}
			acpEnvironment, err = prepareACPBrowserEnvironment(cfg.AgentDockHome, profile, acpEnvironment, runtime.acpBrowser != nil)
			if err != nil {
				_ = toolacp.NewMulti(cfg.EffectiveACPDefaultProfile(), managers).Close()
				_ = runtime.Close()
				return nil, fmt.Errorf("prepare ACP profile %s environment: %w", profile.ID, err)
			}
			var sessionMCP acpruntime.SessionMCPProvider
			if runtime.acpBrowser != nil || runtime.acpComputer != nil {
				sessionMCP = &acpHostCapabilityProvider{bridge: runtime.acpBrowser, computer: runtime.acpComputer, profileID: profile.ID, serverName: acpBrowserServerName(profile), port: cfg.Port}
			}
			manager, err := acpruntime.NewManager(acpruntime.Options{
				Home:       cfg.AgentDockHome,
				DefaultCWD: cfg.AgentDockDefaultDir,
				Agent: acpruntime.AgentSpec{
					Name: profile.ID, Command: profile.Command, Args: append([]string(nil), profile.Args...), Environment: acpEnvironment,
				},
				MaxConcurrentRuns:  cfg.ACPMaxPrompts,
				InteractionTimeout: time.Duration(cfg.ACPInteractionMS) * time.Millisecond,
				SessionMCPProvider: sessionMCP,
				PromptDecorator:    nextCodexMCPReportDecorator(cfg, profile),
				PermissionContinuationAdmission: func(ctx context.Context, continuation acpruntime.PermissionContinuation) (acpruntime.PermissionContinuationFinish, error) {
					finish, admissionErr := runtime.admitHostOperation(ctx, permission.HostOperation{
						Tool:          "acp_provider_continuation",
						Action:        "select",
						Source:        "acp_bridge",
						SessionID:     continuation.SessionID,
						ProfileID:     profile.ID,
						WorkspaceRoot: continuation.WorkspaceRoot,
						Payload: map[string]any{
							"interaction_id": continuation.InteractionID,
							"option_id":      continuation.OptionID,
							"tool_call":      continuation.ToolCall,
						},
					})
					if admissionErr != nil {
						return nil, admissionErr
					}
					return acpruntime.PermissionContinuationFinish(finish), nil
				},
			})
			if err != nil {
				_ = toolacp.NewMulti(cfg.EffectiveACPDefaultProfile(), managers).Close()
				_ = runtime.Close()
				return nil, fmt.Errorf("initialize ACP profile %s: %w", profile.ID, err)
			}
			managers[profile.ID] = manager
		}
		runtime.acp = toolacp.NewMulti(cfg.EffectiveACPDefaultProfile(), managers)
	}
	runtime.nextPermissionOwner = nextWriter
	keepWriter = true
	return runtime, nil
}

func (r *Runtime) Config() config.Config           { return r.cfg }
func (r *Runtime) Workspace() *workspace.Workspace { return r.ws }

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		var closeErrors []error
		r.lifecycleMu.Lock()
		r.closing = true
		commandCancel := r.commandCancel
		r.lifecycleMu.Unlock()

		// 先禁止新的 command reservation，并等已经拿到 reservation 的启动流程离开
		// cmd.Start/平台进程控制器建立窗口。此处不能持有 lifecycleMu 等待，否则启动路径
		// 一旦需要读取 Runtime 生命周期状态就会形成锁顺序死锁。
		if r.command != nil {
			r.command.BeginClose()
			if err := r.command.WaitForStarts(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if commandCancel != nil {
			commandCancel()
		}
		// Revoke Core-only registration before any ACP process teardown.
		// This affects only our own uniquely created socket, not native MCPs.
		if r.nextCodexPin != nil {
			r.nextCodexPin.Revoke()
		}
		if r.nextCodexRegistry != nil {
			if err := r.nextCodexRegistry.Close(); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close private Next Codex MCP registrar: %w", err))
			}
		}
		acpClosed := true
		if r.acp != nil {
			if err := r.acp.Close(); err != nil {
				acpClosed = false
				closeErrors = append(closeErrors, fmt.Errorf("close ACP runtime: %w", err))
			}
		}
		if r.nextCodexStartupLock != nil && acpClosed {
			if err := r.nextCodexStartupLock.close(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		// Failed ACP Close retains the reservation: native teardown is uncertain.

		if r.acpBrowser != nil {
			if err := r.acpBrowser.Close(); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close ACP browser bridge: %w", err))
			} else if acpClosed && r.acpSharedRestartGuard != nil {
				// Never clear the cross-Core crash marker if ACP or its
				// browser bridge failed to close authoritatively.
				if err := r.acpSharedRestartGuard.releaseAfterVerifiedClose(); err != nil {
					closeErrors = append(closeErrors, fmt.Errorf("release Next shared browser restart guard: %w", err))
				}
			}
		}
		if r.acpComputer != nil {
			if err := r.acpComputer.Close(); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close ACP computer control bridge: %w", err))
			}
		}
		if r.browser != nil {
			if err := r.browser.Close(); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close browser runtime: %w", err))
			}
		}
		if r.computer != nil {
			if err := r.computer.Close(); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close computer control runtime: %w", err))
			}
		}
		if r.command != nil {
			if err := r.command.Close(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if r.dynamicMCP != nil {
			if err := r.dynamicMCP.Close(); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close dynamic MCP clients: %w", err))
			}
		}
		if r.plugins != nil {
			r.plugins.ReleaseDesktopCandidates()
			r.plugins.ReleaseMCPLeases()
		}
		// Keep permission ownership until all Core services finished teardown.
		if r.nextPermissionOwner != nil && acpClosed {
			if err := r.nextPermissionOwner.close(); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close private Next permission owner: %w", err))
			}
		}
		r.closeErr = errors.Join(closeErrors...)
	})
	return r.closeErr
}

func (r *Runtime) commandExecutionContext() (context.Context, error) {
	r.lifecycleMu.RLock()
	defer r.lifecycleMu.RUnlock()
	if r.closing || r.commandCtx == nil {
		return nil, toolError("RUNTIME_CLOSING", "AgentDock runtime is shutting down", "runtime")
	}
	return r.commandCtx, nil
}

func (r *Runtime) ToolNames() []string {
	return append([]string(nil), r.toolNames...)
}

func (r *Runtime) ToolDefinitions() []ToolDefinition {
	definitions := make([]ToolDefinition, 0, len(r.toolNames))
	for _, name := range r.toolNames {
		definition, _ := toolDefinitionForConfig(name, r.cfg)
		definitions = append(definitions, definition)
	}
	return definitions
}

func (r *Runtime) ToolDefinition(name string) (ToolDefinition, bool) {
	if _, available := r.toolValidators[name]; !available {
		return ToolDefinition{}, false
	}
	return toolDefinitionForConfig(name, r.cfg)
}

func (r *Runtime) Call(ctx context.Context, name string, args map[string]any) (result Result, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	startedAt := time.Now()
	r.observer.BeginTool()
	source := observability.SourceFromContext(ctx)
	executionTool := "unknown"
	if _, available := r.toolValidators[name]; available {
		executionTool = name
	}
	parentCtx := ctx
	ctx, span := r.tracing.StartTool(ctx, source)
	defer span.End()
	ctx = observability.WithExecution(ctx, startedAt)
	ctx, executionCall := r.execution.Begin(ctx, execution.BeginInput{Tool: executionTool, Source: string(source), InsertionSupported: true})
	// Private, generation-bound call scope for the source-only Supervisor M8
	// adapter. Does not change normal tool dispatch or permission evaluation.
	ctx = r.bindSupervisorM8RuntimeScope(ctx, executionCall)
	defer func() { r.finishExecutionCall(ctx, executionCall.ID, result, &err) }()
	defer r.observeToolCall(parentCtx, ctx, name, startedAt, &err)

	if args == nil {
		args = map[string]any{}
	}
	if err = r.validateToolArguments(name, args); err != nil {
		return nil, err
	}
	args, err = snapshotValidatedToolArguments(args)
	if err != nil {
		return nil, toolErrorDetails(
			"INVALID_ARGUMENT",
			"tool arguments cannot be normalized after validation",
			"validation",
			map[string]any{"tool": name},
		)
	}
	spec, ok := toolSpecByName(name)
	if !ok || spec.Handler == nil {
		err = toolErrorDetails("UNKNOWN_TOOL", "tool has no handler", "validation", map[string]any{"tool": name})
		return nil, err
	}
	admission, admissionErr := r.admitRuntimeTool(ctx, executionCall, name, args, source)
	if admissionErr != nil {
		err = admissionErr
		return nil, err
	}
	if err = permissionAdmissionToolError(admission); err != nil {
		return nil, err
	}
	result, err = spec.Handler(ctx, r, args)
	r.settleConsumedPermission(admission, result, err)
	if result != nil {
		r.recordExecutionResultFacts(executionCall.ID, name, args, result)
	}
	if failed, _, _ := executionResultFailure(result); err == nil && ctx.Err() == nil && result != nil && !failed {
		if deliveries := r.execution.DeliverInsertions(executionCall.ID); len(deliveries) > 0 {
			result["agentdock_user_insertions"] = deliveries
		}
	}
	return result, err
}

func (r *Runtime) validateToolArguments(name string, args map[string]any) error {
	validator, available := r.toolValidators[name]
	if !available {
		return toolErrorDetails("UNKNOWN_TOOL", "tool is not available", "validation", map[string]any{"tool": name})
	}
	if args == nil {
		args = map[string]any{}
	}
	if err := validator.Validate(args); err != nil {
		return toolErrorDetails(
			"INVALID_ARGUMENT",
			"tool arguments do not match the declared input schema",
			"validation",
			map[string]any{"tool": name, "reason": toolcontract.CompactValidationError(err)},
		)
	}
	return nil
}
