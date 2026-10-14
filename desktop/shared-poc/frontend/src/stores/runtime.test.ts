import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const mocks = vi.hoisted(() => ({ status: vi.fn(), action: vi.fn() }))
vi.mock('../api/desktopApi', async (importOriginal) => {
  const original = await importOriginal<typeof import('../api/desktopApi')>()
  return { ...original, desktopApi: { ...original.desktopApi, runtimeStatus: mocks.status, runtimeAction: mocks.action } }
})
import { useRuntimeStore } from './runtime'

beforeEach(() => {
  vi.resetAllMocks()
  setActivePinia(createPinia())
})
describe('Runtime observation truth', () => {
  it('does not interpret initial false defaults as an observed stopped core', async () => {
    const store = useRuntimeStore()
    expect(store.observed).toBe(false)
    expect(store.stateKind).toBe('unknown')
    expect(store.stateLabel).toBeTruthy()
    mocks.status.mockResolvedValueOnce({
      status: { running: false, healthy: false, startupEnabled: false, nexusConnected: false },
      runtimeRoot: '/tmp/next',
      error: null,
    })
    await store.refresh()
    expect(store.observed).toBe(true)
    expect(store.stateKind).toBe('stopped')
  })
  it('shows unavailable instead of falsely reporting stopped when status read fails', async () => {
    const store = useRuntimeStore()
    mocks.status.mockResolvedValueOnce({ status: null, error: {
      code: 'runtime_unavailable', category: 'unavailable', message: '', retryable: true,
    } })
    await store.refresh()
    expect(store.stateKind).toBe('unavailable')
    expect(store.observed).toBe(false)
  })
})
