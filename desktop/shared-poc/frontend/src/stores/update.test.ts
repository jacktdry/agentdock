import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const mocks = vi.hoisted(() => ({ check: vi.fn() }))
vi.mock('../api/desktopApi', async (importOriginal) => {
  const original = await importOriginal<typeof import('../api/desktopApi')>()
  return { ...original, desktopApi: { ...original.desktopApi, checkUpdate: mocks.check } }
})
import { useUpdateStore } from './update'

beforeEach(() => {
  vi.resetAllMocks()
  setActivePinia(createPinia())
})
describe('Intentional Next update-channel disablement', () => {
  it('exposes an intentional disabled state without pretending to have checked versions', async () => {
    mocks.check.mockResolvedValue({
      status: null,
      error: { code: 'next_update_channel_disabled', category: 'unavailable', retryable: false, message: '' },
    })
    const store = useUpdateStore()
    await store.refresh()
    expect(store.channelDisabled).toBe(true)
    expect(store.data).toBeNull()
    expect(store.error?.code).toBe('next_update_channel_disabled')
  })
  it('does not conflate network or transport failures with intentionally disabled updates', async () => {
    mocks.check.mockRejectedValue(new Error('network unavailable'))
    const store = useUpdateStore()
    await store.refresh()
    expect(store.channelDisabled).toBe(false)
    expect(store.error?.code).toBe('update_call_failed')
  })
  it('retains ordinary update status when the feed is enabled', async () => {
    mocks.check.mockResolvedValue({ status: {
      currentVersion: '1.0.0', latestVersion: '1.1.0', desktopCurrentVersion: '1.0.0',
      updateAvailable: true, desktopUpdateAvailable: false,
    }, error: null })
    const store = useUpdateStore()
    await store.refresh()
    expect(store.channelDisabled).toBe(false)
    expect(store.data?.updateAvailable).toBe(true)
  })
})
