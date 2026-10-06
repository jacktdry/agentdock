import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { ACPManagerSnapshot, ACPStatusResult, APIError } from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import { ErrorCategory } from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'

const mocks = vi.hoisted(() => ({
  status: vi.fn(),
  settings: vi.fn(),
  saveSettings: vi.fn(),
  close: vi.fn(),
  update: vi.fn(),
  allowed: new Set<string>(),
}))

vi.mock('../../bindings/github.com/uvwt/agentdock/internal/desktopapi/acpservice', () => ({
  Status: mocks.status,
  Settings: mocks.settings,
  SaveSettings: mocks.saveSettings,
  Close: mocks.close,
  UpdateLifecycle: mocks.update,
}))
vi.mock('./contract', () => ({ useContractStore: () => ({ canInvoke: (_domain: string, op: string) => mocks.allowed.has(op) }) }))
vi.mock('../api/desktopApi', () => ({
  Domain: { DomainACP: 'acp' },
  clientError: (code: string) => ({ code, message: '', category: 'internal', retryable: false }),
}))

import { useACPStore } from './acp'

const status: ACPStatusResult = {
  enabled: true,
  profiles: [],
  memory: { endpoint: 'http://127.0.0.1:8766/mcp', healthy: true, protocolVersion: 'v1' },
}
const manager: ACPManagerSnapshot = {
  revision: 'rev-1',
  enabled: true,
  defaultProfile: '',
  profiles: [],
  capabilities: [],
}
const error: APIError = {
  code: 'acp_unavailable',
  message: 'safe backend message',
  category: ErrorCategory.ErrorCategoryUnavailable,
  retryable: true,
}

beforeEach(() => {
  vi.resetAllMocks()
  setActivePinia(createPinia())
  mocks.allowed.clear()
  for (const op of ['status', 'settings', 'saveSettings', 'close', 'updateLifecycle']) mocks.allowed.add(op)
  mocks.status.mockResolvedValue(status)
  mocks.settings.mockResolvedValue(manager)
})

describe('ACP snapshots', () => {
  it('loads settings and status while deriving page state from configuration inventory', async () => {
    const store = useACPStore()
    await store.refresh()
    expect(store.status).toEqual(status)
    expect(store.settings).toEqual(manager)
    expect(store.state).toBe('empty')

    mocks.settings.mockResolvedValue({ ...manager, enabled: false })
    mocks.status.mockResolvedValue({ ...status, enabled: false })
    await store.refresh()
    expect(store.state).toBe('disabled')
  })

  it('preserves last snapshots and safe structured errors on independent failures', async () => {
    const store = useACPStore()
    await store.refresh()

    mocks.status.mockRejectedValue(new Error('Bearer secret'))
    await store.refresh()
    expect(store.error?.code).toBe('acp_status_failed')
    expect(store.status).toEqual(status)
    expect(store.settings).toEqual(manager)
    expect(JSON.stringify(store.error)).not.toContain('secret')

    mocks.status.mockResolvedValue(status)
    mocks.settings.mockResolvedValue({ ...manager, error })
    await store.refresh()
    expect(store.error).toEqual(error)
    expect(store.settings).toEqual(manager)
    expect(store.state).toBe('unavailable')
  })

  it('reads whichever ACP snapshot capability is available', async () => {
    mocks.allowed.delete('status')
    await useACPStore().refresh()
    expect(mocks.status).not.toHaveBeenCalled()
    expect(mocks.settings).toHaveBeenCalledTimes(1)

    mocks.status.mockClear()
    mocks.settings.mockClear()
    mocks.allowed.add('status')
    mocks.allowed.delete('settings')
    setActivePinia(createPinia())
    await useACPStore().refresh()
    expect(mocks.status).toHaveBeenCalledTimes(1)
    expect(mocks.settings).not.toHaveBeenCalled()
  })
})

describe('ACP settings mutations', () => {
  const configured: ACPManagerSnapshot = {
    revision: 'rev-1',
    enabled: false,
    defaultProfile: '',
    capabilities: [],
    profiles: [{
      id: 'codex',
      displayName: 'Codex',
      runtimeKind: 'codex',
      preset: 'codex',
      source: 'builtin',
      enabled: false,
      configuredCommand: '/opt/codex-acp',
      configuredArgs: [],
      availability: 'unknown',
      versionState: 'not_checked',
      canDetect: false,
      canUpdate: false,
    }],
  }

  it('sends the exact revisioned profile array and keeps restart-required visible', async () => {
    mocks.settings.mockResolvedValue(configured)
    const store = useACPStore()
    await store.refresh()

    const snapshot = {
      ...configured,
      revision: 'rev-2',
      profiles: [{ ...configured.profiles![0], displayName: 'Codex Updated' }],
    }
    mocks.saveSettings.mockResolvedValue({
      completed: true,
      persisted: true,
      applied: false,
      restartRequired: true,
      runtimeImpact: 'existing_runtime_unchanged',
      snapshot,
    })

    const ok = await store.upsertProfile({
      id: 'codex',
      displayName: 'Codex Updated',
      kind: 'codex',
      command: '/opt/codex-acp',
      args: [],
      enabled: false,
    }, 'codex')

    expect(ok).toBe(true)
    expect(mocks.saveSettings).toHaveBeenCalledWith('rev-1', false, '', [{
      id: 'codex',
      displayName: 'Codex Updated',
      kind: 'codex',
      command: '/opt/codex-acp',
      args: [],
      enabled: false,
    }])
    expect(store.settings).toEqual(snapshot)
    expect(store.restartRequired).toBe(true)
    expect(store.completed).toBe(true)
  })

  it('preserves the current snapshot when a stale revision is rejected', async () => {
    mocks.settings.mockResolvedValue(configured)
    const store = useACPStore()
    await store.refresh()
    const conflict: APIError = {
      code: 'acp_settings_conflict',
      message: 'reload',
      category: ErrorCategory.ErrorCategoryConflict,
      retryable: false,
    }
    mocks.saveSettings.mockResolvedValue({
      completed: false,
      persisted: false,
      applied: false,
      restartRequired: false,
      runtimeImpact: '',
      error: conflict,
    })

    expect(await store.setGlobalEnabled(false)).toBe(false)
    expect(store.error).toEqual(conflict)
    expect(store.settings).toEqual(configured)
    expect(store.restartRequired).toBe(false)
  })

  it('blocks disabling or deleting the default profile before calling the backend', async () => {
    const defaultManager: ACPManagerSnapshot = {
      ...configured,
      enabled: true,
      defaultProfile: 'codex',
      profiles: [{ ...configured.profiles![0], enabled: true }],
    }
    mocks.settings.mockResolvedValue(defaultManager)
    const store = useACPStore()
    await store.refresh()

    expect(await store.setProfileEnabled('codex', false)).toBe(false)
    expect(await store.removeProfile('codex')).toBe(false)
    expect(mocks.saveSettings).not.toHaveBeenCalled()
  })

  it('requires an enabled default with a command before turning ACP on', async () => {
    mocks.settings.mockResolvedValue(configured)
    const store = useACPStore()
    await store.refresh()
    expect(store.canEnableACP).toBe(false)
    expect(await store.setGlobalEnabled(true)).toBe(false)
    expect(mocks.saveSettings).not.toHaveBeenCalled()
  })
})

describe.each(['close', 'updateLifecycle'] as const)('ACP %s', operation => {
  const update = { profileId: 'profile', sessionId: 'session', policy: 'idle-managed', idleCloseAfterMs: 1800000 }
  const invoke = (store: ReturnType<typeof useACPStore>) => operation === 'close'
    ? store.close('profile', 'session')
    : store.updateLifecycle(update)
  const mutation = () => operation === 'close' ? mocks.close : mocks.update

  it('uses exact binding arguments and refreshes runtime only after completed success', async () => {
    const store = useACPStore()
    await store.refresh()
    mutation().mockResolvedValue({ completed: true })
    await invoke(store)
    expect(mutation()).toHaveBeenCalledWith(...(operation === 'close' ? ['profile', 'session'] : [update]))
    expect(mocks.status).toHaveBeenCalledTimes(2)
    expect(mocks.settings).toHaveBeenCalledTimes(1)
    expect(store.completed).toBe(true)
    expect(store.busy).toBe(false)
  })

  it('preserves mutation APIError and runtime status without a misleading refresh', async () => {
    const store = useACPStore()
    await store.refresh()
    mutation().mockResolvedValue({ completed: false, error })
    await invoke(store)
    expect(store.error).toEqual(error)
    expect(store.status).toEqual(status)
    expect(store.completed).toBe(false)
    expect(mocks.status).toHaveBeenCalledTimes(1)
  })

  it('treats incomplete and rejected calls as safe failures', async () => {
    const store = useACPStore()
    await store.refresh()
    mutation().mockResolvedValue({ completed: false })
    await invoke(store)
    expect(store.error?.code).toBe('acp_mutation_incomplete')
    mutation().mockRejectedValue(new Error('Bearer secret'))
    await invoke(store)
    expect(store.error?.code).toBe('acp_mutation_failed')
    expect(JSON.stringify(store.error)).not.toContain('secret')
    expect(store.busy).toBe(false)
  })

  it('rechecks mutation permission and blocks runtime actions while ACP is disabled', async () => {
    const store = useACPStore()
    await store.refresh()
    mocks.allowed.delete(operation)
    expect(store.canInvoke(operation)).toBe(false)
    await invoke(store)
    expect(mutation()).not.toHaveBeenCalled()

    mocks.allowed.add(operation)
    mocks.status.mockResolvedValue({ ...status, enabled: false })
    await store.refresh()
    await invoke(store)
    expect(mutation()).not.toHaveBeenCalled()
  })

  it('serializes refresh and runtime mutations while work is pending', async () => {
    const store = useACPStore()
    await store.refresh()
    let resolve!: (value: { completed: boolean }) => void
    mutation().mockImplementation(() => new Promise(r => { resolve = r }))
    const pending = invoke(store)
    expect(store.busy).toBe(true)
    await store.refresh()
    await invoke(store)
    expect(mutation()).toHaveBeenCalledTimes(1)
    expect(mocks.status).toHaveBeenCalledTimes(1)
    resolve({ completed: true })
    await pending
    expect(mocks.status).toHaveBeenCalledTimes(2)
  })
})
