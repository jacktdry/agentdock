import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Snapshot } from '../../bindings/github.com/uvwt/agentdock/internal/browserdesktop/models'
const mocks = vi.hoisted(() => ({ snapshot: vi.fn(), load: vi.fn(), canInvoke: vi.fn(), error: null as unknown }))
vi.mock('../api/desktopApi', () => ({ Domain: { DomainBrowser: 'browser' }, desktopApi: { browserSnapshot: mocks.snapshot } }))
vi.mock('./contract', () => ({ useContractStore: () => ({ load: mocks.load, canInvoke: mocks.canInvoke, get error() { return mocks.error } }) }))
import { useBrowserStore } from './browser'
const idle = (): Snapshot => ({
  observedAt: '2026-10-08T00:00:00Z', availability: 'available', state: 'idle', stale: false,
  browserEnabled: true, acpEnabled: true, companyRequiredEdgePolicies: 1,
  configuredConnectors: 2, configuredAuthenticatedEdgeProfiles: 1, configuredRequiredExternalPolicies: 2,
  connectorHealth: 'not_observed', managedLeases: 0, requiredExternalLeases: 0, explicitExternalLeases: 0,
  owners: 0, leases: 0, workers: 0, activeLeases: 0, expiredLeases: 0, releasingLeases: 0,
  failedLeases: 0, unownedLeases: 0, readyWorkers: 0, failedWorkers: 0, activeOperations: 0,
  queuedOperations: 0, maxConcurrency: 4, queueCapacity: 8, managedOrphans: 0, externalOrphans: 0, lifecycleError: false,
})
beforeEach(() => {
  vi.resetAllMocks()
  setActivePinia(createPinia())
  mocks.error = null
  mocks.canInvoke.mockReturnValue(true)
  mocks.snapshot.mockResolvedValue({ snapshot: idle() })
})
describe('read-only browser observations', () => {
  it('gates snapshot on negotiated browser capability', async () => {
    mocks.canInvoke.mockReturnValue(false)
    const store = useBrowserStore()
    await store.refresh()
    expect(mocks.canInvoke).toHaveBeenCalledWith('browser', 'snapshot')
    expect(mocks.snapshot).not.toHaveBeenCalled()
    expect(store.snapshot).toBeNull()
    expect(store.error).toBe('browser.capability_unavailable')
  })
  it.each(['rejected', 'exception'])('fails closed after negotiation %s even if capability is present', async mode => {
    const store = useBrowserStore()
    await store.refresh()
    if (mode === 'rejected') mocks.error = { message: '/secret/token' }
    else mocks.load.mockRejectedValue(new Error('/secret/token'))
    await store.refresh()
    expect(mocks.snapshot).toHaveBeenCalledTimes(1)
    expect(store.snapshot).toBeNull()
    expect(JSON.stringify(store.error)).not.toContain('secret')
  })
  it.each(['error', 'exception', 'core_unavailable'])('clears previous observation on %s instead of reporting idle', async mode => {
    const store = useBrowserStore()
    await store.refresh()
    expect(store.stateKey).toBe('browser.idle')
    if (mode === 'exception') mocks.snapshot.mockRejectedValue(new Error('/secret/token'))
    else mocks.snapshot.mockResolvedValue({ snapshot: { ...idle(), availability: 'core_unavailable' }, error: mode === 'error' ? { message: '/secret/token' } : undefined })
    await store.refresh()
    expect(store.snapshot).toBeNull()
    expect(store.stateKey).toBe('browser.unavailable')
    expect(store.error).toBe(mode === 'exception' ? 'browser.snapshot_unavailable' : 'browser.core_unavailable')
  })
  it.each(['browser_disabled', 'acp_disabled', 'broker_unavailable'])('distinguishes %s from confirmed idle', async availability => {
    mocks.snapshot.mockResolvedValue({ snapshot: { ...idle(), availability } })
    const store = useBrowserStore()
    await store.refresh()
    expect(store.availabilityKey).toBe('browser.' + availability)
    expect(store.stateKey).toBe('browser.unavailable')
    expect(store.error).toBeNull()
    expect(store.snapshot?.configuredConnectors).toBe(2)
    expect(store.snapshot?.configuredRequiredExternalPolicies).toBe(2)
    expect(store.snapshot?.connectorHealth).toBe('not_observed')
  })
  it('does not interpret observation age as stale or retained leases as running', async () => {
    const store = useBrowserStore()
    await store.refresh()
    expect(store.stateKey).toBe('browser.idle')
    mocks.snapshot.mockResolvedValue({ snapshot: { ...idle(), state: 'leases_present', leases: 3, activeLeases: 1 } })
    await store.refresh()
    expect(store.stateKey).toBe('browser.leases_present')
    mocks.snapshot.mockResolvedValue({ snapshot: { ...idle(), state: 'stale', stale: true, failedWorkers: 1 } })
    await store.refresh()
    expect(store.availabilityKey).toBe('browser.available')
    expect(store.stateKey).toBe('browser.stale')
  })
  it('clears counters during refresh and prevents concurrent duplicate reads', async () => {
    const store = useBrowserStore()
    await store.refresh()
    let finish!: (value: { snapshot: Snapshot }) => void
    mocks.snapshot.mockReturnValue(new Promise(resolve => { finish = resolve }))
    const pending = store.refresh()
    expect(store.snapshot).toBeNull()
    await store.refresh()
    expect(mocks.snapshot).toHaveBeenCalledTimes(2)
    finish({ snapshot: idle() }); await pending
    expect(store.busy).toBe(false)
  })
})
