package client

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/uvwt/agentdock/internal/envstore"
)

// desktopTarget is used only while registryMu and the registry file lock are
// held. Passive reads may omit identity tokens; side effects must use
// desktopMutationTarget so delete/recreate cannot retarget a request by name.
func (m *Manager) desktopTarget(snapshot RegistrySnapshot, name, revision, generation string) (ServerConfig, error) {
	r, err := m.registryWithOwned(snapshot)
	if err != nil {
		return ServerConfig{}, err
	}
	cfg, ok := r.Servers[strings.TrimSpace(name)]
	if !ok {
		return ServerConfig{}, newError("MCP_SERVER_NOT_FOUND", "MCP server not found", false, nil, nil)
	}
	if cfg.SourceType != "standalone" {
		return ServerConfig{}, newError("MCP_OWNED_BY_PLUGIN", "Plugin owns this MCP server", false, nil, nil)
	}
	if revision != "" && revision != r.Revision {
		return ServerConfig{}, newError("MCP_REGISTRY_CONFLICT", "MCP registry revision changed", false, nil, nil)
	}
	if generation != "" && generation != cfg.Generation {
		return ServerConfig{}, newError("MCP_SERVER_GENERATION_CONFLICT", "MCP server generation changed", false, nil, nil)
	}
	return cfg, nil
}

func (m *Manager) desktopMutationTarget(snapshot RegistrySnapshot, name, revision, generation string) (ServerConfig, error) {
	cfg, err := m.desktopTarget(snapshot, name, revision, generation)
	if err != nil {
		return ServerConfig{}, err
	}
	if strings.TrimSpace(revision) == "" {
		return ServerConfig{}, newError("MCP_REGISTRY_CONFLICT", "MCP registry revision is required", false, nil, nil)
	}
	if strings.TrimSpace(generation) == "" {
		return ServerConfig{}, newError("MCP_SERVER_GENERATION_CONFLICT", "MCP server generation is required", false, nil, nil)
	}
	return cfg, nil
}

// DesktopEnvironment pins authoritative ownership throughout the checked env
// operation, including against other Manager instances' registry mutations.
func (m *Manager) DesktopEnvironment(name, operation, key string, value *string, expectedEnv, revision, generation string) (envstore.Snapshot, error) {
	m.registryMu.Lock()
	defer m.registryMu.Unlock()
	if err := m.ensureOpenLocked(); err != nil {
		return envstore.Snapshot{}, err
	}
	var result envstore.Snapshot
	err := m.store.withSnapshotLocked(func(snapshot RegistrySnapshot) error {
		var cfg ServerConfig
		var err error
		if operation == "snapshot" {
			cfg, err = m.desktopTarget(snapshot, name, revision, generation)
		} else {
			cfg, err = m.desktopMutationTarget(snapshot, name, revision, generation)
		}
		if err != nil {
			return err
		}
		scope := envstore.Scope{Kind: envstore.ScopeMCP, Name: cfg.StorageKey}
		if operation != "snapshot" && expectedEnv == "" {
			return newError("MCP_ENV_CONFLICT", "Environment revision is required", false, nil, nil)
		}
		switch operation {
		case "snapshot":
			result, err = m.envs.Snapshot(scope)
		case "set":
			if value == nil {
				return newError("MCP_CONFIG_INVALID", "Environment value is required", false, nil, nil)
			}
			result, err = m.envs.SetChecked(scope, strings.TrimSpace(key), *value, expectedEnv)
		case "unset":
			_, result, err = m.envs.UnsetChecked(scope, strings.TrimSpace(key), expectedEnv)
		case "purge":
			result, err = m.envs.PurgeChecked(scope, expectedEnv)
		default:
			return newError("MCP_CONFIG_INVALID", "Unsupported environment operation", false, nil, nil)
		}
		if errors.Is(err, envstore.ErrRevisionConflict) {
			return newError("MCP_ENV_CONFLICT", "Environment revision changed", false, nil, nil)
		}
		if err != nil {
			return newError("MCP_ENV_ERROR", "Environment operation failed", false, nil, nil)
		}
		return nil
	})
	return result, err
}

// DesktopReconnect explicitly reserves the authoritative standalone runtime.
// The state lock stays pinned after authoritative validation and through I/O,
// so a same-Manager removal/replacement cannot redirect the effect to a new MCP.
func (m *Manager) DesktopReconnect(ctx context.Context, name, revision, generation string) (ProtectedServer, []ToolSummary, error) {
	m.registryMu.Lock()
	if err := m.ensureOpenLocked(); err != nil {
		m.registryMu.Unlock()
		return ProtectedServer{}, nil, err
	}
	var cfg ServerConfig
	var state *serverState
	err := m.store.withSnapshotLocked(func(snapshot RegistrySnapshot) error {
		var err error
		cfg, err = m.desktopMutationTarget(snapshot, name, revision, generation)
		if err != nil {
			return err
		}
		if !cfg.Enabled {
			return newError("MCP_SERVER_DISABLED", "MCP server is disabled", false, nil, nil)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		state = m.states[cfg.Name]
		if state == nil {
			state = &serverState{}
			m.states[cfg.Name] = state
		}
		state.mu.Lock()
		if state.generation == "" {
			state.generation = m.servers[cfg.Name].Generation
		}
		m.servers[cfg.Name] = cfg
		return nil
	})
	m.registryMu.Unlock()
	if err != nil {
		return ProtectedServer{}, nil, err
	}
	runtimeCfg, err := m.runtimeConfig(cfg)
	var tools map[string]Tool
	if err == nil {
		callCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutMS)*time.Millisecond)
		tools, err = m.refreshStateLocked(callCtx, runtimeCfg, state)
		cancel()
		if err == nil {
			state.generation = cfg.Generation
		}
	} else {
		recordStateError(state, err)
	}
	state.mu.Unlock()
	items := summarizeTools(cfg, tools)
	for i := range items {
		items[i].Name = protectedText(items[i].Name)
		items[i].QualifiedName = qualifiedToolName(cfg.Name, items[i].Name)
		// Provider prose is not part of the protected lightweight catalog.
		items[i].Title, items[i].Description = "", ""
	}
	return m.ProtectedServer(cfg), items, err
}

func (m *Manager) DesktopAuthorize(ctx context.Context, name, callbackID, revision, generation string) (ProtectedAuthorization, error) {
	cfg, reservation, err := m.reserveAuthorizationExpected(name, callbackID, revision, generation, true)
	if err != nil {
		return ProtectedAuthorization{}, err
	}
	auth, err := m.beginDesktopAuthorization(ctx, cfg, reservation)
	if err != nil {
		return ProtectedAuthorization{}, err
	}
	result := ProtectedAuthorization{FlowID: auth.FlowID, AuthorizationURL: auth.AuthorizationURL, CallbackID: auth.CallbackID, ExpiresAt: auth.ExpiresAt}
	for _, option := range auth.CallbackOptions {
		result.CallbackOptions = append(result.CallbackOptions, ProtectedCallback{ID: option.ID, Label: protectedText(option.Label)})
	}
	return result, nil
}

func (m *Manager) DesktopClearAuthorization(name, revision, generation string) error {
	m.registryMu.Lock()
	defer m.registryMu.Unlock()
	if err := m.ensureOpenLocked(); err != nil {
		return err
	}
	var state *serverState
	err := m.store.withSnapshotLocked(func(snapshot RegistrySnapshot) error {
		cfg, err := m.desktopMutationTarget(snapshot, name, revision, generation)
		if err != nil {
			return err
		}
		if err := m.oauth.Clear(cfg.StorageKey); err != nil {
			return newError("MCP_AUTH_CLEAR_FAILED", "Local authorization clear failed", false, nil, nil)
		}
		m.mu.RLock()
		state = m.states[cfg.Name]
		if state != nil {
			state.mu.Lock()
		}
		m.mu.RUnlock()
		return nil
	})
	if err != nil {
		return err
	}
	if state == nil {
		return nil
	}
	defer state.mu.Unlock()
	// Client cleanup can perform transport I/O; release the registry file lock
	// first, retaining the pinned state and Manager ownership transition lock.
	return closeStateLocked(state)
}
