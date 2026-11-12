import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const mocks = vi.hoisted(() => ({ read: vi.fn(), save: vi.fn() }))
vi.mock('../api/desktopApi', () => ({
  desktopApi: { readBasicSettings: mocks.read, saveBasicSettings: mocks.save },
  clientError: (code: string) => ({ code, message: '', category: 'internal', retryable: false }),
}))
import { useBasicSettingsStore } from './basicSettings'

const settings = { port: 8767, logLevel: 'info', coreAutostart: false }
const goodRead = () => ({ settings, coreAutostartMutable: false, portMutable: false, error: null })
const readError = { code: 'settings_read_failed', message: '', category: 'unavailable', retryable: true }

beforeEach(() => {
  vi.resetAllMocks()
  setActivePinia(createPinia())
  mocks.read.mockResolvedValue(goodRead())
  mocks.save.mockResolvedValue({ completed: true, error: null })
})

describe('basic_settings_save_verification', () => {
  it('completes normally only after an authoritative read succeeds', async () => {
    const store = useBasicSettingsStore()
    await store.load()
    mocks.read.mockResolvedValueOnce({ ...goodRead(), settings: { ...settings, logLevel: 'warn' } })
    await store.save({ ...settings, logLevel: 'warn' })
    expect(store.current?.logLevel).toBe('warn')
    expect(store.completed).toBe(true)
    expect(store.error).toBeNull()
  })

  it('preserves a rejected save and does not perform follow-up reads', async () => {
    const store = useBasicSettingsStore()
    await store.load()
    mocks.save.mockResolvedValueOnce({ completed: false, error: { code: 'settings_save_rejected' } })
    await store.save({ ...settings, logLevel: 'warn' })
    expect(store.completed).toBe(false)
    expect(store.error?.code).toBe('settings_save_rejected')
    expect(mocks.read).toHaveBeenCalledTimes(1)
  })

  it('does not claim a verified save when the follow-up read reports an error', async () => {
    const store = useBasicSettingsStore()
    await store.load()
    mocks.read.mockResolvedValueOnce({ error: readError })
    await store.save({ ...settings, logLevel: 'warn' })
    expect(mocks.save).toHaveBeenCalledTimes(1)
    expect(store.current).toBeNull()
    expect(store.error?.code).toBe('settings_read_failed')
    expect(store.completed).toBe(false)
  })

  it('reports a read failure, not a save failure, when verification throws after a successful save', async () => {
    const store = useBasicSettingsStore()
    await store.load()
    mocks.read.mockRejectedValueOnce(new Error('verification unavailable'))
    await store.save({ ...settings, logLevel: 'warn' })
    expect(store.current).toBeNull()
    expect(store.error?.code).toBe('settings_read_call_failed')
    expect(store.completed).toBe(false)
  })
})
