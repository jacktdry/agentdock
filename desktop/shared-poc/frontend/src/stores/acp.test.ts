import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { ACPStatusResult, APIError } from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import { ErrorCategory } from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
const mocks = vi.hoisted(() => ({ status: vi.fn(), close: vi.fn(), update: vi.fn(), allowed: new Set<string>() }))
vi.mock('../../bindings/github.com/uvwt/agentdock/internal/desktopapi/acpservice', () => ({
  Status: mocks.status, Close: mocks.close, UpdateLifecycle: mocks.update,
}))
vi.mock('./contract', () => ({ useContractStore: () => ({ canInvoke: (_domain: string, op: string) => mocks.allowed.has(op) }) }))
vi.mock('../api/desktopApi', () => ({
  Domain: { DomainACP: 'acp' }, clientError: (code: string) => ({ code, message: '', category: 'internal', retryable: false }),
}))
import { useACPStore } from './acp'
const status: ACPStatusResult = {
  enabled: true, profiles: [], memory: { endpoint: 'http://127.0.0.1:8766/mcp', healthy: true, protocolVersion: 'v1' },
}
const error: APIError = { code: 'acp_unavailable', message: 'safe backend message', category: ErrorCategory.ErrorCategoryUnavailable, retryable: true }
beforeEach(() => {
  vi.resetAllMocks()
  setActivePinia(createPinia())
  mocks.allowed.clear()
  for (const op of ['status', 'close', 'updateLifecycle']) mocks.allowed.add(op)
  mocks.status.mockResolvedValue(status)
})
describe('ACP status', () => {
  it('loads status and preserves the last snapshot and structured error on failure', async () => {
    const store = useACPStore()
    await store.refresh()
    expect(store.status).toEqual(status)
    expect(store.state).toBe('empty')
    mocks.status.mockResolvedValue({ ...status, error })
    await store.refresh()
    expect(store.error).toEqual(error)
    expect(store.status).toEqual(status)
    expect(store.state).toBe('unavailable')
  })
  it('reports disabled and transport failure without exposing thrown secrets', async () => {
    const store = useACPStore()
    mocks.status.mockResolvedValue({ ...status, enabled: false })
    await store.refresh()
    expect(store.state).toBe('disabled')
    mocks.status.mockRejectedValue(new Error('Bearer secret'))
    await store.refresh()
    expect(store.error?.code).toBe('acp_status_failed')
    expect(JSON.stringify(store.error)).not.toContain('secret')
    expect(store.busy).toBe(false)
  })
  it('does not read when status permission is denied', async () => {
    mocks.allowed.delete('status')
    await useACPStore().refresh()
    expect(mocks.status).not.toHaveBeenCalled()
  })
})
describe.each(['close', 'updateLifecycle'] as const)('ACP %s', operation => {
  const update = { profileId: 'profile', sessionId: 'session', policy: 'idle-managed', idleCloseAfterMs: 1800000 }
  const invoke = (store: ReturnType<typeof useACPStore>) => operation === 'close' ? store.close('profile', 'session') : store.updateLifecycle(update)
  const mutation = () => operation === 'close' ? mocks.close : mocks.update
  it('uses exact binding arguments and refreshes only after completed success', async () => {
    const store = useACPStore()
    await store.refresh()
    mutation().mockResolvedValue({ completed: true })
    await invoke(store)
    expect(mutation()).toHaveBeenCalledWith(...(operation === 'close' ? ['profile', 'session'] : [update]))
    expect(mocks.status).toHaveBeenCalledTimes(2)
    expect(store.completed).toBe(true)
    expect(store.busy).toBe(false)
  })
  it('preserves mutation APIError and status without a misleading refresh', async () => {
    const store = useACPStore()
    await store.refresh()
    mutation().mockResolvedValue({ completed: false, error })
    await invoke(store)
    expect(store.error).toEqual(error)
    expect(store.status).toEqual(status)
    expect(store.completed).toBe(false)
    expect(mocks.status).toHaveBeenCalledTimes(1)
  })
  it('treats incomplete and rejected calls as failures', async () => {
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
  it('rechecks mutation permission and blocks disabled ACP', async () => {
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
  it('serializes refresh and mutations while work is pending', async () => {
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
