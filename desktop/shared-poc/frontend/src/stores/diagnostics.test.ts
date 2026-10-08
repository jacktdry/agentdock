import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
const mocks = vi.hoisted(() => ({ snapshot: vi.fn(), directories: vi.fn(), open: vi.fn() }))
vi.mock('../api/desktopApi', () => ({
  desktopApi: { diagnostics: mocks.snapshot, diagnosticsDirectories: mocks.directories, openNextDirectory: mocks.open },
  clientError: (code: string, error: Error) => ({ code, message: error.message, category: 'internal', retryable: false }),
}))
import { useDiagnosticsStore } from './diagnostics'
beforeEach(() => {
  vi.resetAllMocks()
  setActivePinia(createPinia())
  mocks.snapshot.mockResolvedValue({ snapshot: { platform: 'darwin' } })
  mocks.directories.mockResolvedValue({ logs: { enabled: true, reason: '' }, configuration: { enabled: true, reason: '' } })
})
describe('diagnostics_directory_intents', () => {
  it('disables unsupported actions without invoking native operations', async () => {
    mocks.directories.mockResolvedValue({ logs: { enabled: false, reason: 'requires_native' }, configuration: { enabled: false, reason: 'requires_native' } })
    const store = useDiagnosticsStore()
    await store.refresh()
    await store.openDirectory('logs')
    expect(mocks.open).not.toHaveBeenCalled()
    expect(store.directories?.logs.reason).toBe('requires_native')
  })
  it('keeps new capability/error state free of paths, commands and secrets', async () => {
    const store = useDiagnosticsStore()
    mocks.directories.mockResolvedValue({ logs: { enabled: false, reason: '/private/secret' }, configuration: { enabled: true, reason: '/private/secret' } })
    await store.refresh()
    expect(JSON.stringify(store.directories)).not.toContain('/private')
    mocks.open.mockResolvedValue({ ok: false, error: { message: '/private/secret', details: { command: 'open private' } } })
    await store.openDirectory('configuration')
    expect(JSON.stringify(store.openError)).not.toContain('private')
    expect(store.opened).toBe(false)
    expect(store.directories).toBeNull()
    mocks.directories.mockRejectedValue(new Error('/private/secret'))
    await store.refresh()
    expect(JSON.stringify(store.error)).not.toContain('private')
  })
  it('sends only the enum once and requires refresh after failure without retry', async () => {
    const store = useDiagnosticsStore()
    await store.refresh()
    let finish!: (value: { ok: boolean }) => void
    mocks.open.mockReturnValue(new Promise(resolve => { finish = resolve }))
    const pending = store.openDirectory('logs')
    await store.openDirectory('configuration')
    expect(mocks.open).toHaveBeenCalledExactlyOnceWith('logs')
    finish({ ok: true }); await pending
    expect(store.opened).toBe(true)
    mocks.open.mockRejectedValue(new Error('/secret/command'))
    await store.openDirectory('logs')
    await store.openDirectory('logs')
    expect(mocks.open).toHaveBeenCalledTimes(2)
    expect(JSON.stringify(store.openError)).not.toContain('secret')
    expect(store.directories).toBeNull()
  })
})
