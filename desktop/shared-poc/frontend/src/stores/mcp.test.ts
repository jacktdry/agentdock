import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const mocks = vi.hoisted(() => ({
  snapshot: vi.fn(),
  inspect: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  setEnabled: vi.fn(),
  environment: vi.fn(),
  setEnvironment: vi.fn(),
  unsetEnvironment: vi.fn(),
  purgeEnvironment: vi.fn(),
  reconnect: vi.fn(),
  authorizationStatus: vi.fn(),
  authorizationFlowStatus: vi.fn(),
  authorize: vi.fn(),
  clearAuthorization: vi.fn(),
  operationStatus: vi.fn(),
}))

vi.mock('../api/desktopApi', () => ({
  Domain: { DomainMCP: 'mcp' },
  clientError: (code: string) => ({ code, message: '', category: 'internal', retryable: false }),
  desktopApi: {
    mcpSnapshot: mocks.snapshot,
    mcpInspect: mocks.inspect,
    mcpCreate: mocks.create,
    mcpUpdate: mocks.update,
    mcpRemove: mocks.remove,
    mcpSetEnabled: mocks.setEnabled,
    mcpEnvironment: mocks.environment,
    mcpSetEnvironment: mocks.setEnvironment,
    mcpUnsetEnvironment: mocks.unsetEnvironment,
    mcpPurgeEnvironment: mocks.purgeEnvironment,
    mcpReconnect: mocks.reconnect,
    mcpAuthorizationStatus: mocks.authorizationStatus,
    mcpAuthorizationFlowStatus: mocks.authorizationFlowStatus,
    mcpAuthorize: mocks.authorize,
    mcpClearAuthorization: mocks.clearAuthorization,
    mcpOperationStatus: mocks.operationStatus,
  },
}))

vi.mock('./contract', () => ({
  useContractStore: () => ({ canInvoke: () => true }),
}))

import { useMCPStore } from './mcp'
import type { MCPManagedServer, MCPSnapshot } from '../api/desktopApi'

const server: MCPManagedServer = {
  name: 'demo',
  displayName: 'demo',
  description: 'Demo',
  sourceType: 'standalone',
  pluginName: '',
  transport: 'streamable_http',
  protocolVersion: '',
  url: 'https://example.test/mcp',
  urlProtected: false,
  command: '',
  args: [],
  argsProtected: false,
  cwd: '',
  enabled: false,
  timeoutMs: 30000,
  headerEnv: {},
  envFromEnv: {},
  generation: 'gen-1',
  environmentConfigured: false,
  enableRequiresEnvironmentConfirmation: false,
  blockedReasons: [],
  observation: {
    connection: 'not_connected',
    status: 'idle',
    authStatus: 'unauthorized',
    toolCount: null,
    observedAt: '',
    lastErrorCode: '',
    stale: false,
  },
}

const snapshot: MCPSnapshot = {
  registryRevision: 'rev-1',
  authoritative: true,
  servers: [server],
}

beforeEach(() => {
  vi.resetAllMocks()
  setActivePinia(createPinia())
  mocks.snapshot.mockResolvedValue({ snapshot, error: null })
  mocks.inspect.mockResolvedValue({ registryRevision: 'rev-1', server, tools: [], toolCount: 0, error: null })
  mocks.environment.mockResolvedValue({ revision: 'env-1', items: [], error: null })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('mcp_store_authority', () => {
  it('retains prior inventory as stale and disables mutation after passive read failure', async () => {
    const store = useMCPStore()
    await store.refresh()
    expect(store.servers).toHaveLength(1)

    mocks.snapshot.mockResolvedValueOnce({
      snapshot: { registryRevision: '', authoritative: false, servers: [] },
      error: { code: 'MCP_CORE_UNAVAILABLE', message: 'unavailable', category: 'unavailable', retryable: true },
    })
    await store.refresh()

    expect(store.stale).toBe(true)
    expect(store.servers[0]?.name).toBe('demo')
    expect(store.canInvoke('create')).toBe(false)
    await store.create({ name: 'other', description: 'Other', transport: 'stdio', command: '/bin/true' }, 'rev-1')
    expect(mocks.create).not.toHaveBeenCalled()
  })

  it('never retains a scoped secret value in Pinia state', async () => {
    const store = useMCPStore()
    await store.refresh()
    const canary = 'SECRET_CANARY_mcp_store_never_keep'
    mocks.setEnvironment.mockResolvedValue({
      requestId: 'a'.repeat(32),
      outcome: 'completed',
      outcomeUnknown: false,
      runtimeImpact: 'next_connection',
      reconnectRequired: true,
      revision: 'env-2',
      items: [{ key: 'TOKEN', configured: true }],
      error: null,
    })

    await store.setEnvironment(server, 'TOKEN', canary, 'rev-1', 'gen-1', 'env-1')

    expect(mocks.setEnvironment).toHaveBeenCalledWith(expect.stringMatching(/^[0-9a-f]{32}$/), 'demo', 'TOKEN', canary, 'rev-1', 'gen-1', 'env-1')
    expect(JSON.stringify(store.$state)).not.toContain(canary)
  })

  it('reconciles outcome unknown by request id without replaying the mutation', async () => {
    const store = useMCPStore()
    await store.refresh()
    mocks.remove.mockImplementation(async (requestId: string) => ({
      requestId,
      outcome: 'outcome_unknown',
      outcomeUnknown: true,
      completed: false,
      persisted: false,
      runtimeApplied: false,
      reconnectRequired: false,
      recoveryRequired: false,
      error: { code: 'MCP_TIMEOUT', message: 'timeout', category: 'timeout', retryable: true },
    }))
    mocks.operationStatus.mockImplementation(async (requestId: string) => ({
      requestId,
      found: true,
      pending: false,
      outcome: 'completed',
      outcomeUnknown: false,
      operationAction: 'desktop_remove',
      startedAt: '',
      completedAt: '',
      error: null,
    }))

    await store.remove(server, 'rev-1', 'gen-1')

    expect(mocks.remove).toHaveBeenCalledTimes(1)
    expect(mocks.operationStatus).toHaveBeenCalledTimes(1)
    expect(store.lastOperation?.outcome).toBe('completed')
  })


  it('loads cached tool summaries passively and accepts plugin-owned inspection', async () => {
    const store = useMCPStore()
    await store.refresh()
    const plugin = { ...server, name: 'plugin.demo', sourceType: 'plugin', pluginName: 'demo' }
    mocks.inspect.mockResolvedValueOnce({
      registryRevision: 'rev-1',
      server: plugin,
      tools: [{ name: 'cached_tool', qualifiedName: 'plugin.demo.cached_tool', server: 'plugin.demo', sourceType: 'plugin', pluginName: 'demo' }],
      toolCount: 1,
      error: null,
    })

    await store.inspect(plugin)

    expect(mocks.inspect).toHaveBeenCalledExactlyOnceWith('plugin.demo')
    expect(store.toolsFor('plugin.demo').map((tool) => tool.name)).toEqual(['cached_tool'])
  })

  it('blocks every P5 mutation path for plugin-owned servers', async () => {
    const store = useMCPStore()
    await store.refresh()
    const plugin = { ...server, name: 'plugin.demo', sourceType: 'plugin', pluginName: 'demo' }

    await store.update(plugin, { description: 'x' }, 'rev-1', 'gen-1')
    await store.remove(plugin, 'rev-1', 'gen-1')
    await store.setEnabled(plugin, true, false, 'rev-1', 'gen-1')
    await store.reconnect(plugin, 'rev-1', 'gen-1')
    await store.authorize(plugin, 'local', 'rev-1', 'gen-1')
    await store.clearAuthorization(plugin, 'rev-1', 'gen-1')

    expect(mocks.update).not.toHaveBeenCalled()
    expect(mocks.remove).not.toHaveBeenCalled()
    expect(mocks.setEnabled).not.toHaveBeenCalled()
    expect(mocks.reconnect).not.toHaveBeenCalled()
    expect(mocks.authorize).not.toHaveBeenCalled()
    expect(mocks.clearAuthorization).not.toHaveBeenCalled()
  })

  it('passes explicit retained-environment reuse only from the confirmed enable path', async () => {
    const store = useMCPStore()
    await store.refresh()
    const retained = { ...server, environmentConfigured: true, enableRequiresEnvironmentConfirmation: true }
    mocks.setEnabled.mockImplementation(async (requestId: string) => ({
      requestId,
      outcome: 'completed',
      outcomeUnknown: false,
      completed: true,
      persisted: true,
      runtimeApplied: true,
      reconnectRequired: false,
      recoveryRequired: false,
      registryRevision: 'rev-2',
      server: { ...retained, enabled: true },
      error: null,
    }))

    await store.setEnabled(retained, true, true, 'rev-1', 'gen-1')

    expect(mocks.setEnabled).toHaveBeenCalledWith(
      expect.stringMatching(/^[0-9a-f]{32}$/),
      'demo',
      true,
      true,
      'rev-1',
      'gen-1',
    )
  })

  it('cancels OAuth polling when the view releases the flow', async () => {
    vi.useFakeTimers()
    const store = useMCPStore()
    await store.refresh()
    mocks.authorize.mockImplementation(async (requestId: string) => ({
      requestId,
      outcome: 'completed',
      outcomeUnknown: false,
      runtimeImpact: 'authorization_pending',
      reconnectRequired: false,
      flowId: 'flow-stop',
      callbackId: 'local',
      expiresAt: '2099-01-01T00:00:00Z',
      callbackOptions: [],
      error: null,
    }))

    await store.authorize(server, 'local', 'rev-1', 'gen-1')
    store.stopOAuthPolling()
    await vi.advanceTimersByTimeAsync(4000)

    expect(mocks.authorizationFlowStatus).not.toHaveBeenCalled()
  })

  it('polls OAuth by opaque flow id while the native backend owns the browser handoff', async () => {
    vi.useFakeTimers()
    const store = useMCPStore()
    await store.refresh()
    mocks.authorize.mockImplementation(async (requestId: string) => ({
      requestId,
      outcome: 'completed',
      outcomeUnknown: false,
      runtimeImpact: 'authorization_pending',
      reconnectRequired: false,
      flowId: 'flow-safe',
      callbackId: 'local',
      expiresAt: '2099-01-01T00:00:00Z',
      callbackOptions: [],
      error: null,
    }))
    mocks.authorizationFlowStatus.mockResolvedValue({
      authorization: {
        flowId: 'flow-safe',
        status: 'denied',
        expiresAt: '2099-01-01T00:00:00Z',
        errorCode: 'MCP_AUTH_DENIED',
        callbackOptions: [],
      },
      error: null,
    })

    await store.authorize(server, 'local', 'rev-1', 'gen-1')
    expect(store.authorization?.flowId).toBe('flow-safe')
    expect(JSON.stringify(store.$state)).not.toContain('provider.example')

    await vi.advanceTimersByTimeAsync(2000)
    expect(store.authorization?.status).toBe('denied')
    expect(mocks.authorizationFlowStatus).toHaveBeenCalledTimes(1)
    store.stopOAuthPolling()
  })
})
