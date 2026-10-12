import { describe, expect, it, vi } from 'vitest'
import { createSSRApp } from 'vue'
import { renderToString } from 'vue/server-renderer'
import type { Snapshot } from '../../../bindings/github.com/uvwt/agentdock/internal/browserdesktop/models'
import { setLocale, t } from '../../i18n'
import BrowserSnapshotDetails from './BrowserSnapshotDetails.vue'
import BrowserRouteObservations from './BrowserRouteObservations.vue'
const mocks = vi.hoisted(() => ({ store: { snapshot: null as Snapshot | null, busy: false, error: null as string | null, availabilityKey: 'browser.broker_unavailable', stateKey: 'browser.unavailable', refresh: vi.fn() } }))
vi.mock('../../stores/browser', () => ({ useBrowserStore: () => mocks.store }))
import BrowserPanel from './BrowserPanel.vue'
const snapshot = (): Snapshot => ({
  managedPoolMode: 'exclusive',
  observedAt: '2026-10-08T00:00:00Z', availability: 'available', state: 'leases_present', stale: false,
  browserEnabled: true, acpEnabled: true, companyRequiredEdgePolicies: 1,
  configuredConnectors: 2, configuredAuthenticatedEdgeProfiles: 3, configuredRequiredExternalPolicies: 4,
  connectorHealth: 'not_observed', managedLeases: 5, requiredExternalLeases: 6, explicitExternalLeases: 7,
  authenticatedTransportOwners: 0, owners: 1, leases: 18, workers: 1, activeLeases: 1, expiredLeases: 0, releasingLeases: 0,
  failedLeases: 0, unownedLeases: 0, readyWorkers: 1, failedWorkers: 0, activeOperations: 0,
  queuedOperations: 0, maxConcurrency: 4, queueCapacity: 8, managedOrphans: 0, externalOrphans: 0, lifecycleError: false,
})
describe('passive route presentation', () => {
  it.each(['en', 'zh-Hant', 'zh-Hans'] as const)('renders separate config, retained and unchecked health in %s', async locale => {
    setLocale(locale)
    const html = await renderToString(createSSRApp(BrowserRouteObservations, { snapshot: snapshot() }))
    expect(html).toContain(t('browser.configured_required_external_policies') + '</dt><dd>4</dd>')
    expect(html).toContain(t('browser.managed_leases') + '</dt><dd>5</dd>')
    expect(html).toContain(t('browser.required_external_leases') + '</dt><dd>6</dd>')
    expect(html).toContain(t('browser.explicit_external_leases') + '</dt><dd>7</dd>')
    expect(html).toContain(t('browser.connector_health_not_observed'))
    expect(html).toContain(t('browser.connector_health_hint'))
    expect(mocks.store.refresh).not.toHaveBeenCalled()
  })
  it.each(['broker_unavailable', 'browser_disabled', 'acp_disabled'])('keeps verified config and hides every retained grid for %s', async availability => {
    setLocale('en')
    mocks.store.snapshot = { ...snapshot(), availability, state: 'unavailable' }
    mocks.store.error = null
    const html = await renderToString(createSSRApp(BrowserPanel))
    expect(html).toContain(t('browser.configured_connectors') + '</dt><dd>2</dd>')
    expect(html).toContain(t('browser.connector_health_not_observed'))
    expect(html).toContain(t('browser.pool_unknown'))
    expect(html).not.toContain(t('browser.pool_capacity') + '</dt>')
    expect(html).not.toContain(t('browser.retained_routes'))
    expect(html).not.toContain(t('browser.leases') + '</dt>')
    expect(html).not.toContain(t('browser.workers') + '</dt>')
    expect(html).not.toContain(t('browser.managed_leases'))
  })
  it('shows no config or retained observations without a verified Core snapshot', async () => {
    mocks.store.snapshot = null
    mocks.store.error = 'browser.core_unavailable'
    const html = await renderToString(createSSRApp(BrowserPanel))
    expect(html).toContain(t('browser.core_unavailable'))
    expect(html).not.toContain(t('browser.configured_connectors'))
    expect(html).not.toContain(t('browser.retained_routes'))
  })
})

describe('read-only pool bookkeeping presentation', () => {
  it.each(['en', 'zh-Hant', 'zh-Hans'] as const)('renders modes, optional counts and limitations in %s', async locale => {
    setLocale(locale)
    for (const mode of ['exclusive', 'experimental_shared', 'unknown']) {
      const s: Snapshot = { ...snapshot(), managedPoolMode: mode }
      let html = await renderToString(createSSRApp(BrowserSnapshotDetails, { snapshot: s }))
      expect(html).toContain(t(mode === 'exclusive' ? 'browser.pool_exclusive' : mode === 'experimental_shared' ? 'browser.pool_shared' : 'browser.pool_unknown'))
      expect(html).toContain(t('browser.pool_hint'))
      expect(html).not.toContain(t('browser.pool_capacity') + '</dt>')
      s.sharedPoolBudget = { capacity: 2, occupied: 1, available: 1, claimed: 3, uncertain: 1 }
      html = await renderToString(createSSRApp(BrowserSnapshotDetails, { snapshot: s }))
      if (mode === 'experimental_shared') {
        for (const [key, count] of [['capacity', 2], ['occupied', 1], ['available', 1], ['claimed', 3], ['uncertain', 1]] as const) {
          expect(html).toContain(t(`browser.pool_${key}`) + `</dt><dd>${count}</dd>`)
        }
      } else expect(html).not.toContain(t('browser.pool_capacity') + '</dt>')
      expect(html).not.toMatch(/<(button|input|a)[ >]/)
    }
  })
})

describe('authenticated MCP endpoint observation', () => {
  it.each(['en', 'zh-Hant', 'zh-Hans'] as const)('shows separate counts and scope in %s', async locale => {
    setLocale(locale)
    const s = { ...snapshot(), owners: 2, authenticatedTransportOwners: 1 }
    const html = await renderToString(createSSRApp(BrowserSnapshotDetails, { snapshot: s }))
    expect(html).toContain(t('browser.owners') + '</dt><dd>2</dd>')
    expect(html).toContain(t('browser.authenticated_transport_owners') + '</dt><dd>1</dd>')
    expect(t('browser.authenticated_transport_hint')).not.toBe('browser.authenticated_transport_hint')
    expect(html).toContain(t('browser.authenticated_transport_hint'))
    expect(html).not.toMatch(/<(button|input|a)[ >]/)
    expect(mocks.store.refresh).not.toHaveBeenCalled()
  })
  it.each([-1, 2, NaN, 0.5])('hides inconsistent transport counts %s', async observed => {
    setLocale('en')
    const html = await renderToString(createSSRApp(BrowserSnapshotDetails, {
      snapshot: { ...snapshot(), authenticatedTransportOwners: observed },
    }))
    expect(html).toContain(t('browser.pool_unknown'))
    expect(html).not.toContain(t('browser.authenticated_transport_owners') + '</dt>')
    expect(html).not.toContain(t('browser.owners') + '</dt>')
  })
})
