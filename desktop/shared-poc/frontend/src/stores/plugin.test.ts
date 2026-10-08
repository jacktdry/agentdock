import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const mocks = vi.hoisted(() => ({
  snapshot: vi.fn(),
  inspect: vi.fn(),
  chooseCandidate: vi.fn(),
  discardCandidate: vi.fn(),
  installCandidate: vi.fn(),
  updateCandidate: vi.fn(),
  setEnabled: vi.fn(),
  removeKeep: vi.fn(),
  removePurge: vi.fn(),
  environment: vi.fn(),
  setEnvironment: vi.fn(),
  unsetEnvironment: vi.fn(),
  operationStatus: vi.fn(),
}))

vi.mock('../api/desktopApi', () => ({
  Domain: { DomainPlugin: 'plugin' },
  clientError: (code: string) => ({ code, message: '', category: 'internal', retryable: false }),
  desktopApi: {
    pluginSnapshot: mocks.snapshot,
    pluginInspect: mocks.inspect,
    pluginChooseCandidate: mocks.chooseCandidate,
    pluginDiscardCandidate: mocks.discardCandidate,
    pluginInstallCandidate: mocks.installCandidate,
    pluginUpdateCandidate: mocks.updateCandidate,
    pluginSetEnabled: mocks.setEnabled,
    pluginRemoveKeep: mocks.removeKeep,
    pluginRemovePurge: mocks.removePurge,
    pluginEnvironment: mocks.environment,
    pluginSetEnvironment: mocks.setEnvironment,
    pluginUnsetEnvironment: mocks.unsetEnvironment,
    pluginOperationStatus: mocks.operationStatus,
  },
}))

vi.mock('./contract', () => ({
  useContractStore: () => ({ canInvoke: () => true }),
}))

import { usePluginStore } from './plugin'
import type { PluginCandidate, PluginManagedItem, PluginManagerSnapshot } from '../api/desktopApi'

const plugin: PluginManagedItem = {
  name: 'demo',
  description: 'Demo Plugin',
  version: '1.0.0',
  format: 'portable',
  enabled: false,
  generation: 'a'.repeat(64),
  installedAt: '2026-10-08T00:00:00Z',
  skillsCount: 1,
  mcpCount: 1,
  warningCount: 0,
  packageFingerprint: 'sha256:demo',
  recoveryState: '',
}

const snapshot: PluginManagerSnapshot = {
  registryRevision: 'b'.repeat(64),
  authoritative: true,
  plugins: [plugin],
  recoveryItems: [],
}

const candidate: PluginCandidate = {
  candidateId: 'c'.repeat(64),
  kind: 'install',
  targetName: '',
  expiresAt: '2026-10-08T10:30:00Z',
  review: {
    valid: true,
    name: 'candidate',
    version: '1.0.0',
    description: '',
    format: 'portable',
    packageFingerprint: 'sha256:candidate',
    skills: [],
    mcp: [],
    executables: [],
    warnings: [],
    issues: [],
  },
}

beforeEach(() => {
  vi.resetAllMocks()
  setActivePinia(createPinia())
  mocks.snapshot.mockResolvedValue({ snapshot, error: null })
  mocks.inspect.mockResolvedValue({
    registryRevision: snapshot.registryRevision,
    detail: { plugin, mcp: [], warnings: [] },
    error: null,
  })
  mocks.environment.mockResolvedValue({ envRevision: 'env-1', items: [{ key: 'TOKEN', configured: false }], error: null })
})

describe('plugin_store_authority', () => {
  it('retains prior inventory as stale and disables mutation after passive read failure', async () => {
    const store = usePluginStore()
    await store.refresh()
    expect(store.plugins).toHaveLength(1)

    mocks.snapshot.mockResolvedValueOnce({
      snapshot: { registryRevision: '', authoritative: false, plugins: [], recoveryItems: [] },
      error: { code: 'PLUGIN_CORE_UNAVAILABLE', message: 'unavailable', category: 'unavailable', retryable: true },
    })
    await store.refresh()

    expect(store.stale).toBe(true)
    expect(store.plugins[0]?.name).toBe('demo')
    expect(store.canInvoke('setEnabled')).toBe(false)
    await store.setEnabled(plugin, true)
    expect(mocks.setEnabled).not.toHaveBeenCalled()
  })

  it('does not retain write-only connection values in Pinia state', async () => {
    const store = usePluginStore()
    await store.refresh()
    await store.readEnvironment(plugin, 'remote')
    const canary = 'SECRET_CANARY_plugin_store_never_keep'
    mocks.setEnvironment.mockResolvedValue({
      requestId: 'd'.repeat(32),
      outcome: 'completed',
      outcomeUnknown: false,
      completed: true,
      persisted: true,
      runtimeApplied: false,
      recoveryRequired: false,
      runtimeImpact: 'next_connection',
      reconnectRequired: true,
      envRevision: 'env-2',
      items: [{ key: 'TOKEN', configured: true }],
      error: null,
    })

    await store.setEnvironment(plugin, 'remote', 'TOKEN', canary)
    expect(mocks.setEnvironment).toHaveBeenCalledWith(expect.objectContaining({
      name: 'demo',
      expectedRegistryRevision: snapshot.registryRevision,
      expectedGeneration: plugin.generation,
      expectedEnvRevision: 'env-1',
      key: 'TOKEN',
      value: canary,
    }))
    expect(JSON.stringify(store.$state)).not.toContain(canary)
  })

  it('uses opaque candidate id with current registry revision and clears it after install', async () => {
    const store = usePluginStore()
    await store.refresh()
    mocks.chooseCandidate.mockResolvedValue({ candidate, sourceLabel: 'candidate.zip', cancelled: false, error: null })
    await store.chooseCandidate('install', 'zip')
    expect(store.candidate?.candidateId).toBe(candidate.candidateId)

    mocks.installCandidate.mockResolvedValue({
      requestId: 'e'.repeat(32),
      outcome: 'completed',
      outcomeUnknown: false,
      completed: true,
      persisted: true,
      runtimeApplied: true,
      recoveryRequired: false,
      runtimeImpact: 'installed_disabled',
      error: null,
    })
    await store.installCandidate()

    expect(mocks.installCandidate).toHaveBeenCalledWith(expect.objectContaining({
      candidateId: candidate.candidateId,
      name: candidate.review.name,
      expectedRegistryRevision: snapshot.registryRevision,
    }))
    expect(store.candidate).toBeNull()
  })

  it('retries purge recovery with the recovery incarnation fence', async () => {
    const recovery = { name: 'removed-demo', generation: 'f'.repeat(64), state: 'cleanup_required' }
    mocks.snapshot.mockResolvedValue({
      snapshot: { ...snapshot, recoveryItems: [recovery] },
      error: null,
    })
    mocks.removePurge.mockResolvedValue({
      requestId: '1'.repeat(32),
      outcome: 'completed',
      outcomeUnknown: false,
      completed: true,
      persisted: true,
      runtimeApplied: true,
      recoveryRequired: false,
      runtimeImpact: 'applied',
      error: null,
    })
    const store = usePluginStore()
    await store.refresh()
    await store.retryPurge(recovery)

    expect(mocks.removePurge).toHaveBeenCalledWith(expect.objectContaining({
      name: recovery.name,
      expectedRegistryRevision: snapshot.registryRevision,
      expectedGeneration: recovery.generation,
    }))
  })
})
