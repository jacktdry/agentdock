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

describe('P8 operator route choices and safe next steps', () => {
  const renderRoutes = (s: Snapshot) => renderToString(createSSRApp(BrowserRouteObservations, { snapshot: s }))
  const assertMessage = (html: string, key: Parameters<typeof t>[0]) =>
    expect(html).toContain(t(key).replaceAll("'", '&#39;'))

  it.each(['en', 'zh-Hant', 'zh-Hans'] as const)('explains three distinct routes without live-health claims in %s', async locale => {
    setLocale(locale)
    const html = await renderRoutes(snapshot())
    for (const key of [
      'browser.route_guide_title',
      'browser.route_managed_title', 'browser.route_managed_unverified',
      'browser.route_company_title', 'browser.route_company_unverified',
      'browser.route_explicit_title', 'browser.route_explicit_unobserved',
    ] as const) {
      expect(t(key)).not.toBe(key)
      assertMessage(html, key)
    }
    expect(html).toContain('aria-labelledby="browser-route-guide-title"')
    expect(html).not.toMatch(/<(button|input|a)[ >]/)
    expect(mocks.store.refresh).not.toHaveBeenCalled()
  })

  it('does not confuse missing policies/definitions with a verified external route', async () => {
    setLocale('en')
    const policiesMissing = await renderRoutes({ ...snapshot(), companyRequiredEdgePolicies: 0 })
    assertMessage(policiesMissing, 'browser.route_company_no_policy')
    const profileMissing = await renderRoutes({ ...snapshot(), configuredAuthenticatedEdgeProfiles: 0 })
    assertMessage(profileMissing, 'browser.route_company_incomplete')
    const connectorMissing = await renderRoutes({ ...snapshot(), configuredConnectors: 0 })
    assertMessage(connectorMissing, 'browser.route_company_incomplete')

    // Even a misleading raw 'healthy' string or large configured counts never
    // certifies a company Edge login, CDP target, consent or exact profile.
    const configured = await renderRoutes({
      ...snapshot(), connectorHealth: 'healthy', configuredAuthenticatedEdgeProfiles: 200,
    })
    assertMessage(configured, 'browser.route_company_unverified')
    expect(configured).not.toContain('Edge is authenticated')
    expect(configured).not.toContain('CDP is connected')
    assertMessage(configured, 'browser.route_explicit_unobserved')
  })

  it('handles stale/disabled/malformed observation without offering a browser mutation', async () => {
    setLocale('en')
    for (const modified of [
      { availability: 'browser_disabled' }, { availability: 'acp_disabled' },
      { availability: 'broker_unavailable' }, { stale: true }, { state: 'stale' },
      { failedLeases: 1 }, { lifecycleError: true }, { owners: -1 },
      { managedPoolMode: 'unknown' },
      { managedPoolMode: 'experimental_shared', sharedPoolBudget: { capacity: 1, occupied: 1, available: 0, claimed: 1, uncertain: 1 } },
    ]) {
      const html = await renderRoutes({ ...snapshot(), ...modified })
      assertMessage(html, 'browser.route_managed_unavailable')
      expect(html).not.toContain(t('browser.route_managed_unverified'))
    }
    for (const malformed of [
      { companyRequiredEdgePolicies: -1 }, { configuredAuthenticatedEdgeProfiles: NaN },
      { configuredConnectors: 0.5 }, { availability: 'core_unavailable' },
    ]) {
      const html = await renderRoutes({ ...snapshot(), ...malformed })
      assertMessage(html, 'browser.route_company_unknown')
      expect(html).not.toContain(t('browser.route_company_unverified'))
      expect(html).not.toMatch(/<(button|input|a)[ >]/)
    }
  })
})
