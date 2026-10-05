import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import {
  AccessLevel,
  Availability,
  OAuthPasswordState,
  PortState,
  type ConnectionSnapshot,
} from '../api/desktopApi'

const mocks = vi.hoisted(() => ({
  snapshot: vi.fn(),
  preflight: vi.fn(),
  action: vi.fn(),
  updatePort: vi.fn(),
  configureTunnel: vi.fn(),
  setAutostart: vi.fn(),
  reveal: vi.fn(),
  endpoint: vi.fn(),
}))

vi.mock('../../bindings/github.com/uvwt/agentdock/internal/desktopapi/connectionservice', () => ({
  Snapshot: mocks.snapshot,
  PreflightPort: mocks.preflight,
  Action: mocks.action,
  UpdatePort: mocks.updatePort,
  ConfigureTunnel: mocks.configureTunnel,
  SetTunnelAutostart: mocks.setAutostart,
  RevealOAuthPassword: mocks.reveal,
  TestPublicEndpoint: mocks.endpoint,
  Status: vi.fn(),
}))

import { useConnectionStore } from './connection'

function snapshot(overrides: Partial<ConnectionSnapshot> = {}): ConnectionSnapshot {
  return {
    portObservation: {
      configuredPort: 8767,
      observedPort: 8767,
      state: PortState.PortOwnedByNext,
      reasonCode: 'selected_next_process',
      observedAt: '2026-10-05T00:00:00Z',
      configRevision: 'rev-1',
    },
    coreRunning: true,
    coreHealth: 'healthy',
    tunnel: {
      registered: true,
      running: true,
      connected: true,
      autostart: 'enabled',
      token_state: 'stored',
      tunnel_generation: 'gen-1',
      public_endpoint: 'unchecked',
      recovery_required: false,
    },
    operations: [
      'snapshot',
      'preflightPort',
      'revealOAuthPassword',
      'testPublicEndpoint',
      'updatePort',
      'configureTunnel',
      'setTunnelAutostart',
      'start',
      'stop',
      'restart',
      'regenerate',
    ].map((name) => ({
      name,
      access: name === 'snapshot' || name === 'preflightPort' || name === 'revealOAuthPassword' || name === 'testPublicEndpoint'
        ? AccessLevel.AccessRead
        : AccessLevel.AccessMutating,
      requiresConfirmation: !['snapshot', 'preflightPort', 'revealOAuthPassword', 'testPublicEndpoint'].includes(name),
      availability: Availability.AvailabilityAvailable,
    })),
    localMCPURL: 'http://127.0.0.1:8767/mcp',
    publicMCPURL: 'https://next.example.test/mcp',
    port: 8767,
    mode: 'quick',
    oauthEnabled: true,
    oauthPasswordState: OAuthPasswordState.OAuthPasswordStored,
    configRevision: 'rev-1',
    tunnelGeneration: 'gen-1',
    ...overrides,
  }
}

beforeEach(() => {
  vi.resetAllMocks()
  setActivePinia(createPinia())
  mocks.snapshot.mockResolvedValue({ snapshot: snapshot() })
  mocks.action.mockResolvedValue({ completed: true, operationId: 'op-1', phase: 'applied' })
  mocks.updatePort.mockResolvedValue({ completed: true, operationId: 'op-2', phase: 'applied' })
  mocks.configureTunnel.mockResolvedValue({ completed: true, operationId: 'op-3', phase: 'applied' })
  mocks.setAutostart.mockResolvedValue({ completed: true, operationId: 'op-4', phase: 'applied' })
  mocks.endpoint.mockResolvedValue({
    state: 'reachable',
    configRevision: 'rev-1',
    tunnelGeneration: 'gen-1',
    checkedAt: '2026-10-05T00:00:01Z',
  })
})

describe('Connection store secret and revision boundaries', () => {
  it('returns an explicitly revealed password without persisting it in Pinia state', async () => {
    const secret = 'oauth-fixture-secret'
    mocks.reveal.mockResolvedValue({
      password: secret,
      state: OAuthPasswordState.OAuthPasswordStored,
      configRevision: 'rev-1',
      tunnelGeneration: 'gen-1',
    })
    const store = useConnectionStore()
    await store.refresh()

    const result = await store.revealOAuthPassword()

    expect(result?.password).toBe(secret)
    expect(JSON.stringify(store.$state)).not.toContain(secret)
  })

  it('drops stale password reveals when the returned revision no longer matches', async () => {
    mocks.reveal.mockResolvedValue({
      password: 'stale-secret',
      state: OAuthPasswordState.OAuthPasswordStored,
      configRevision: 'rev-old',
      tunnelGeneration: 'gen-1',
    })
    const store = useConnectionStore()
    await store.refresh()

    expect(await store.revealOAuthPassword()).toBeNull()
    expect(JSON.stringify(store.$state)).not.toContain('stale-secret')
  })

  it('passes a tunnel token only to the configure mutation and never stores it', async () => {
    const token = 'tunnel-token-fixture'
    const store = useConnectionStore()
    await store.refresh()

    await store.configureTunnel('named', 'https://next.example.test', token)

    expect(mocks.configureTunnel).toHaveBeenCalledWith({
      mode: 'named',
      namedOrigin: 'https://next.example.test',
      newToken: token,
      configRevision: 'rev-1',
    })
    expect(JSON.stringify(store.$state)).not.toContain(token)
  })

  it('binds lifecycle actions to the current config revision', async () => {
    const store = useConnectionStore()
    await store.refresh()

    await store.perform('restart')

    expect(mocks.action).toHaveBeenCalledWith('restart', 'rev-1')
  })

  it('ignores stale preflight and public endpoint results', async () => {
    mocks.preflight.mockResolvedValue({
      observation: {
        ...snapshot().portObservation,
        observedPort: 19000,
        state: PortState.PortAvailable,
        configRevision: 'rev-old',
      },
    })
    mocks.endpoint.mockResolvedValue({
      state: 'reachable',
      configRevision: 'rev-old',
      tunnelGeneration: 'gen-1',
      checkedAt: '2026-10-05T00:00:01Z',
    })
    const store = useConnectionStore()
    await store.refresh()

    await store.preflightPort(19000)
    await store.testPublicEndpoint()

    expect(store.preflight).toBeNull()
    expect(store.publicEndpoint).toBeNull()
  })

  it('ignores a stale public endpoint error instead of overwriting current state', async () => {
    mocks.endpoint.mockResolvedValue({
      state: 'stale',
      configRevision: 'rev-old',
      tunnelGeneration: 'gen-old',
      checkedAt: '2026-10-05T00:00:01Z',
      error: {
        code: 'connection_config_stale',
        message: 'safe',
        category: 'unavailable',
        retryable: true,
      },
    })
    const store = useConnectionStore()
    await store.refresh()

    await store.testPublicEndpoint()

    expect(store.publicEndpoint).toBeNull()
    expect(store.error).toBeNull()
  })
})
